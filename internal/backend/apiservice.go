package backend

import (
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

	imageUrl := "/api/image.png"
	e.GET(imageUrl, s.handleGetCurrentImage)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	imageId, err := s.coreService.GetImageForTime(time.Now())
	if err != nil {
		return ctx.String(500, "Failed to get image for current time")
	}

	imageData, err := s.coreService.GetImageById(imageId)
	if err != nil {
		return ctx.String(500, "Failed to get image data")
	}

	// Set Content-Length header explicitly to allow clients to know exact payload size
	ctx.Response().Header().Set(echo.HeaderContentLength, strconv.Itoa(len(imageData.ProcessedImage)))
	return ctx.Blob(200, "image/png", imageData.ProcessedImage)
}
