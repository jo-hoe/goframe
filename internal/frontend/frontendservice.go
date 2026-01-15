package frontend

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/jo-hoe/goframe/internal/backend/commands"
	"github.com/jo-hoe/goframe/internal/core"
	"github.com/labstack/echo/v4"
)

const (
	defaultThumbnailWidth = 128
	MainPageName          = "index.html"
	mimePNG               = "image/png"
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
	e.GET("/htmx/image", service.htmxGetCurrentImageHandler)

	// Routes for listing, fetching by ID, and deleting images
	e.GET("/htmx/images", service.htmxListImagesHandler)
	e.GET("/htmx/image/:id", service.htmxGetImageByIDHandler)
	e.GET("/htmx/image/original-thumb/:id", service.htmxGetOriginalThumbnailByIDHandler)
	e.DELETE("/htmx/image/:id", service.htmxDeleteImageHandler)
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

	// Build out-of-band update for the current image
	currentImageOOB := service.buildCurrentImageOOB(ts)

	// Build out-of-band update for the image list
	imageListHTML, listErr := service.buildImageListHTML(ts)
	if listErr != nil {
		// If building the list fails, still return the current image update and upload result
		slog.Error("htmxUploadImageHandler: failed to list images for OOB update",
			"status", http.StatusInternalServerError, "error", listErr)
		html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s`, file.Filename, currentImageOOB)
		return ctx.HTML(http.StatusOK, html)
	}
	imageListOOB := fmt.Sprintf(`<div id="image-list" hx-swap-oob="true">%s</div>`, imageListHTML)

	// Return combined HTML with OOB swaps for current image and image list
	html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s%s`, file.Filename, currentImageOOB, imageListOOB)
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

func (service *FrontendService) htmxGetCurrentImageHandler(ctx echo.Context) error {
	id, err := service.coreService.GetImageForTime(time.Now())
	if err != nil {
		slog.Warn("htmxGetCurrentImageHandler: failed to get image for current time",
			"status", http.StatusNotFound,
			"route", "/htmx/image",
			"error", err)
		return ctx.String(http.StatusNotFound, "No image available for current time")
	}

	image, err := service.coreService.GetImageById(id)
	if err != nil || len(image.OriginalImage) == 0 {
		slog.Warn("htmxGetCurrentImageHandler: image not available",
			"status", http.StatusNotFound,
			"route", "/htmx/image",
			"image_id", id,
			"error", err)
		return ctx.String(http.StatusNotFound, "Image not available")
	}

	thumbnail, err := toThumbnail(image.OriginalImage)
	if err != nil || len(thumbnail) == 0 {
		slog.Warn("htmxGetCurrentImageHandler: thumbnail not available",
			"status", http.StatusNotFound,
			"route", "/htmx/image",
			"error", err)
		return ctx.String(http.StatusNotFound, "Thumbnail not available")
	}

	// Prevent caching so the latest uploaded image is always shown
	service.setNoCache(ctx)

	// Return the image data
	return ctx.Blob(http.StatusOK, mimePNG, thumbnail)
}

func (service *FrontendService) htmxGetImageByIDHandler(ctx echo.Context) error {
	id := ctx.Param("id")
	if id == "" {
		slog.Warn("htmxGetImageByIDHandler: missing image id",
			"status", http.StatusBadRequest,
			"route", "/htmx/image/:id")
		return ctx.String(http.StatusBadRequest, "Missing image ID")
	}

	image, err := service.coreService.GetImageById(id)
	if err != nil || len(image.OriginalImage) == 0 {
		slog.Warn("htmxGetImageByIDHandler: image not found",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Image not found")
	}

	// Prevent caching
	service.setNoCache(ctx)

	return ctx.Blob(http.StatusOK, mimePNG, image.OriginalImage)
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
	thumbnail, err := toThumbnail(image.OriginalImage)
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

	// Also refresh current image via OOB swap to reflect deletion change
	currentImageOOB := service.buildCurrentImageOOB(ts)

	// Prevent caching so the latest state is shown
	service.setNoCache(ctx)

	// Return list HTML (to swap into #image-list) plus OOB update for current image
	return ctx.HTML(http.StatusOK, listHTML+currentImageOOB)
}

func toThumbnail(image []byte) ([]byte, error) {
	command, err := commands.NewPixelScaleCommand(map[string]any{"width": defaultThumbnailWidth})
	if err != nil {
		return nil, fmt.Errorf("failed to create thumbnail command: %w", err)
	}
	thumbnail, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail: %w", err)
	}
	return thumbnail, nil
}

// Helpers

func (service *FrontendService) setNoCache(ctx echo.Context) {
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")
}

func (service *FrontendService) timestampNanoStr() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (service *FrontendService) buildCurrentImageOOB(ts string) string {
	return fmt.Sprintf(`<img id="current-image" src="/htmx/image?ts=%s" hx-swap-oob="true" alt="Current image" style="display:none" onload="this.style.display='block'; document.getElementById('no-image').style.display='none';" onerror="this.style.display='none'; document.getElementById('no-image').style.display='block';">`, ts)
}

type imageItem struct {
	id   string
	next time.Time
}

func (service *FrontendService) getSortedImageItems(now time.Time) ([]imageItem, error) {
	ids, err := service.coreService.GetAllImageIDs()
	if err != nil {
		return nil, err
	}

	// Build map of next show times; non-fatal when schedules fail
	schedules, schedErr := service.coreService.GetImageSchedules(now)
	if schedErr != nil {
		slog.Warn("getSortedImageItems: failed to compute schedules", "error", schedErr)
	}
	nextShowMap := make(map[string]time.Time, len(schedules))
	for _, s := range schedules {
		nextShowMap[s.ID] = s.NextShow
	}

	// sort by next show date ascending (soonest first)
	items := make([]imageItem, 0, len(ids))
	farFuture := time.Unix(1<<62-1, 0) // push unknowns to the end
	for _, id := range ids {
		t, ok := nextShowMap[id]
		if !ok {
			t = farFuture
		}
		items = append(items, imageItem{id: id, next: t})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].next.Before(items[j].next) })

	return items, nil
}

func (service *FrontendService) formatNextShow(t time.Time) string {
	if !t.IsZero() && t.Unix() > 0 && t.Year() > 1 {
		return t.Format("2006-01-02 15:04")
	}
	return "unknown"
}

func (service *FrontendService) buildImageListHTML(ts string) (string, error) {
	items, err := service.getSortedImageItems(time.Now())
	if err != nil {
		return "", err
	}

	var b strings.Builder
	if len(items) == 0 {
		b.WriteString(`<p>No images uploaded yet.</p>`)
		return b.String(), nil
	}

	b.WriteString(`<div class="vertical-list">`)
	for _, it := range items {
		nextStr := service.formatNextShow(it.next)
		b.WriteString(fmt.Sprintf(`<div class="vertical-item" style="margin-bottom:1rem"><article>
	<img src="/htmx/image/original-thumb/%s?ts=%s" alt="Original thumbnail %s" style="max-width:100%%;height:auto">
	<footer>
		<small>Next shown: %s</small>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article></div>`, it.id, ts, it.id, nextStr, it.id))
	}
	b.WriteString(`</div>`)
	return b.String(), nil
}
