package core

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jo-hoe/goframe/internal/backend/commands"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"github.com/jo-hoe/goframe/internal/backend/database"
	"github.com/jo-hoe/goframe/internal/common"
)

type CoreService struct {
	config          *ServiceConfig
	databaseService database.DatabaseService
}

func NewCoreService(config *ServiceConfig) *CoreService {
	databaseService, err := getDatabaseService(config)
	if err != nil {
		panic(err)
	}
	return &CoreService{
		config:          config,
		databaseService: databaseService,
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

func (service *CoreService) GetImageForTime(now time.Time) (string, error) {
	cycle := service.computeDayCycle(now)

	// Fetch ascending by created_at; SQLite GetImages orders by created_at ASC, rowid ASC
	images, err := service.databaseService.GetImages("id", "processed_image")
	if err != nil {
		return "", fmt.Errorf("failed to fetch images: %w", err)
	}
	// Filter eligible (processed_image non-empty)
	eligible := make([]database.Image, 0, len(images))
	for _, img := range images {
		if img != nil && len(img.ProcessedImage) > 0 {
			eligible = append(eligible, *img)
		}
	}
	n := len(eligible)
	if n == 0 {
		return "", fmt.Errorf("no eligible images")
	}

	// LIFO: newest first. Since eligible is ascending, pick from end.
	idx := cycle.cyclePosition(n)
	indexFromEnd := n - 1 - idx
	return eligible[indexFromEnd].ID, nil
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

// dayCycle encapsulates rotation timezone, anchor, and day index computations.
type dayCycle struct {
	loc    *time.Location
	anchor time.Time
	index  int
}

// computeDayCycle centralizes all day-related calculations for rotation logic.
// It loads the configured timezone (fallback UTC), clamps the provided time to the anchor,
// and computes the day index since the anchor.
func (service *CoreService) computeDayCycle(t time.Time) dayCycle {
	loc, err := time.LoadLocation(service.config.RotationTimezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rotation timezone; defaulting to UTC", "tz", service.config.RotationTimezone, "err", err)
		loc = time.UTC
	}
	tzTime := t.In(loc)
	anchor := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)
	if tzTime.Before(anchor) {
		tzTime = anchor
	}
	days := int(tzTime.Sub(anchor).Hours() / 24.0)
	return dayCycle{loc: loc, anchor: anchor, index: days}
}

// cyclePosition returns the current position within a cycle of length n based on the day index.
func (dc dayCycle) cyclePosition(n int) int {
	if n <= 0 {
		return 0
	}
	m := dc.index % n
	if m < 0 {
		m += n
	}
	return m
}

// forwardSteps returns the number of days to move forward from curPos to reach targetPos
// within a cycle of length cycleLen. If already at targetPos, it returns a full cycleLen.
func (dc dayCycle) forwardSteps(curPos, targetPos, cycleLen int) int {
	if cycleLen <= 0 {
		return 0
	}
	diff := (targetPos - curPos) % cycleLen
	if diff < 0 {
		diff += cycleLen
	}
	if diff == 0 {
		return cycleLen
	}
	return diff
}

// dayStart returns the start time (00:00) in the rotation timezone for the provided day index.
func (dc dayCycle) dayStart(idx int) time.Time {
	return dc.anchor.Add(time.Duration(idx*24) * time.Hour).In(dc.loc)
}

// ImageSchedule represents when an image will be shown next according to rotation rules.
type ImageSchedule struct {
	ID       string
	NextShow time.Time
}

func (service *CoreService) GetImageById(id string) (*database.Image, error) {
	image, err := service.databaseService.GetImageByID(id)
	if err != nil {
		return nil, err
	}
	return image, nil
}

// GetImageSchedules returns, for each eligible image (processed image present), the next time
// it will be shown according to the same rotation logic used by selectImageForTime.
// The NextShow is aligned to 00:00 of the rotation timezone for the respective day.
func (service *CoreService) GetImageSchedules(date time.Time) ([]ImageSchedule, error) {
	cycle := service.computeDayCycle(date)

	// Fetch ascending by created_at; SQLite GetImages orders by created_at ASC, rowid ASC
	images, err := service.databaseService.GetImages("id", "processed_image")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch images: %w", err)
	}

	// Filter eligible (processed_image non-empty)
	eligible := make([]database.Image, 0, len(images))
	for _, img := range images {
		if img != nil && len(img.ProcessedImage) > 0 {
			eligible = append(eligible, *img)
		}
	}
	n := len(eligible)
	if n == 0 {
		return []ImageSchedule{}, nil
	}

	curMod := cycle.cyclePosition(n)
	schedules := make([]ImageSchedule, 0, n)
	for j, img := range eligible {
		// selectImageForTime picks indexFromEnd = n - 1 - idx where idx = days % n
		// For given image position j in ascending order, it is selected when idx == n - 1 - j
		targetIdx := n - 1 - j
		delta := cycle.forwardSteps(curMod, targetIdx, n)
		nextDayIndex := cycle.index + delta
		nextShow := cycle.dayStart(nextDayIndex)
		schedules = append(schedules, ImageSchedule{
			ID:       img.ID,
			NextShow: nextShow,
		})
	}
	return schedules, nil
}
