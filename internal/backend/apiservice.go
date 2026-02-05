package backend

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jo-hoe/goframe/internal/core"

	"github.com/labstack/echo/v4"
)

type APIService struct {
	config      *core.ServiceConfig
	coreService *core.CoreService
}

func NewAPIService(config *core.ServiceConfig, coreService *core.CoreService) *APIService {
	return &APIService{
		config:      config,
		coreService: coreService,
	}
}

func (s *APIService) SetRoutes(e *echo.Echo) {
	// Set probe route
	e.GET("/probe", func(c echo.Context) error {
		return c.String(200, "API Service is running")
	})

	// Current image (processed)
	imageUrl := "/api/image.png"
	e.GET(imageUrl, s.handleGetCurrentImage)

	// Upload a new image via multipart/form-data field "image"
	e.POST("/api/image", s.handleUploadImage)

	// New APIs:
	// - Get processed image by ID
	e.GET("/api/images/:id/processed.png", s.handleGetProcessedImageByID)
	// - Get original image by ID
	e.GET("/api/images/:id/original.png", s.handleGetOriginalImageByID)
	// - List all images with URLs
	e.GET("/api/images", s.handleListImages)
	// - Delete image by ID
	e.DELETE("/api/images/:id", s.handleDeleteImageByID)
}

// writePNG writes a PNG byte slice with consistent headers (DRY).
func (s *APIService) writePNG(ctx echo.Context, png []byte) error {
	// Set Content-Length header explicitly to allow clients to know exact payload size
	ctx.Response().Header().Set(echo.HeaderContentLength, strconv.Itoa(len(png)))
	// Prevent caching so clients fetch the new image after midnight or updates
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")
	return ctx.Blob(http.StatusOK, "image/png", png)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	imageId, err := s.coreService.GetImageForTime(time.Now())
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to get current image")
	}

	imageData, err := s.coreService.GetImageById(imageId)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to get image data")
	}

	return s.writePNG(ctx, imageData.ProcessedImage)
}

func (s *APIService) handleUploadImage(ctx echo.Context) error {
	file, err := ctx.FormFile("image")
	if err != nil {
		return ctx.String(http.StatusBadRequest, "Missing image file")
	}

	src, err := file.Open()
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	apiImg, err := s.coreService.AddImage(data)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	return ctx.JSON(http.StatusCreated, map[string]string{
		"id": apiImg.ID,
	})
}

// getImageBytesByID is a small helper to fetch either processed or original bytes for an image ID (DRY).
func (s *APIService) getImageBytesByID(id string, processed bool) ([]byte, error) {
	img, err := s.coreService.GetImageById(id)
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
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	data, err := s.getImageBytesByID(id, true)
	if err != nil {
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return s.writePNG(ctx, data)
}

func (s *APIService) handleGetOriginalImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	data, err := s.getImageBytesByID(id, false)
	if err != nil {
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return s.writePNG(ctx, data)
}

type imageListItem struct {
	ID           string `json:"id"`
	ProcessedURL string `json:"processedUrl"`
	OriginalURL  string `json:"originalUrl"`
}

func (s *APIService) handleListImages(ctx echo.Context) error {
	ids, err := s.coreService.GetOrderedImageIDs()
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}
	items := make([]imageListItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, imageListItem{
			ID:           id,
			ProcessedURL: "/api/images/" + id + "/processed.png",
			OriginalURL:  "/api/images/" + id + "/original.png",
		})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *APIService) handleDeleteImageByID(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return ctx.String(http.StatusBadRequest, "Missing image id")
	}
	if err := s.coreService.DeleteImage(id); err != nil {
		return ctx.String(http.StatusNotFound, "Image not found")
	}
	return ctx.NoContent(http.StatusNoContent)
}
