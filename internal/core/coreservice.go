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

	mu      sync.Mutex
	pointer int
	lastDay time.Time
}

func NewCoreService(config *ServiceConfig) *CoreService {
	databaseService, err := getDatabaseService(config)
	if err != nil {
		panic(err)
	}
	return &CoreService{
		config:          config,
		databaseService: databaseService,
		pointer:         0,
		lastDay:         time.Time{},
	}
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	slog.Info("CoreService.AddImage: start", "bytes", len(image))

	command, err := commands.NewPngConverterCommand(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}

	// default PNG conversion
	convertedImageData, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}
	// apply all configured commands
	processedImage, err := service.applyConfiguredCommands(convertedImageData)
	if err != nil {
		return nil, fmt.Errorf("failed to apply configured commands: %w", err)
	}

	// Insert atomically with processed image to avoid NULL windows
	databaseImageID, err := service.databaseService.CreateImage(convertedImageData, processedImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}

	databaseImage := &common.ApiImage{
		ID: databaseImageID,
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

func (service *CoreService) applyConfiguredCommands(image []byte) (processedImage []byte, err error) {
	if image == nil {
		return nil, fmt.Errorf("input image is nil")
	}

	// If no commands are configured, return the original image unchanged
	if service == nil || service.config == nil || len(service.config.Commands) == 0 {
		slog.Debug("CoreService.applyConfiguredCommands: no commands configured, returning original image", "bytes", len(image))
		return image, nil
	}

	// Map core.CommandConfig to commandstructure.CommandConfig
	commandConfigs := make([]commandstructure.CommandConfig, 0, len(service.config.Commands))
	for _, cfg := range service.config.Commands {
		commandConfigs = append(commandConfigs, commandstructure.CommandConfig{
			Name:   cfg.Name,
			Params: cfg.Params,
		})
	}

	slog.Info("CoreService.applyConfiguredCommands: executing configured commands", "count", len(commandConfigs), "input_size_bytes", len(image))
	out, execErr := commandstructure.ExecuteCommands(image, commandConfigs)
	if execErr != nil {
		return nil, fmt.Errorf("failed to apply configured commands: %w", execErr)
	}
	return out, nil
}

func getDatabaseService(DatabaseConfig *ServiceConfig) (database.DatabaseService, error) {
	databaseService, err := database.NewDatabase(DatabaseConfig.Database.Type, DatabaseConfig.Database.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Info("database initialized successfully", "type", DatabaseConfig.Database.Type)
	// Additional core service startup logic can be added here
	return databaseService, nil
}

func (service *CoreService) GetAllImageIDs() ([]string, error) {
	images, err := service.databaseService.GetImages()
	if err != nil {
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}

	// Only include images that have a processed image available
	ids := make([]string, 0, len(images))
	for _, img := range images {
		if len(img.ProcessedImage) > 0 {
			ids = append(ids, img.ID)
		}
	}
	slog.Info("CoreService.GetAllImageIDs: fetched image IDs", "count", len(ids))
	return ids, nil
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
	loc, err := time.LoadLocation(service.config.RotationTimezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rotation timezone; defaulting to UTC", "tz", service.config.RotationTimezone, "err", err)
		loc = time.UTC
	}
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
			service.pointer = (service.pointer + days) % n
		}
		service.lastDay = todayMid
	}
}

// ImageSchedule represents when an image will be shown next according to rotation rules.
type ImageSchedule struct {
	ID       string
	NextShow time.Time
}

func (service *CoreService) GetImageForTime(now time.Time) (string, error) {
	// Fetch ascending by rank; SQLite GetImages orders by rank ASC, rowid ASC
	images, err := service.databaseService.GetImages("id")
	if err != nil {
		return "", fmt.Errorf("failed to fetch images: %w", err)
	}
	n := len(images)
	if n == 0 {
		return "", fmt.Errorf("no images")
	}

	// Advance the in-memory pointer if a new day started
	service.advancePointer(now, n)

	// LIFO: newest first. Since images is ascending, pick from end.
	service.mu.Lock()
	idx := service.pointer % n
	service.mu.Unlock()

	indexFromEnd := n - 1 - idx
	return images[indexFromEnd].ID, nil
}

// GetImageSchedules returns, for each image, the next time
// it will be shown according to the same rotation logic used by selectImageForTime.
// The NextShow is aligned to 00:00 of the rotation timezone for the respective day.
func (service *CoreService) GetImageSchedules(date time.Time) ([]ImageSchedule, error) {
	// Fetch ascending by rank; SQLite GetImages orders by rank ASC, rowid ASC
	images, err := service.databaseService.GetImages("id")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch images: %w", err)
	}

	n := len(images)
	if n == 0 {
		return []ImageSchedule{}, nil
	}

	loc := service.loadRotationLocation()
	dateMid := service.dayStart(date, loc)

	// Snapshot baseline state
	service.mu.Lock()
	basePointer := service.pointer
	baseDay := service.lastDay
	service.mu.Unlock()

	// If not initialized yet, assume baseline is the provided date
	if baseDay.IsZero() {
		baseDay = dateMid
	}

	// Compute forward days from baseline to the requested date
	daysForward := 0
	if !dateMid.Before(baseDay) {
		daysForward = int(dateMid.Sub(baseDay).Hours() / 24.0)
	}

	// Pointer position on the requested date
	pointerAtDate := basePointer
	if n > 0 && daysForward > 0 {
		pointerAtDate = (basePointer + daysForward) % n
	}

	schedules := make([]ImageSchedule, 0, n)
	for j, img := range images {
		// Newest-first index selection
		targetIdx := n - 1 - j
		daysUntil := (targetIdx - pointerAtDate) % n
		if daysUntil < 0 {
			daysUntil += n
		}
		// If already selected on the requested date, schedule for the next cycle
		if daysUntil == 0 {
			daysUntil = n
		}
		nextShow := dateMid.Add(time.Duration(daysUntil) * 24 * time.Hour)
		schedules = append(schedules, ImageSchedule{
			ID:       img.ID,
			NextShow: nextShow,
		})
	}
	return schedules, nil
}
