package backend

import (
	"io"
	"net/http"
	"strconv"

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

	imageUrl := "/api/image.png"
	e.GET(imageUrl, s.handleGetCurrentImage)

	// Upload a new image via multipart/form-data field "image"
	e.POST("/api/image", s.handleUploadImage)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	imageId, err := s.coreService.GetCurrentImageID()
	if err != nil {
		return ctx.String(500, "Failed to get current image")
	}

	imageData, err := s.coreService.GetImageById(imageId)
	if err != nil {
		return ctx.String(500, "Failed to get image data")
	}

	// Set Content-Length header explicitly to allow clients to know exact payload size
	ctx.Response().Header().Set(echo.HeaderContentLength, strconv.Itoa(len(imageData.ProcessedImage)))
	return ctx.Blob(200, "image/png", imageData.ProcessedImage)
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
