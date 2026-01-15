package fontend

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
	e.GET("/htmx/image/original-thumb/:id", service.htmxGetOriginalThumbnailByIDHandler)
	e.DELETE("/htmx/image/:id", service.htmxDeleteImageHandler)
}

func (service *FrontendService) htmxGetCurrentImageHandler(ctx echo.Context) error {
	id, err := service.coreService.GetImageForTime(time.Now())
	if err != nil {
		// Explicit logging of error with status code and route
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
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	// Return the image data
	return ctx.Blob(http.StatusOK, "image/png", thumbnail)
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
	currentImageOOB := fmt.Sprintf(`<img id="current-image" src="/htmx/image?ts=%s" hx-swap-oob="true" alt="Current image" style="display:none" onload="this.style.display='block'; document.getElementById('no-image').style.display='none';" onerror="this.style.display='none'; document.getElementById('no-image').style.display='block';">`, ts)

	// Build out-of-band update for the image list
	ids, err := service.coreService.GetAllImageIDs()
	if err != nil {
		// If building the list fails, still return the current image update and upload result
		slog.Error("htmxUploadImageHandler: failed to list images for OOB update",
			"status", http.StatusInternalServerError, "error", err)
		html := fmt.Sprintf(`<div id="upload-result">Uploaded file: %s</div>%s`, file.Filename, currentImageOOB)
		return ctx.HTML(http.StatusOK, html)
	}
	// Build schedules for next show times
	schedules, schedErr := service.coreService.GetImageSchedules(time.Now())
	if schedErr != nil {
		slog.Warn("htmxUploadImageHandler: failed to compute schedules for OOB update", "error", schedErr)
	}
	nextShowMap := make(map[string]time.Time, len(schedules))
	for _, s := range schedules {
		nextShowMap[s.ID] = s.NextShow
	}

	var listBuilder strings.Builder
	if len(ids) == 0 {
		listBuilder.WriteString(`<p>No images uploaded yet.</p>`)
	} else {
		listBuilder.WriteString(`<div class="vertical-list">`)
		// sort by next show date ascending (soonest first)
		items := make([]struct {
			id string
			t  time.Time
		}, 0, len(ids))
		for _, id := range ids {
			t, ok := nextShowMap[id]
			if !ok {
				// push unknowns to the end
				t = time.Unix(1<<62-1, 0)
			}
			items = append(items, struct {
				id string
				t  time.Time
			}{id: id, t: t})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].t.Before(items[j].t) })

		for _, it := range items {
			nextStr := "unknown"
			if !it.t.IsZero() && it.t.Unix() > 0 && it.t.Year() > 1 {
				nextStr = it.t.Format("2006-01-02 15:04")
			}
			listBuilder.WriteString(fmt.Sprintf(`<div class="vertical-item" style="margin-bottom:1rem"><article>
	<img src="/htmx/image/original-thumb/%s?ts=%s" alt="Original thumbnail %s" style="max-width:100%%;height:auto">
	<footer>
		<small>Next shown: %s</small>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article></div>`, it.id, ts, it.id, nextStr, it.id))
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

	// Build map of next show times
	schedules, schedErr := service.coreService.GetImageSchedules(time.Now())
	if schedErr != nil {
		// Non-fatal; continue without schedule
		slog.Warn("htmxListImagesHandler: failed to compute schedules", "error", schedErr)
	}
	nextShowMap := make(map[string]time.Time, len(schedules))
	for _, s := range schedules {
		nextShowMap[s.ID] = s.NextShow
	}

	var b strings.Builder
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(ids) == 0 {
		b.WriteString(`<p>No images uploaded yet.</p>`)
	} else {
		b.WriteString(`<div class="vertical-list">`)
		// sort by next show date ascending (soonest first)
		items := make([]struct {
			id string
			t  time.Time
		}, 0, len(ids))
		for _, id := range ids {
			t, ok := nextShowMap[id]
			if !ok {
				// push unknowns to the end
				t = time.Unix(1<<62-1, 0)
			}
			items = append(items, struct {
				id string
				t  time.Time
			}{id: id, t: t})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].t.Before(items[j].t) })

		for _, it := range items {
			nextStr := "unknown"
			if !it.t.IsZero() && it.t.Unix() > 0 && it.t.Year() > 1 {
				nextStr = it.t.Format("2006-01-02 15:04")
			}
			b.WriteString(fmt.Sprintf(`<div class="vertical-item" style="margin-bottom:1rem"><article>
	<img src="/htmx/image/original-thumb/%s?ts=%s" alt="Original thumbnail %s" style="max-width:100%%;height:auto">
	<footer>
		<small>Next shown: %s</small>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article></div>`, it.id, ts, it.id, nextStr, it.id))
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

	image, err := service.coreService.GetImageById(id)
	if err != nil || len(image.OriginalImage) == 0 {
		slog.Warn("htmxGetImageByIDHandler: image not found",
			"status", http.StatusNotFound, "image_id", id, "error", err)
		return ctx.String(http.StatusNotFound, "Image not found")
	}

	// Prevent caching
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	return ctx.Blob(http.StatusOK, "image/png", image.OriginalImage)
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
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	return ctx.Blob(http.StatusOK, "image/png", thumbnail)
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

	// Build updated list HTML (same layout and sorting as list handler)
	ids, err := service.coreService.GetAllImageIDs()
	if err != nil {
		slog.Error("htmxDeleteImageHandler: failed to list images after delete",
			"status", http.StatusInternalServerError, "error", err)
		return ctx.String(http.StatusInternalServerError, "Failed to list images")
	}

	// Build map of next show times
	schedules, schedErr := service.coreService.GetImageSchedules(time.Now())
	if schedErr != nil {
		slog.Warn("htmxDeleteImageHandler: failed to compute schedules after delete", "error", schedErr)
	}
	nextShowMap := make(map[string]time.Time, len(schedules))
	for _, s := range schedules {
		nextShowMap[s.ID] = s.NextShow
	}

	var b strings.Builder
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(ids) == 0 {
		b.WriteString(`<p>No images uploaded yet.</p>`)
	} else {
		b.WriteString(`<div class="vertical-list">`)
		// sort by next show date ascending (soonest first)
		items := make([]struct {
			id string
			t  time.Time
		}, 0, len(ids))
		for _, id := range ids {
			t, ok := nextShowMap[id]
			if !ok {
				// push unknowns to the end
				t = time.Unix(1<<62-1, 0)
			}
			items = append(items, struct {
				id string
				t  time.Time
			}{id: id, t: t})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].t.Before(items[j].t) })

		for _, it := range items {
			nextStr := "unknown"
			if !it.t.IsZero() && it.t.Unix() > 0 && it.t.Year() > 1 {
				nextStr = it.t.Format("2006-01-02 15:04")
			}
			b.WriteString(fmt.Sprintf(`<div class="vertical-item" style="margin-bottom:1rem"><article>
	<img src="/htmx/image/original-thumb/%s?ts=%s" alt="Original thumbnail %s" style="max-width:100%%;height:auto">
	<footer>
		<small>Next shown: %s</small>
		<button hx-delete="/htmx/image/%s" hx-target="#image-list" hx-swap="innerHTML" class="secondary">Delete</button>
	</footer>
</article></div>`, it.id, ts, it.id, nextStr, it.id))
		}
		b.WriteString(`</div>`)
	}

	// Also refresh current image via OOB swap to reflect deletion change
	currentImageOOB := fmt.Sprintf(`<img id="current-image" src="/htmx/image?ts=%s" hx-swap-oob="true" alt="Current image" style="display:none" onload="this.style.display='block'; document.getElementById('no-image').style.display='none';" onerror="this.style.display='none'; document.getElementById('no-image').style.display='block';">`, ts)

	// Prevent caching so the latest state is shown
	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response().Header().Set("Pragma", "no-cache")
	ctx.Response().Header().Set("Expires", "0")

	// Return list HTML (to swap into #image-list) plus OOB update for current image
	return ctx.HTML(http.StatusOK, b.String()+currentImageOOB)
}
