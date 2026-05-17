package frontend

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
)

const (
	MainPageName = "index.html"
)

type moveDirection string

const (
	dirUp   moveDirection = "up"
	dirDown moveDirection = "down"
)

func parseMoveDirection(s string) (moveDirection, bool) {
	d := moveDirection(strings.ToLower(strings.TrimSpace(s)))
	return d, d == dirUp || d == dirDown
}

// cycleMove moves the element at idx one step in dir, wrapping at the ends.
func cycleMove(order []string, idx int, dir moveDirection) []string {
	n := len(order)
	result := make([]string, n)
	copy(result, order)

	var target int
	switch dir {
	case dirUp:
		target = (idx - 1 + n) % n
	case dirDown:
		target = (idx + 1) % n
	}
	result[idx], result[target] = result[target], result[idx]
	return result
}

func sliceIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

type FrontendService struct {
	coreService *core.CoreService
	config      *config.ServiceConfig
}

func NewFrontendService(config *config.ServiceConfig, coreService *core.CoreService) *FrontendService {
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
	// Create template renderer
	e.Renderer = &Template{
		templates: template.Must(template.New("").ParseFS(templateFS, viewsPattern)),
	}

	e.GET("/", service.rootRedirectHandler) // Redirect root to index.html
	e.GET("/"+MainPageName, service.indexHandler)
	e.POST("/htmx/uploadImage", service.htmxUploadImageHandler)

	// Routes for listing, fetching by ID, and deleting images
	e.GET("/htmx/images", service.htmxListImagesHandler)
	e.GET("/htmx/image/original/:id", service.htmxRedirectOriginalByIDHandler)
	e.DELETE("/htmx/image/:id", service.htmxDeleteImageHandler)
	e.POST("/htmx/image/:id/move", service.htmxMoveImageHandler)

	// Favicon (SVG) route
	e.GET("/icon.svg", service.iconHandler)
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

	_, err = service.coreService.AddImage(ctx.Request().Context(), image, "")
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to process uploaded image",
			"status", http.StatusInternalServerError, "error", err, "filename", file.Filename)
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	// Return an out-of-band swap to refresh the displayed image, plus a simple status message

	// Build out-of-band update for the image list
	imageListHTML, listErr := service.buildImageListHTML(ctx.Request().Context())
	if listErr != nil {
		// If building the list fails, still return the upload result
		slog.Error("htmxUploadImageHandler: failed to list images for OOB update",
			"status", http.StatusInternalServerError, "error", listErr)
		html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>`, file.Filename)
		return ctx.HTML(http.StatusOK, html)
	}
	imageListOOB := fmt.Sprintf(`<div id="image-list" hx-swap-oob="true">%s</div>`, imageListHTML)

	// Return HTML with OOB swap for image list
	html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s`, file.Filename, imageListOOB)
	return ctx.HTML(http.StatusOK, html)
}

func (service *FrontendService) htmxListImagesHandler(ctx echo.Context) error {
	listHTML, err := service.buildImageListHTML(ctx.Request().Context())
	if err != nil {
		slog.Error("htmxListImagesHandler: failed to list images",
			"status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}

	// Prevent caching so the latest images are always shown
	service.setNoCache(ctx)

	return ctx.HTML(http.StatusOK, listHTML)
}

func (service *FrontendService) htmxRedirectOriginalByIDHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxRedirectOriginalByIDHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/original/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	imageURL, err := service.coreService.GetImageURL(ctx.Request().Context(), id, "original")
	if err != nil {
		slog.Warn("htmxRedirectOriginalByIDHandler: image not available",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Image not available")
	}

	return ctx.Redirect(http.StatusFound, imageURL)
}

func (service *FrontendService) htmxDeleteImageHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxDeleteImageHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	if err := service.coreService.DeleteImage(ctx.Request().Context(), id); err != nil {
		slog.Error("htmxDeleteImageHandler: failed to delete image",
			"status", http.StatusInternalServerError, "image_id", id, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to delete image")
	}

	// Build updated list HTML
	listHTML, err := service.buildImageListHTML(ctx.Request().Context())
	if err != nil {
		slog.Error("htmxDeleteImageHandler: failed to list images after delete",
			"status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}

	// Prevent caching so the latest state is shown
	service.setNoCache(ctx)

	// Return list HTML (to swap into #image-list)
	return ctx.HTML(http.StatusOK, listHTML)
}

func (service *FrontendService) setNoCache(ctx echo.Context) {
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")
}

func (service *FrontendService) formatNextShow(t time.Time) string {
	if !t.IsZero() && t.Unix() > 0 && t.Year() > 1 {
		return t.Format("2006-01-02")
	}
	return "unknown"
}

func (service *FrontendService) buildImageListHTML(ctx context.Context) (string, error) {
	// Render strictly in persisted DB order for deterministic Up/Down moves
	ids, err := service.coreService.GetOrderedImageIDs(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	if len(ids) == 0 {
		b.WriteString(`<p>No images uploaded yet.</p>`)
		return b.String(), nil
	}
	// compute per-position dates; top of list is today's image
	base := time.Now()

	b.WriteString(`<div class="vertical-list" id="image-sort-list">`)
	for i, id := range ids {
		showDate := base.AddDate(0, 0, i)
		nextStr := service.formatNextShow(showDate)

		imgURL, _ := service.coreService.GetImageURL(ctx, id, "original")

		fmt.Fprintf(&b, `<div class="vertical-item" data-id="%s" style="margin-bottom:1rem"><article>
	<img src="%s" alt="Original image %s" loading="lazy" style="max-width:100%%;height:auto">
	<footer style="display:flex;gap:0.5rem;align-items:center;flex-wrap:wrap">
		<small>Scheduled date: %s</small>
		<div style="display:flex;gap:0.5rem">
			<button hx-post="/htmx/image/%s/move?dir=up" hx-target="#image-list" hx-swap="innerHTML" aria-label="Move up" title="Move up">
				<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
					<polygon points="12,5 19,18 5,18" />
				</svg>
			</button>
			<button hx-post="/htmx/image/%s/move?dir=down" hx-target="#image-list" hx-swap="innerHTML" aria-label="Move down" title="Move down">
				<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
					<polygon points="5,6 19,6 12,19" />
				</svg>
			</button>
			<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
		</div>
	</footer>
</article></div>`, id, imgURL, id, nextStr, id, id, id)
	}
	b.WriteString(`</div>`)
	return b.String(), nil
}

func (service *FrontendService) htmxMoveImageHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	dir, ok := parseMoveDirection(ctx.QueryParam("dir"))
	if id == "" || !ok {
		slog.Warn("htmxMoveImageHandler: invalid params", "id", id, "dir", ctx.QueryParam("dir"))
		return ctx.String(http.StatusBadRequest, "Invalid parameters")
	}

	order, err := service.coreService.GetOrderedImageIDs(ctx.Request().Context())
	if err != nil {
		slog.Error("htmxMoveImageHandler: failed to get order", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to fetch order")
	}
	if len(order) == 0 {
		return ctx.String(http.StatusBadRequest, "No images")
	}

	idx := sliceIndex(order, id)
	if idx < 0 {
		return ctx.String(http.StatusBadRequest, "Image not found")
	}

	order = cycleMove(order, idx, dir)

	if err := service.coreService.UpdateImageOrder(ctx.Request().Context(), order); err != nil {
		slog.Error("htmxMoveImageHandler: failed to update order", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to update order")
	}

	listHTML, err := service.buildImageListHTML(ctx.Request().Context())
	if err != nil {
		slog.Error("htmxMoveImageHandler: failed to rebuild image list", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to rebuild image list")
	}

	service.setNoCache(ctx)
	return ctx.HTML(http.StatusOK, listHTML)
}

func (service *FrontendService) iconHandler(ctx echo.Context) error {
	data, err := assetsFS.ReadFile("views/icon.svg")
	if err != nil {
		slog.Error("iconHandler: failed to read icon.svg", "status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to load icon")
	}
	// Cache for 7 days
	ctx.Response().Header().Set("Cache-Control", "public, max-age=604800, immutable")
	return ctx.Blob(http.StatusOK, "image/svg+xml", data)
}
