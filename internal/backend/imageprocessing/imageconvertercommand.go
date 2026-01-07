package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log/slog"
	"strings"
)

// ImageConverterParams represents typed parameters for image converter command
type ImageConverterParams struct {
	TargetType string
}

// NewImageConverterParamsFromMap creates ImageConverterParams from a generic map
func NewImageConverterParamsFromMap(params map[string]any) (*ImageConverterParams, error) {
	targetType := getStringParam(params, "targetType", "png")
	targetType = strings.ToLower(targetType)

	// Validate target type
	validTypes := map[string]bool{
		"png":  true,
		"jpeg": true,
		"jpg":  true,
		"gif":  true,
	}

	if !validTypes[targetType] {
		return nil, fmt.Errorf("invalid target type: %s (must be 'png', 'jpeg', 'jpg', or 'gif')", targetType)
	}

	// Normalize jpeg/jpg to jpeg
	if targetType == "jpg" {
		targetType = "jpeg"
	}

	return &ImageConverterParams{
		TargetType: targetType,
	}, nil
}

// ImageConverterCommand handles image format conversion
type ImageConverterCommand struct {
	name   string
	params *ImageConverterParams
}

// NewImageConverterCommand creates a new image converter command from configuration parameters
func NewImageConverterCommand(params map[string]any) (Command, error) {
	typedParams, err := NewImageConverterParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &ImageConverterCommand{
		name:   "ImageConverterCommand",
		params: typedParams,
	}, nil
}

// Name returns the command name
func (c *ImageConverterCommand) Name() string {
	return c.name
}

// Execute converts the image to the target format
func (c *ImageConverterCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("ImageConverterCommand: decoding image",
		"input_size_bytes", len(imageData),
		"target_format", c.params.TargetType)

	// Decode the image (supports multiple formats)
	img, currentFormat, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("ImageConverterCommand: failed to decode image", "error", err)
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Normalize format names
	currentFormat = strings.ToLower(currentFormat)
	if currentFormat == "jpg" {
		currentFormat = "jpeg"
	}

	slog.Debug("ImageConverterCommand: image decoded",
		"current_format", currentFormat,
		"target_format", c.params.TargetType)

	// If already in target format, return original
	if currentFormat == c.params.TargetType {
		slog.Debug("ImageConverterCommand: already in target format, no conversion needed")
		return imageData, nil
	}

	slog.Debug("ImageConverterCommand: converting image format",
		"from", currentFormat,
		"to", c.params.TargetType)

	// Encode to target format
	var buf bytes.Buffer
	switch c.params.TargetType {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		slog.Error("ImageConverterCommand: unsupported target format",
			"target_format", c.params.TargetType)
		return nil, fmt.Errorf("unsupported target format: %s", c.params.TargetType)
	}

	if err != nil {
		slog.Error("ImageConverterCommand: failed to encode image",
			"target_format", c.params.TargetType,
			"error", err)
		return nil, fmt.Errorf("failed to encode image to %s: %w", c.params.TargetType, err)
	}

	slog.Debug("ImageConverterCommand: conversion complete",
		"output_size_bytes", buf.Len(),
		"output_format", c.params.TargetType)

	return buf.Bytes(), nil
}

// GetTargetType returns the configured target type
func (c *ImageConverterCommand) GetTargetType() string {
	return c.params.TargetType
}

// GetParams returns the typed parameters
func (c *ImageConverterCommand) GetParams() *ImageConverterParams {
	return c.params
}

func init() {
	// Register the command in the default registry
	if err := DefaultRegistry.Register("ImageConverterCommand", NewImageConverterCommand); err != nil {
		panic(fmt.Sprintf("failed to register ImageConverterCommand: %v", err))
	}
}
