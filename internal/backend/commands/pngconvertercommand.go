package commands

import (
	"bytes"
	"fmt"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"image"
	"image/png"
	"log/slog"
)

// hasCorrectPngSignature checks whether the provided data begins with a valid PNG signature
func hasCorrectPngSignature(data []byte) bool {
	// PNG signature: 0x89 'P' 'N' 'G' 0x0D 0x0A 0x1A 0x0A
	if len(data) < 8 {
		return false
	}
	expected := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	return bytes.Equal(data[:8], expected)
}

// PngConverterCommand handles image format conversion to PNG
type PngConverterCommand struct {
	name string
}

// NewPngConverterCommand creates a new PNG converter command
func NewPngConverterCommand(params map[string]any) (commandstructure.Command, error) {
	return &PngConverterCommand{
		name: "PngConverterCommand",
	}, nil
}

// NewPngConverterCommandDirect creates a new PNG converter command directly (no parameters needed)
func NewPngConverterCommandDirect() *PngConverterCommand {
	return &PngConverterCommand{
		name: "PngConverterCommand",
	}
}

// Name returns the command name
func (c *PngConverterCommand) Name() string {
	return c.name
}

// Execute converts the image to PNG format
func (c *PngConverterCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("PngConverterCommand: checking image format",
		"input_size_bytes", len(imageData))

	// Check if already PNG with correct signature
	if hasCorrectPngSignature(imageData) {
		slog.Debug("PngConverterCommand: already in PNG format with correct signature, no conversion needed")
		return imageData, nil
	}

	slog.Debug("PngConverterCommand: decoding image for conversion")

	// Decode the image (supports multiple input formats)
	img, currentFormat, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("PngConverterCommand: failed to decode image", "error", err)
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	slog.Debug("PngConverterCommand: converting image to PNG",
		"current_format", currentFormat)

	// Encode to PNG format
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	if err != nil {
		slog.Error("PngConverterCommand: failed to encode image to PNG",
			"error", err)
		return nil, fmt.Errorf("failed to encode image to PNG: %w", err)
	}

	slog.Debug("PngConverterCommand: conversion complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("PngConverterCommand", NewPngConverterCommand); err != nil {
		panic(fmt.Sprintf("failed to register PngConverterCommand: %v", err))
	}
}
