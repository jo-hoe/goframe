package frontend

import (
	"context"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/core"
	"github.com/jo-hoe/goframe/internal/database"
	"github.com/jo-hoe/goframe/internal/imagevalidation"
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
	e.GET("/htmx/uploadStatus/:id", service.htmxUploadStatusHandler)

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
	form, err := ctx.MultipartForm()
	if err != nil {
		slog.Error("htmxUploadImageHandler: failed to parse multipart form", "error", err)
		return ctx.HTML(http.StatusBadRequest, `<div class="error">Failed to parse upload form</div>`)
	}
	defer func() { _ = form.RemoveAll() }()

	fileHeaders := form.File["image"]
	if len(fileHeaders) == 0 {
		return ctx.HTML(http.StatusBadRequest, `<div class="error">No files provided</div>`)
	}

	var rows strings.Builder
	// Track the highest status code: 202 if any file was accepted for processing,
	// 200 if all were already succeeded, 400 if all failed validation.
	statusCode := http.StatusOK

	for _, fh := range fileHeaders {
		src, err := fh.Open()
		if err != nil {
			slog.Error("htmxUploadImageHandler: failed to open file", "filename", fh.Filename, "error", err)
			fmt.Fprintf(&rows, `<div class="error">Failed to open %s</div>`, html.EscapeString(fh.Filename))
			if statusCode == http.StatusOK {
				statusCode = http.StatusBadRequest
			}
			continue
		}

		image, readErr := io.ReadAll(src)
		_ = src.Close()
		if readErr != nil {
			slog.Error("htmxUploadImageHandler: failed to read file", "filename", fh.Filename, "error", readErr)
			fmt.Fprintf(&rows, `<div class="error">Failed to read %s</div>`, html.EscapeString(fh.Filename))
			if statusCode == http.StatusOK {
				statusCode = http.StatusBadRequest
			}
			continue
		}

		state, submitErr := service.coreService.SubmitImage(ctx.Request().Context(), image, "")
		if submitErr != nil {
			if verr, ok := imagevalidation.AsValidationError(submitErr); ok {
				slog.Info("htmxUploadImageHandler: rejected invalid upload", "reason", verr.Reason, "filename", fh.Filename)
				fmt.Fprintf(&rows, `<div class="error">%s: %s</div>`, html.EscapeString(fh.Filename), html.EscapeString(verr.Reason))
				if statusCode == http.StatusOK {
					statusCode = http.StatusBadRequest
				}
			} else {
				slog.Error("htmxUploadImageHandler: failed to submit image", "filename", fh.Filename, "error", submitErr)
				fmt.Fprintf(&rows, `<div class="error">Failed to submit %s</div>`, html.EscapeString(fh.Filename))
				if statusCode == http.StatusOK {
					statusCode = http.StatusInternalServerError
				}
			}
			continue
		}

		if state.Status == database.StatusProcessing {
			statusCode = http.StatusAccepted
		}
		rows.WriteString(service.renderUploadStatusFragment(ctx.Request().Context(), state))
	}

	service.setNoCache(ctx)
	return ctx.HTML(statusCode, rows.String())
}

func (service *FrontendService) htmxUploadStatusHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	state, err := service.coreService.GetUploadState(ctx.Request().Context(), id)
	if err != nil {
		slog.Info("htmxUploadStatusHandler: invalid image id", "image_id", id, "error", err)
		return ctx.HTML(http.StatusBadRequest, fmt.Sprintf(`<div id="upload-result-%s" class="error">Invalid image id</div>`, html.EscapeString(id)))
	}
	service.setNoCache(ctx)
	return ctx.HTML(http.StatusOK, service.renderUploadStatusFragment(ctx.Request().Context(), state))
}

// renderUploadStatusFragment renders a per-upload status row for the given
// derived state. Each row is uniquely identified by "upload-result-<id>" so
// multiple in-flight uploads can coexist without replacing each other.
// While processing the row keeps self-polling; on success it stops polling and
// emits an out-of-band refresh of the image list; on failure it shows the error.
func (service *FrontendService) renderUploadStatusFragment(ctx context.Context, state *database.UploadState) string {
	elemID := "upload-result-" + html.EscapeString(state.ID)
	switch state.Status {
	case database.StatusSucceeded:
		listHTML, err := service.buildImageListHTML(ctx)
		if err != nil {
			slog.Error("renderUploadStatusFragment: failed to build image list", "error", err)
			return fmt.Sprintf(`<div id="%s" class="success">Upload successful.</div>`, elemID)
		}
		oob := fmt.Sprintf(`<div id="image-list" hx-swap-oob="true">%s</div>`, listHTML)
		return fmt.Sprintf(`<div id="%s" class="success">Upload successful.</div>`, elemID) + oob
	case database.StatusFailed:
		return fmt.Sprintf(`<div id="%s" class="error">Processing failed: %s</div>`, elemID, html.EscapeString(state.Error))
	default:
		// pending / processing / unknown: keep polling.
		return fmt.Sprintf(
			`<div id="%s" hx-get="/htmx/uploadStatus/%s" hx-trigger="load delay:2s" hx-swap="outerHTML">`+
				`<span class="loading-spinner" aria-hidden="true"></span> Upload received — processing…</div>`,
			elemID, html.EscapeString(state.ID),
		)
	}
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
