package backend

import (
	"fmt"
	"log"
	"strconv"

	"github.com/jo-hoe/goframe/internal/common"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type APIService struct {
	port int
}

func NewAPIService(port int) *APIService {
	return &APIService{port: port}
}

func (s *APIService) Start() {
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Pre(middleware.RemoveTrailingSlash())

	e.Validator = &common.GenericEchoValidator{}

	setRoutes(e)

	port := strconv.Itoa(s.port)
	log.Printf("starting server on port %s", port)
	e.Logger.Fatal(e.Start(fmt.Sprintf(":%s", port)))
}

func setRoutes(e *echo.Echo) {
	// Set probe route
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "API Service is running")
	})
}
