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

// ImageConverterParams represents typed parameters for image converter processor
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

// ImageConverterProcessor handles image format conversion
type ImageConverterProcessor struct {
	name   string
	params *ImageConverterParams
}

// NewImageConverterProcessor creates a new image converter processor from configuration parameters
func NewImageConverterProcessor(params map[string]any) (ImageProcessor, error) {
	typedParams, err := NewImageConverterParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &ImageConverterProcessor{
		name:   "ImageConverterProcessor",
		params: typedParams,
	}, nil
}

// Type returns the processor type
func (p *ImageConverterProcessor) Type() string {
	return p.name
}

// ProcessImage converts the image to the target format
func (p *ImageConverterProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	slog.Debug("ImageConverterProcessor: decoding image",
		"input_size_bytes", len(imageData),
		"target_format", p.params.TargetType)

	// Decode the image (supports multiple formats)
	img, currentFormat, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("ImageConverterProcessor: failed to decode image", "error", err)
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Normalize format names
	currentFormat = strings.ToLower(currentFormat)
	if currentFormat == "jpg" {
		currentFormat = "jpeg"
	}

	slog.Debug("ImageConverterProcessor: image decoded",
		"current_format", currentFormat,
		"target_format", p.params.TargetType)

	// If already in target format, return original
	if currentFormat == p.params.TargetType {
		slog.Debug("ImageConverterProcessor: already in target format, no conversion needed")
		return imageData, nil
	}

	slog.Debug("ImageConverterProcessor: converting image format",
		"from", currentFormat,
		"to", p.params.TargetType)

	// Encode to target format
	var buf bytes.Buffer
	switch p.params.TargetType {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		slog.Error("ImageConverterProcessor: unsupported target format",
			"target_format", p.params.TargetType)
		return nil, fmt.Errorf("unsupported target format: %s", p.params.TargetType)
	}

	if err != nil {
		slog.Error("ImageConverterProcessor: failed to encode image",
			"target_format", p.params.TargetType,
			"error", err)
		return nil, fmt.Errorf("failed to encode image to %s: %w", p.params.TargetType, err)
	}

	slog.Debug("ImageConverterProcessor: conversion complete",
		"output_size_bytes", buf.Len(),
		"output_format", p.params.TargetType)

	return buf.Bytes(), nil
}

// GetTargetType returns the configured target type
func (p *ImageConverterProcessor) GetTargetType() string {
	return p.params.TargetType
}

// GetParams returns the typed parameters
func (p *ImageConverterProcessor) GetParams() *ImageConverterParams {
	return p.params
}

func init() {
	// Register the processor in the default registry
	if err := DefaultRegistry.Register("ImageConverterProcessor", NewImageConverterProcessor); err != nil {
		panic(fmt.Sprintf("failed to register ImageConverterProcessor: %v", err))
	}
}
