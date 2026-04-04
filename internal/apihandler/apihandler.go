package apihandler

import (
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/jo-hoe/goframe/internal/core"

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
	e.GET("/api/images/:id/processed.png", s.handleGetProcessedImageByID)
	e.GET("/api/images/:id/original.png", s.handleGetOriginalImageByID)
	e.GET("/api/images", s.handleListImages)
	e.DELETE("/api/images/:id", s.handleDeleteImageByID)
}

// writePNG writes a PNG byte slice with consistent headers.
func (s *APIService) writePNG(ctx echo.Context, png []byte) error {
	ctx.Response().Header().Set(echo.HeaderContentLength, strconv.Itoa(len(png)))
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")
	return ctx.Blob(http.StatusOK, "image/png", png)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	now := time.Now()
	imageId, err := s.coreService.GetImageForTime(ctx.Request().Context(), now)
	if err != nil {
		slog.Error("failed to get current image id", "error", err, "at", now, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to get current image")
	}

	imageData, err := s.coreService.GetImageById(ctx.Request().Context(), imageId)
	if err != nil {
		slog.Error("failed to get image data", "imageId", imageId, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to get image data")
	}

	return s.writePNG(ctx, imageData.ProcessedImage)
}

func (s *APIService) handleUploadImage(ctx echo.Context) error {
	form, err := ctx.MultipartForm()
	if err != nil {
		slog.Info("invalid multipart form", "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Invalid multipart form")
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
		slog.Info("no file provided in multipart form", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "No file provided")
	}

	src, err := fh.Open()
	if err != nil {
		slog.Error("failed to open uploaded file", "file", fh.Filename, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		slog.Error("failed to read uploaded file", "file", fh.Filename, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	source := ""
	if sv := form.Value["source"]; len(sv) > 0 {
		source = sv[0]
	}

	apiImg, err := s.coreService.AddImage(ctx.Request().Context(), data, source)
	if err != nil {
		slog.Error("failed to process uploaded image", "file", fh.Filename, "sizeBytes", len(data), "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	return ctx.JSON(http.StatusCreated, map[string]string{
		"id": apiImg.ID,
	})
}

// getImageBytesByID fetches either processed or original bytes for an image ID.
func (s *APIService) getImageBytesByID(ctx echo.Context, id string, processed bool) ([]byte, error) {
	img, err := s.coreService.GetImageById(ctx.Request().Context(), id)
	if err != nil {
		return nil, err
	}
	if processed {
		return img.ProcessedImage, nil
	}
	return img.OriginalImage, nil
}

func (s *APIService) handleGetProcessedImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Info("missing image id parameter", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	data, err := s.getImageBytesByID(ctx, id, true)
	if err != nil {
		slog.Info("processed image not found", "imageId", id, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return s.writePNG(ctx, data)
}

func (s *APIService) handleGetOriginalImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Info("missing image id parameter", "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	data, err := s.getImageBytesByID(ctx, id, false)
	if err != nil {
		slog.Info("original image not found", "imageId", id, "error", err, "method", ctx.Request().Method, "path", ctx.Request().URL.Path)
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return s.writePNG(ctx, data)
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
		items = append(items, imageListItem{
			ID:           img.ID,
			CreatedAt:    img.CreatedAt,
			ProcessedURL: "/api/images/" + img.ID + "/processed.png",
			OriginalURL:  "/api/images/" + img.ID + "/original.png",
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
