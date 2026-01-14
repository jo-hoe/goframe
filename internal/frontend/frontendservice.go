package fontend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
)

const MainPageName = "index.html"

type FrontendService struct {
	coreService *core.CoreService
	config      *core.ServiceConfig
}

func NewFrontendService(config *core.ServiceConfig, coreService *core.CoreService) *FrontendService {
	return &FrontendService{
		coreService: coreService,
		config:      config,
	}
}

// rootRedirectHandler redirects root path to index.html
func (service *FrontendService) rootRedirectHandler(ctx echo.Context) error {
	return ctx.Redirect(http.StatusMovedPermanently, "/"+MainPageName)
}

func (service *FrontendService) SetRoutes(e *echo.Echo) {
	// Create template with helper functions
	funcMap := template.FuncMap{}

	e.Renderer = &Template{
		templates: template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, viewsPattern)),
	}

	e.GET("/", service.rootRedirectHandler) // Redirect root to index.html
	e.GET("/"+MainPageName, service.indexHandler)
	e.POST("/htmx/uploadImage", service.htmxUploadImageHandler)
	e.GET("/htmx/image", service.htmxGetCurrentImageHandler)

	// New routes for listing, fetching by ID, and deleting images
	e.GET("/htmx/images", service.htmxListImagesHandler)
	e.GET("/htmx/image/:id", service.htmxGetImageByIDHandler)
	e.DELETE("/htmx/image/:id", service.htmxDeleteImageHandler)
}

func (service *FrontendService) htmxGetCurrentImageHandler(ctx echo.Context) error {
	image, err := service.coreService.GetImageForDate()
	if err != nil {
		// Explicit logging of error with status code and route
		slog.Warn("htmxGetCurrentImageHandler: no image available",
			"status", http.StatusNotFound,
			"route", "/htmx/image",
			"error", err)
		return ctx.String(http.StatusNotFound, "No image available")
	}

	// Prevent caching so the latest uploaded image is always shown
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	// Return the image data
	return ctx.Blob(http.StatusOK, "image/png", image)
}

func (service *FrontendService) indexHandler(ctx echo.Context) error {
	return ctx.Render(http.StatusOK, MainPageName, nil)
}

func (service *FrontendService) htmxUploadImageHandler(ctx echo.Context) error {
	// Get uploaded file
	file, err := ctx.FormFile("image")
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to get uploaded file",
			"status", http.StatusBadRequest, "error", err)
		return ctx.String(http.StatusBadRequest, "Failed to get uploaded file")
	}

	src, err := file.Open()
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to open uploaded file",
			"status", http.StatusInternalServerError, "error", err, "filename", file.Filename)
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
		slog.Error("htmxUploadImageHandler: failed to read uploaded file",
			"status", http.StatusInternalServerError, "error", err, "filename", file.Filename)
		return ctx.String(http.StatusInternalServerError, "Failed to read uploaded file")
	}

	_, err = service.coreService.AddImage(image)
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to process uploaded image",
			"status", http.StatusInternalServerError, "error", err, "filename", file.Filename)
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	// Return an out-of-band swap to refresh the displayed image, plus a simple status message
	ts := fmt.Sprintf("%d", time.Now().UnixNano())

	// Build out-of-band update for the current image
	currentImageOOB := fmt.Sprintf(`<img id="current-image" src="/htmx/image?ts=%s" hx-swap-oob="true">`, ts)

	// Build out-of-band update for the image list
	ids, err := service.coreService.GetAllImageIDs()
	if err != nil {
		// If building the list fails, still return the current image update and upload result
		slog.Error("htmxUploadImageHandler: failed to list images for OOB update",
			"status", http.StatusInternalServerError, "error", err)
		html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s`, file.Filename, currentImageOOB)
		return ctx.HTML(http.StatusOK, html)
	}
	var listBuilder strings.Builder
	if len(ids) == 0 {
		listBuilder.WriteString(`<p>No images uploaded yet.</p>`)
	} else {
		listBuilder.WriteString(`<div class="grid">`)
		for _, id := range ids {
			listBuilder.WriteString(fmt.Sprintf(`<article>
	<img src="/htmx/image/%s?ts=%s" alt="Image %s" style="max-width:100%%;height:auto">
	<footer>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article>`, id, ts, id, id))
		}
		listBuilder.WriteString(`</div>`)
	}
	imageListOOB := fmt.Sprintf(`<div id="image-list" hx-swap-oob="true">%s</div>`, listBuilder.String())

	// Return combined HTML with OOB swaps for current image and image list
	html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s%s`, file.Filename, currentImageOOB, imageListOOB)
	return ctx.HTML(http.StatusOK, html)
}

func (service *FrontendService) htmxListImagesHandler(ctx echo.Context) error {
	ids, err := service.coreService.GetAllImageIDs()
	if err != nil {
		slog.Error("htmxListImagesHandler: failed to list images",
			"status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}

	var b strings.Builder
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(ids) == 0 {
		b.WriteString(`<p>No images uploaded yet.</p>`)
	} else {
		b.WriteString(`<div class="grid">`)
		for _, id := range ids {
			b.WriteString(fmt.Sprintf(`<article>
	<img src="/htmx/image/%s?ts=%s" alt="Image %s" style="max-width:100%%;height:auto">
	<footer>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article>`, id, ts, id, id))
		}
		b.WriteString(`</div>`)
	}

	// Prevent caching so the latest images are always shown
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	return ctx.HTML(http.StatusOK, b.String())
}

func (service *FrontendService) htmxGetImageByIDHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxGetImageByIDHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	image, err := service.coreService.GetProcessedImageByID(id)
	if err != nil {
		slog.Warn("htmxGetImageByIDHandler: image not found",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Image not found")
	}

	// Prevent caching
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	return ctx.Blob(http.StatusOK, "image/png", image)
}

func (service *FrontendService) htmxDeleteImageHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxDeleteImageHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	if err := service.coreService.DeleteImage(id); err != nil {
		slog.Error("htmxDeleteImageHandler: failed to delete image",
			"status", http.StatusInternalServerError, "image_id", id, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to delete image")
	}

	// Return updated list HTML so HTMX can refresh the list
	return service.htmxListImagesHandler(ctx)
}
