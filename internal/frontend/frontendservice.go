package frontend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/backend/commands"
	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
)

const (
	MainPageName = "index.html"
	mimePNG      = "image/png"
)

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
	// Create template renderer
	e.Renderer = &Template{
		templates: template.Must(template.New("").ParseFS(templateFS, viewsPattern)),
	}

	e.GET("/", service.rootRedirectHandler) // Redirect root to index.html
	e.GET("/"+MainPageName, service.indexHandler)
	e.POST("/htmx/uploadImage", service.htmxUploadImageHandler)

	// Routes for listing, fetching by ID, and deleting images
	e.GET("/htmx/images", service.htmxListImagesHandler)
	e.GET("/htmx/image/original-thumb/:id", service.htmxGetOriginalThumbnailByIDHandler)
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

	_, err = service.coreService.AddImage(image)
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to process uploaded image",
			"status", http.StatusInternalServerError, "error", err, "filename", file.Filename)
		return ctx.String(http.StatusInternalServerError, "Failed to process uploaded image")
	}

	// Return an out-of-band swap to refresh the displayed image, plus a simple status message
	ts := service.timestampNanoStr()

	// Build out-of-band update for the image list
	imageListHTML, listErr := service.buildImageListHTML(ts)
	if listErr != nil {
		// If building the list fails, still return the current image update and upload result
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
	listHTML, err := service.buildImageListHTML(service.timestampNanoStr())
	if err != nil {
		slog.Error("htmxListImagesHandler: failed to list images",
			"status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}

	// Prevent caching so the latest images are always shown
	service.setNoCache(ctx)

	return ctx.HTML(http.StatusOK, listHTML)
}

func (service *FrontendService) htmxGetOriginalThumbnailByIDHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxGetOriginalThumbnailByIDHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/original-thumb/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	image, err := service.coreService.GetImageById(id)
	if err != nil || len(image.OriginalImage) == 0 {
		slog.Warn("htmxGetOriginalThumbnailByIDHandler: image not available",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Image not available")
	}
	thumbnail, err := service.toThumbnail(image.OriginalImage)
	if err != nil || len(thumbnail) == 0 {
		slog.Warn("htmxGetOriginalThumbnailByIDHandler: thumbnail not available",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Thumbnail not available")
	}

	// Prevent caching
	service.setNoCache(ctx)

	return ctx.Blob(http.StatusOK, mimePNG, thumbnail)
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

	// Build updated list HTML
	ts := service.timestampNanoStr()
	listHTML, err := service.buildImageListHTML(ts)
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

func (service *FrontendService) toThumbnail(image []byte) ([]byte, error) {
	width := service.config.ThumbnailWidth
	command, err := commands.NewPixelScaleCommand(map[string]any{"width": width})
	if err != nil {
		return nil, fmt.Errorf("failed to create thumbnail command: %w", err)
	}
	thumbnail, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail: %w", err)
	}
	return thumbnail, nil
}

func (service *FrontendService) setNoCache(ctx echo.Context) {
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")
}

func (service *FrontendService) timestampNanoStr() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (service *FrontendService) formatNextShow(t time.Time) string {
	if !t.IsZero() && t.Unix() > 0 && t.Year() > 1 {
		return t.Format("2006-01-02")
	}
	return "unknown"
}

func (service *FrontendService) buildImageListHTML(ts string) (string, error) {
	// Render strictly in persisted DB order for deterministic Up/Down moves
	ids, err := service.coreService.GetOrderedImageIDs()
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
		// Controls: Up disabled for first, Down disabled for last
		disableUp := ""
		disableDown := ""
		if i == 0 {
			disableUp = " disabled"
		}
		if i == len(ids)-1 {
			disableDown = " disabled"
		}

		b.WriteString(fmt.Sprintf(`<div class="vertical-item" data-id="%s" style="margin-bottom:1rem"><article>
	<img src="/htmx/image/original-thumb/%s?ts=%s" alt="Original thumbnail %s" style="max-width:100%%;height:auto">
	<footer style="display:flex;gap:0.5rem;align-items:center;flex-wrap:wrap">
		<small>Scheduled date: %s</small>
		<div style="display:flex;gap:0.5rem">
			<button hx-post="/htmx/image/%s/move?dir=up" hx-target="#image-list" hx-swap="innerHTML"%s aria-label="Move up" title="Move up">
				<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
					<polygon points="12,5 19,18 5,18" />
				</svg>
			</button>
			<button hx-post="/htmx/image/%s/move?dir=down" hx-target="#image-list" hx-swap="innerHTML"%s aria-label="Move down" title="Move down">
				<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
					<polygon points="5,6 19,6 12,19" />
				</svg>
			</button>
			<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
		</div>
	</footer>
</article></div>`, id, id, ts, id, nextStr, id, disableUp, id, disableDown, id))
	}
	b.WriteString(`</div>`)
	return b.String(), nil
}

func (service *FrontendService) htmxMoveImageHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	dir := strings.ToLower(strings.TrimSpace(ctx.QueryParam("dir")))
	if id == "" || (dir != "up" && dir != "down") {
		slog.Warn("htmxMoveImageHandler: invalid params", "id", id, "dir", dir)
		return ctx.String(http.StatusBadRequest, "Invalid parameters")
	}

	// Get current order from DB
	order, err := service.coreService.GetOrderedImageIDs()
	if err != nil {
		slog.Error("htmxMoveImageHandler: failed to get order", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to fetch order")
	}
	if len(order) == 0 {
		return ctx.String(http.StatusBadRequest, "No images")
	}

	// Find index
	idx := -1
	for i := range order {
		if order[i] == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ctx.String(http.StatusBadRequest, "Image not found")
	}

	// Compute target index and swap
	switch dir {
	case "up":
		if idx > 0 {
			order[idx], order[idx-1] = order[idx-1], order[idx]
		}
	case "down":
		if idx < len(order)-1 {
			order[idx], order[idx+1] = order[idx+1], order[idx]
		}
	}

	if err := service.coreService.UpdateImageOrder(order); err != nil {
		slog.Error("htmxMoveImageHandler: failed to update order", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to update order")
	}

	// Rebuild list
	ts := service.timestampNanoStr()
	listHTML, err := service.buildImageListHTML(ts)
	if err != nil {
		slog.Error("htmxMoveImageHandler: failed to rebuild image list", "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to rebuild image list")
	}

	// Prevent caching
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
