package fontend

import (
	"fmt"
	"net/http"
	"text/template"

	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
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
	e.POST("/htmx/image", service.htmxGetCurrentImageHandler)
}

func (service *FrontendService) htmxGetCurrentImageHandler(ctx echo.Context) error {
	image, err := service.coreService.GetCurrentImage()
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to get current image")
	}

	imageType := service.coreService.GetConfig().ImageTargetType
	contentType := "image/" + imageType

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
	defer src.Close()

	// Read file content
	image := make([]byte, file.Size)
	_, err = src.Read(image)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	service.coreService.AddImage(image)

	// For demonstration, just return the filename
	return ctx.String(http.StatusOK, "Uploaded file: "+file.Filename)
}
