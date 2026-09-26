package apihandler

import (
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/jo-hoe/goframe/internal/core"
	"github.com/jo-hoe/goframe/internal/database"
	"github.com/jo-hoe/goframe/internal/imagevalidation"

	"github.com/labstack/echo/v4"
)

// APIService wires the goframe REST API routes to the Echo server.
type APIService struct {
	coreService *core.CoreService
}

// NewAPIService creates a new APIService backed by the given CoreService.
func NewAPIService(coreService *core.CoreService) *APIService {
	return &APIService{
		coreService: coreService,
	}
}

// SetRoutes registers all API routes on the given Echo instance.
func (s *APIService) SetRoutes(e *echo.Echo) {
	e.GET("/probe", func(c echo.Context) error {
		return c.String(200, "API Service is running")
	})

	e.GET("/api/image.png", s.handleGetCurrentImage)
	e.POST("/api/image", s.handleUploadImage)
	e.GET("/api/images/:id/status", s.handleGetUploadStatus)
	e.GET("/api/images/:id/processed.png", s.handleGetProcessedImageByID)
	e.GET("/api/images/:id/original.png", s.handleGetOriginalImageByID)
	e.GET("/api/images", s.handleListImages)
	e.DELETE("/api/images/:id", s.handleDeleteImageByID)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	now := time.Now()
	imageID, err := s.coreService.GetImageForTime(ctx.Request().Context(), now)
	if err != nil {
		slog.Error("failed to get current image id", "error", err, "at", now, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to get current image")
	}

	imageURL, err := s.coreService.GetImageURL(ctx.Request().Context(), imageID, "processed")
	if err != nil {
		slog.Error("failed to get image url", "imageId", imageID, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to get image URL")
	}

	return ctx.Redirect(http.StatusFound, imageURL)
}

// readSingleUploadedFile extracts the bytes of the first file part from a
// multipart form. It is shared by the upload handlers. It returns a client
// error (400) when no file is present and a server error (500) on read failure.
func readSingleUploadedFile(ctx echo.Context) ([]byte, string, error) {
	form, err := ctx.MultipartForm()
	if err != nil {
		return nil, "", echo.NewHTTPError(http.StatusBadRequest, "Invalid multipart form")
	}
	defer func() { _ = form.RemoveAll() }()

	var fh *multipart.FileHeader
	for _, fhs := range form.File {
		if len(fhs) > 0 {
			fh = fhs[0]
			break
		}
	}
	if fh == nil {
		return nil, "", echo.NewHTTPError(http.StatusBadRequest, "No file provided")
	}

	src, err := fh.Open()
	if err != nil {
		return nil, fh.Filename, echo.NewHTTPError(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		return nil, fh.Filename, echo.NewHTTPError(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	source := ""
	if sv := form.Value["source"]; len(sv) > 0 {
		source = sv[0]
	}
	return data, source, nil
}

type uploadResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"statusUrl"`
}

func (s *APIService) handleUploadImage(ctx echo.Context) error {
	data, source, err := readSingleUploadedFile(ctx)
	if err != nil {
		return err
	}

	state, err := s.coreService.SubmitImage(ctx.Request().Context(), data, source)
	if err != nil {
		if verr, ok := imagevalidation.AsValidationError(err); ok {
			slog.Info("rejected invalid upload", "reason", verr.Reason, "sizeBytes", len(data))
			return ctx.String(http.StatusBadRequest, verr.Reason)
		}
		slog.Error("failed to submit uploaded image", "sizeBytes", len(data), "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to submit uploaded image")
	}

	statusURL := "/api/images/" + state.ID + "/status"
	ctx.Response().Header().Set("Location", statusURL)

	// 200 when the image already exists (idempotent repeat), 202 when accepted
	// for (or already undergoing) background processing.
	code := http.StatusAccepted
	if state.Status == database.StatusSucceeded {
		code = http.StatusOK
	}
	return ctx.JSON(code, uploadResponse{
		ID:        state.ID,
		Status:    string(state.Status),
		StatusURL: statusURL,
	})
}

func (s *APIService) handleGetUploadStatus(ctx echo.Context) error {
	id := ctx.Param("id")
	state, err := s.coreService.GetUploadState(ctx.Request().Context(), id)
	if err != nil {
		slog.Info("failed to get upload status", "imageId", id, "error", err)
		return ctx.String(http.StatusBadRequest, "Invalid image id")
	}
	return ctx.JSON(http.StatusOK, state)
}

func (s *APIService) handleGetProcessedImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Info("missing image id parameter", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	imageURL, err := s.coreService.GetImageURL(ctx.Request().Context(), id, "processed")
	if err != nil {
		slog.Info("processed image not found", "imageId", id, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return ctx.Redirect(http.StatusFound, imageURL)
}

func (s *APIService) handleGetOriginalImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Info("missing image id parameter", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	imageURL, err := s.coreService.GetImageURL(ctx.Request().Context(), id, "original")
	if err != nil {
		slog.Info("original image not found", "imageId", id, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return ctx.Redirect(http.StatusFound, imageURL)
}

type imageListItem struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	ProcessedURL string    `json:"processedUrl"`
	OriginalURL  string    `json:"originalUrl"`
	Source       string    `json:"source,omitempty"`
}

func (s *APIService) handleListImages(ctx echo.Context) error {
	images, err := s.coreService.GetOrderedImages(ctx.Request().Context())
	if err != nil {
		slog.Error("failed to list images", "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}
	items := make([]imageListItem, 0, len(images))
	for _, img := range images {
		processedURL, _ := s.coreService.GetImageURL(ctx.Request().Context(), img.ID, "processed")
		originalURL, _ := s.coreService.GetImageURL(ctx.Request().Context(), img.ID, "original")
		items = append(items, imageListItem{
			ID:           img.ID,
			CreatedAt:    img.CreatedAt,
			ProcessedURL: processedURL,
			OriginalURL:  originalURL,
			Source:       img.Source,
		})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *APIService) handleDeleteImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Info("missing image id parameter for delete", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	if err := s.coreService.DeleteImage(ctx.Request().Context(), id); err != nil {
		slog.Info("attempted to delete non-existing image", "imageId", id, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return ctx.NoContent(http.StatusNoContent)
}
