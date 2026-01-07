package backend

import (
	"fmt"

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
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "API Service is running")
	})

	imageUrl := fmt.Sprintf("/api/image.%s", s.config.ImageTargetType)
	e.GET(imageUrl, s.handleGetCurrentImage)
}

func (s *APIService) handleGetCurrentImage(ctx echo.Context) error {
	imageData, err := s.coreService.GetCurrentImage()
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.Blob(200, "image/"+s.config.ImageTargetType, imageData)
}
