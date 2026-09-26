package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/database"
	"github.com/jo-hoe/goframe/internal/imageprocessing"
	"github.com/jo-hoe/goframe/internal/imagevalidation"
)

// processTimeout bounds how long a single background image-processing job may run.
const processTimeout = 5 * time.Minute

// maxConcurrentProcessing bounds how many uploads are processed concurrently so
// that a burst of uploads cannot exhaust memory or CPU.
const maxConcurrentProcessing = 4

// CoreService is the central business logic layer for the goframe server.
type CoreService struct {
	config          *config.ServiceConfig
	databaseService database.DatabaseService
	commandConfigs  []imageprocessing.CommandConfig
	tzLoc           *time.Location

	// sem bounds concurrent background processing.
	sem chan struct{}
	// wg tracks in-flight background workers so tests can await completion.
	wg sync.WaitGroup
}

// NewCoreService constructs and initialises a CoreService from the given config.
func NewCoreService(cfg *config.ServiceConfig) (*CoreService, error) {
	db, err := database.NewDatabaseWithNamespace(
		cfg.Database.Type,
		cfg.Database.Endpoint,
		cfg.Database.Bucket,
		cfg.Database.AccessKey,
		cfg.Database.SecretKey,
		cfg.Database.ImageBaseURL,
	)
	if err != nil {
		return nil, fmt.Errorf("initialising database: %w", err)
	}

	cmdCfgs := make([]imageprocessing.CommandConfig, 0, len(cfg.Commands))
	for _, c := range cfg.Commands {
		cmdCfgs = append(cmdCfgs, imageprocessing.CommandConfig{
			Name:   c.Name,
			Params: c.Params,
		})
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil || loc == nil {
		slog.Warn("invalid timezone; defaulting to UTC", "tz", cfg.Timezone, "err", err)
		loc = time.UTC
	}

	return &CoreService{
		config:          cfg,
		databaseService: db,
		commandConfigs:  cmdCfgs,
		tzLoc:           loc,
		sem:             make(chan struct{}, maxConcurrentProcessing),
	}, nil
}

// SubmitImage validates the image, then processes it asynchronously. It returns
// immediately with the derived upload state. The image ID is the SHA-256 of the
// bytes, so submitting identical bytes is idempotent: if the image already
// exists or is already being processed, no new work is started.
//
// The returned state reflects the moment of submission (succeeded for an
// already-processed image, otherwise processing). Callers poll GetUploadState
// with the returned ID to observe progress.
func (service *CoreService) SubmitImage(ctx context.Context, image []byte, source string) (*database.UploadState, error) {
	if err := imagevalidation.Validate(image, service.config.MaxUploadBytes); err != nil {
		return nil, err
	}

	id := database.ContentID(image)

	state, err := service.databaseService.GetUploadState(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload state: %w", err)
	}
	switch state.Status {
	case database.StatusSucceeded, database.StatusProcessing:
		slog.Info("CoreService.SubmitImage: idempotent no-op", "id", id, "status", state.Status)
		return state, nil
	}

	if err := service.databaseService.MarkProcessing(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to mark processing: %w", err)
	}

	// Persist the raw upload synchronously before returning 202 so an accepted
	// upload is durable the instant the API accepts it, even if the process
	// crashes before background normalization/processing runs. Normalization and
	// the full pipeline (storing original.png + processed.png) happen afterwards
	// in processImage.
	if err := service.databaseService.StoreUpload(ctx, id, image); err != nil {
		if clearErr := service.databaseService.ClearStatusMarkers(ctx, id); clearErr != nil {
			slog.Error("CoreService.SubmitImage: failed to clear markers after upload failure", "id", id, "error", clearErr)
		}
		return nil, fmt.Errorf("failed to store upload: %w", err)
	}

	// Copy the bytes: the caller may reuse/free its buffer once we return.
	buf := make([]byte, len(image))
	copy(buf, image)

	service.wg.Add(1)
	// #nosec G118 -- by design: processing outlives the request. The 202 response
	// is sent immediately, cancelling the request context; the worker uses a fresh
	// context.Background() with its own bounded timeout (see processImage).
	go service.processImage(id, buf, source)

	return &database.UploadState{ID: id, Status: database.StatusProcessing}, nil
}

// processImage runs the processing pipeline and persists the result in the
// background. It uses a fresh, bounded context (the request context is already
// gone once SubmitImage returned) and records the outcome via status markers.
func (service *CoreService) processImage(id string, image []byte, source string) {
	defer service.wg.Done()

	service.sem <- struct{}{}
	defer func() { <-service.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), processTimeout)
	defer cancel()

	if err := service.processImageOnce(ctx, id, image, source); err != nil {
		slog.Error("CoreService.processImage: failed", "id", id, "error", err)
		if markErr := service.databaseService.MarkFailed(ctx, id, err.Error()); markErr != nil {
			slog.Error("CoreService.processImage: failed to record failure", "id", id, "error", markErr)
		}
		return
	}
	if err := service.databaseService.ClearStatusMarkers(ctx, id); err != nil {
		slog.Error("CoreService.processImage: failed to clear markers", "id", id, "error", err)
	}
	slog.Info("CoreService.processImage: succeeded", "id", id)
}

// processImageOnce applies the pipeline and stores the resulting blobs + metadata.
func (service *CoreService) processImageOnce(ctx context.Context, id string, image []byte, source string) error {
	convertedImageData, processedImage, err := service.applyPipeline(image)
	if err != nil {
		return err
	}
	if err := service.databaseService.CreateImage(ctx, id, convertedImageData, processedImage, time.Now().In(service.tzLoc), source, ""); err != nil {
		return fmt.Errorf("failed to create database image: %w", err)
	}
	return nil
}

// GetUploadState returns the derived processing state for a content-addressed ID.
func (service *CoreService) GetUploadState(ctx context.Context, id string) (*database.UploadState, error) {
	return service.databaseService.GetUploadState(ctx, id)
}

// WaitForProcessing blocks until all in-flight background processing completes.
// It is intended for tests and graceful shutdown.
func (service *CoreService) WaitForProcessing() {
	service.wg.Wait()
}

// GetImageById returns a single image's metadata by ID. Blobs are not populated.
func (service *CoreService) GetImageById(ctx context.Context, id string) (*database.Image, error) {
	return service.databaseService.GetImageByID(ctx, id)
}

// GetImageURL returns the browser-facing URL for the given image ID and variant
// ("original" or "processed"), routed through the ingress.
func (service *CoreService) GetImageURL(ctx context.Context, id, variant string) (string, error) {
	return service.databaseService.GetCurrentImageURL(ctx, id, variant)
}

// DeleteImage removes an image by its ID.
func (service *CoreService) DeleteImage(ctx context.Context, id string) error {
	slog.Info("CoreService.DeleteImage: deleting image", "id", id)
	return service.databaseService.DeleteImage(ctx, id)
}

// Close gracefully closes underlying resources.
func (service *CoreService) Close() error {
	slog.Info("CoreService.Close: closing resources")
	return service.databaseService.Close()
}

// GetOrderedImageIDs returns the persisted order of image IDs.
func (service *CoreService) GetOrderedImageIDs(ctx context.Context) ([]string, error) {
	return service.getOrderedImageIDs(ctx)
}

// GetOrderedImages returns images in current display order (index 0 = today).
func (service *CoreService) GetOrderedImages(ctx context.Context) ([]*database.Image, error) {
	return service.databaseService.GetImageMetadata(ctx)
}

// GetImageForTime returns the current image ID from the operator-managed rotation key.
func (service *CoreService) GetImageForTime(ctx context.Context, _ time.Time) (string, error) {
	return service.databaseService.GetCurrentImageID(ctx)
}

// UpdateImageOrder updates the persistent display order to match the given list of IDs.
func (service *CoreService) UpdateImageOrder(ctx context.Context, order []string) error {
	if len(order) == 0 {
		return nil
	}
	return service.databaseService.UpdateOrder(ctx, order)
}

func (service *CoreService) getOrderedImageIDs(ctx context.Context) ([]string, error) {
	return service.databaseService.GetRotationOrderedIDs(ctx)
}

// applyPipeline converts the input image to PNG and applies the configured command pipeline.
func (service *CoreService) applyPipeline(image []byte) (converted []byte, processed []byte, err error) {
	if image == nil {
		return nil, nil, fmt.Errorf("input image is nil")
	}

	normCmd, err := imageprocessing.NewNormalizeOrientationCommandWithParams()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NormalizeOrientationCommand: %w", err)
	}
	preProcessed, err := normCmd.Execute(image)
	if err != nil {
		return nil, nil, fmt.Errorf("NormalizeOrientationCommand failed: %w", err)
	}

	params := map[string]any{}
	if service.config.SvgFallbackLongSidePixelCount > 0 {
		params["svgFallbackLongSidePixelCount"] = service.config.SvgFallbackLongSidePixelCount
	}
	pngCmd, err := imageprocessing.NewPngConverterCommand(params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}
	convertedImageData, err := pngCmd.Execute(preProcessed)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}

	if len(service.commandConfigs) == 0 {
		slog.Debug("CoreService.applyPipeline: no commands configured, returning converted image", "bytes", len(convertedImageData))
		return convertedImageData, convertedImageData, nil
	}

	slog.Info("CoreService.applyPipeline: executing configured commands", "count", len(service.commandConfigs), "input_size_bytes", len(convertedImageData))
	out, execErr := imageprocessing.ExecuteCommands(convertedImageData, service.commandConfigs)
	if execErr != nil {
		return nil, nil, fmt.Errorf("failed to apply configured commands: %w", execErr)
	}
	return convertedImageData, out, nil
}
