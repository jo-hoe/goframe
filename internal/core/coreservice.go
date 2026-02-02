package core

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jo-hoe/goframe/internal/backend/commands"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"github.com/jo-hoe/goframe/internal/backend/database"
	"github.com/jo-hoe/goframe/internal/common"
)

type CoreService struct {
	config          *ServiceConfig
	databaseService database.DatabaseService
	commandConfigs  []commandstructure.CommandConfig
	tzLoc           *time.Location

	mu      sync.Mutex
	pointer int
	lastDay time.Time
}

func NewCoreService(config *ServiceConfig) *CoreService {
	db, err := database.NewDatabase(config.Database.Type, config.Database.ConnectionString)
	if err != nil {
		panic(err)
	}

	// Precompute command configs
	cmdCfgs := make([]commandstructure.CommandConfig, 0, len(config.Commands))
	for _, cfg := range config.Commands {
		cmdCfgs = append(cmdCfgs, commandstructure.CommandConfig{
			Name:   cfg.Name,
			Params: cfg.Params,
		})
	}

	// Cache rotation timezone location
	loc, err := time.LoadLocation(config.RotationTimezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rotation timezone; defaulting to UTC", "tz", config.RotationTimezone, "err", err)
		loc = time.UTC
	}

	return &CoreService{
		config:          config,
		databaseService: db,
		commandConfigs:  cmdCfgs,
		tzLoc:           loc,
		pointer:         0,
		lastDay:         time.Time{},
	}
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	slog.Info("CoreService.AddImage: start", "bytes", len(image))

	convertedImageData, processedImage, err := service.applyPipeline(image)
	if err != nil {
		return nil, err
	}

	// Insert atomically with processed image to avoid NULL windows
	databaseImageID, err := service.databaseService.CreateImage(convertedImageData, processedImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}

	databaseImage := &common.ApiImage{
		ID: databaseImageID,
	}

	// Re-rank the newly inserted image directly after the current image (image of the day)
	// in the persisted order so it will be shown next.
	order, err := service.getOrderedImageIDs()
	if err != nil {
		slog.Warn("CoreService.AddImage: failed to fetch order after insert", "err", err)
		return databaseImage, nil
	}
	// If there was at least one image before this insert, place the new one right after the current (index 0)
	if len(order) >= 2 {
		currentID := order[0]
		newOrder := make([]string, 0, len(order))
		newOrder = append(newOrder, currentID, databaseImageID)
		for _, id := range order {
			if id != currentID && id != databaseImageID {
				newOrder = append(newOrder, id)
			}
		}
		if err := service.UpdateImageOrder(newOrder); err != nil {
			slog.Warn("CoreService.AddImage: failed to position new image after current", "err", err)
		}
	}

	return databaseImage, nil
}

func (service *CoreService) GetImageById(id string) (*database.Image, error) {
	image, err := service.databaseService.GetImageByID(id)
	if err != nil {
		return nil, err
	}
	return image, nil
}

func (service *CoreService) applyPipeline(image []byte) (converted []byte, processed []byte, err error) {
	if image == nil {
		return nil, nil, fmt.Errorf("input image is nil")
	}

	// Always convert to PNG first. Provide SVG fallback size only for SVGs without declared size.
	params := map[string]any{}
	if service.config.SvgFallbackWidth > 0 && service.config.SvgFallbackHeight > 0 {
		params["svgFallbackWidth"] = service.config.SvgFallbackWidth
		params["svgFallbackHeight"] = service.config.SvgFallbackHeight
	}
	pngCmd, err := commands.NewPngConverterCommand(params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}
	convertedImageData, err := pngCmd.Execute(image)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}

	// Apply configured commands (if any)
	if len(service.commandConfigs) == 0 {
		slog.Debug("CoreService.applyPipeline: no commands configured, returning converted image", "bytes", len(convertedImageData))
		return convertedImageData, convertedImageData, nil
	}

	slog.Info("CoreService.applyPipeline: executing configured commands", "count", len(service.commandConfigs), "input_size_bytes", len(convertedImageData))
	out, execErr := commandstructure.ExecuteCommands(convertedImageData, service.commandConfigs)
	if execErr != nil {
		return nil, nil, fmt.Errorf("failed to apply configured commands: %w", execErr)
	}
	return convertedImageData, out, nil
}

func (service *CoreService) DeleteImage(id string) error {
	slog.Info("CoreService.DeleteImage: deleting image", "id", id)
	return service.databaseService.DeleteImage(id)
}

// Close gracefully closes underlying resources (e.g., database connections)
func (service *CoreService) Close() error {
	slog.Info("CoreService.Close: closing resources")
	return service.databaseService.Close()
}

// loadRotationLocation loads the configured timezone or falls back to UTC.
func (service *CoreService) loadRotationLocation() *time.Location {
	// Use cached location if available
	if service.tzLoc != nil {
		return service.tzLoc
	}
	loc, err := time.LoadLocation(service.config.RotationTimezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rotation timezone; defaulting to UTC", "tz", service.config.RotationTimezone, "err", err)
		loc = time.UTC
	}
	service.tzLoc = loc
	return loc
}

// dayStart returns 00:00 in the rotation timezone for the given time's calendar day.
func (service *CoreService) dayStart(t time.Time, loc *time.Location) time.Time {
	tt := t.In(loc)
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, loc)
}

// advancePointer moves the in-memory pointer forward by the number of days
// elapsed since the last recorded day in the rotation timezone. It does not move backwards.
func (service *CoreService) advancePointer(now time.Time, n int) {
	// Rotate persisted order forward by whole days at midnight so that index 0 is always "today".
	loc := service.loadRotationLocation()
	todayMid := service.dayStart(now, loc)

	service.mu.Lock()
	defer service.mu.Unlock()

	// Initialize baseline day on first use
	if service.lastDay.IsZero() {
		service.lastDay = todayMid
		return
	}

	// Advance only when a new day has begun in the rotation timezone
	if todayMid.After(service.lastDay) {
		days := int(todayMid.Sub(service.lastDay).Hours() / 24.0)
		if days > 0 && n > 0 {
			// Compute rotation by k positions (left shift): today's should become previous "tomorrow"
			k := days % n

			ids, err := service.getOrderedImageIDs()
			if err == nil && len(ids) == n {
				// Rotate left by k: new order = ids[k:] + ids[:k]
				newOrder := append([]string{}, ids[k:]...)
				newOrder = append(newOrder, ids[:k]...)
				_ = service.databaseService.UpdateRanks(newOrder)
			}
		}
		service.lastDay = todayMid
	}
}

func (service *CoreService) getOrderedImageIDs() ([]string, error) {
	images, err := service.databaseService.GetImages("id")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch images: %w", err)
	}
	ids := make([]string, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.ID)
	}
	return ids, nil
}

// GetOrderedImageIDs exposes the persisted order of images (ascending by rank).
func (service *CoreService) GetOrderedImageIDs() ([]string, error) {
	return service.getOrderedImageIDs()
}

func (service *CoreService) GetImageForTime(now time.Time) (string, error) {
	// Ensure persisted order reflects the day change so index 0 is "today"
	// First fetch count to compute rotation
	ids, err := service.getOrderedImageIDs()
	if err != nil {
		return "", err
	}
	n := len(ids)
	if n == 0 {
		return "", fmt.Errorf("no images")
	}

	// Rotate persisted order if a new day started
	service.advancePointer(now, n)

	// Re-fetch order after potential rotation
	ids, err = service.getOrderedImageIDs()
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no images")
	}

	// Index 0 is today's image
	return ids[0], nil
}

// UpdateImageOrder updates the persistent order (LexoRanks) to match the given list of IDs,
// attempting to preserve the currently selected image by adjusting the in-memory pointer.
func (service *CoreService) UpdateImageOrder(order []string) error {
	if len(order) == 0 {
		return nil
	}

	if err := service.databaseService.UpdateRanks(order); err != nil {
		return err
	}

	n := len(order)
	if n == 0 {
		return nil
	}

	// After re-ranking, make the top-of-list (index 0) be today's image.
	// This ensures moving an item to the top immediately makes it today's image as returned by /api/image.png.
	service.mu.Lock()
	service.pointer = 0
	service.mu.Unlock()

	return nil
}
