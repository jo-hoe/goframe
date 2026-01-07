package fontend

import (
	"net/http"
	"text/template"

	"github.com/labstack/echo/v4"
)

const MainPageName = "index.html"

type FrontendService struct {
}

func NewFrontendService() *FrontendService {
	return &FrontendService{}
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
	// Handle image upload via HTMX
	return ctx.String(http.StatusOK, "Image uploaded successfully via HTMX")
}
