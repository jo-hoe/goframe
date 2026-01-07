package fontend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const MainPageName = "index.html"

type FrontendService struct {
	coreService *core.CoreService
}

func NewFrontendService(coreService *core.CoreService) *FrontendService {
	return &FrontendService{
		coreService: coreService,
	}
}

// rootRedirectHandler redirects root path to index.html
func (service *FrontendService) rootRedirectHandler(ctx echo.Context) error {
	return ctx.Redirect(http.StatusMovedPermanently, "/"+MainPageName)
}

func (service *FrontendService) Start() {
	e := echo.New()

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Pre(middleware.RemoveTrailingSlash())

	service.setUIRoutes(e)

	// Start server
	portString := fmt.Sprintf(":%d", service.coreService.GetConfig().Port)
	e.Logger.Fatal(e.Start(portString))
}

func (service *FrontendService) setUIRoutes(e *echo.Echo) {
	// Create template with helper functions
	funcMap := template.FuncMap{}

	e.Renderer = &Template{
		templates: template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, viewsPattern)),
	}

	e.GET("/", service.rootRedirectHandler) // Redirect root to index.html
	e.GET(MainPageName, service.indexHandler)
	e.POST("/htmx/uploadImage", service.htmxUploadImageHandler)
	e.GET("/htmx/image", service.htmxGetCurrentImageHandler)
}

func (service *FrontendService) htmxGetCurrentImageHandler(ctx echo.Context) error {
	image, err := service.coreService.GetCurrentImage()
	if err != nil {
		return ctx.String(http.StatusNotFound, "No image available")
	}

	imageType := service.coreService.GetConfig().ImageTargetType
	contentType := "image/" + imageType

	// Prevent caching so the latest uploaded image is always shown
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	// Return the image data
	return ctx.Blob(http.StatusOK, contentType, image)
}

func (service *FrontendService) indexHandler(ctx echo.Context) error {
	return ctx.Render(http.StatusOK, MainPageName, nil)
}

func (service *FrontendService) htmxUploadImageHandler(ctx echo.Context) error {
	// Get uploaded file
	file, err := ctx.FormFile("image")
	if err != nil {
		return ctx.String(http.StatusBadRequest, "Failed to get uploaded file")
	}

	src, err := file.Open()
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer func() {
		if cerr := src.Close(); cerr != nil {
			slog.Error("htmxUploadImageHandler: failed to close uploaded file reader", "error", cerr, "filename", file.Filename)
		}
	}()

	// Read file content reliably
	image, err := io.ReadAll(src)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	_, err = service.coreService.AddImage(image)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	// Return an out-of-band swap to refresh the displayed image, plus a simple status message
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div><img id="current-image" src="/htmx/image?ts=%s" hx-swap-oob="true">`, file.Filename, ts)
	return ctx.HTML(http.StatusOK, html)
}
