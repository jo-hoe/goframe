package fontend

import (
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

func (service *FrontendService) SetUIRoutes(e *echo.Echo) {
	// Create template with helper functions
	funcMap := template.FuncMap{}

	e.Renderer = &Template{
		templates: template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, viewsPattern)),
	}

	e.GET("/", service.rootRedirectHandler) // Redirect root to index.html
	e.GET(MainPageName, service.indexHandler)
	e.POST("/htmx/uploadImage", service.htmxUploadImageHandler)
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
	imageFile, err := file.Open()
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer imageFile.Close()

	// For demonstration, just return the filename
	return ctx.String(http.StatusOK, "Uploaded file: "+file.Filename)
}
