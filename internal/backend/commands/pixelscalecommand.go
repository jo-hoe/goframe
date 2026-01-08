package commands

import (
	"bytes"
	"fmt"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"image"
	"image/png"
	"log/slog"
)

// PixelScaleParams represents typed parameters for pixel scale command
type PixelScaleParams struct {
	Height *int // Optional: if nil, will be calculated from width
	Width  *int // Optional: if nil, will be calculated from height
}

// NewPixelScaleParamsFromMap creates PixelScaleParams from a generic map
func NewPixelScaleParamsFromMap(params map[string]any) (*PixelScaleParams, error) {
	// At least one dimension must be specified
	_, hasHeight := params["height"]
	_, hasWidth := params["width"]

	if !hasHeight && !hasWidth {
		return nil, fmt.Errorf("at least one of 'height' or 'width' must be specified")
	}

	result := &PixelScaleParams{}

	// Process height if provided
	if hasHeight {
		height := commandstructure.GetIntParam(params, "height", 0)
		if height <= 0 {
			return nil, fmt.Errorf("height must be positive, got %d", height)
		}
		result.Height = &height
	}

	// Process width if provided
	if hasWidth {
		width := commandstructure.GetIntParam(params, "width", 0)
		if width <= 0 {
			return nil, fmt.Errorf("width must be positive, got %d", width)
		}
		result.Width = &width
	}

	return result, nil
}

// PixelScaleCommand handles image scaling with aspect ratio preservation
type PixelScaleCommand struct {
	name   string
	params *PixelScaleParams
}

// NewPixelScaleCommand creates a new pixel scale command from configuration parameters
func NewPixelScaleCommand(params map[string]any) (commandstructure.Command, error) {
	typedParams, err := NewPixelScaleParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &PixelScaleCommand{
		name:   "PixelScaleCommand",
		params: typedParams,
	}, nil
}

// Name returns the command name
func (c *PixelScaleCommand) Name() string {
	return c.name
}

// Execute scales the image to target dimensions while preserving aspect ratio
func (c *PixelScaleCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("PixelScaleCommand: decoding image",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("PixelScaleCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()
	aspectRatio := float64(originalWidth) / float64(originalHeight)

	// Calculate target dimensions
	var targetWidth, targetHeight int

	if c.params.Width != nil && c.params.Height != nil {
		// Both dimensions specified - use them directly
		targetWidth = *c.params.Width
		targetHeight = *c.params.Height
		slog.Debug("PixelScaleCommand: both dimensions specified",
			"target_width", targetWidth,
			"target_height", targetHeight)
	} else if c.params.Width != nil {
		// Only width specified - calculate height to preserve aspect ratio
		targetWidth = *c.params.Width
		targetHeight = int(float64(targetWidth) / aspectRatio)
		slog.Debug("PixelScaleCommand: width specified, calculated height",
			"target_width", targetWidth,
			"calculated_height", targetHeight,
			"aspect_ratio", aspectRatio)
	} else {
		// Only height specified - calculate width to preserve aspect ratio
		targetHeight = *c.params.Height
		targetWidth = int(float64(targetHeight) * aspectRatio)
		slog.Debug("PixelScaleCommand: height specified, calculated width",
			"target_height", targetHeight,
			"calculated_width", targetWidth,
			"aspect_ratio", aspectRatio)
	}

	slog.Debug("PixelScaleCommand: scaling image",
		"original_width", originalWidth,
		"original_height", originalHeight,
		"target_width", targetWidth,
		"target_height", targetHeight)

	// Create target image
	targetImg := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	// Scale using nearest-neighbor interpolation
	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			// Map target coordinates back to original image coordinates
			srcX := int(float64(x) * float64(originalWidth) / float64(targetWidth))
			srcY := int(float64(y) * float64(originalHeight) / float64(targetHeight))

			// Ensure we don't go out of bounds
			if srcX >= originalWidth {
				srcX = originalWidth - 1
			}
			if srcY >= originalHeight {
				srcY = originalHeight - 1
			}

			targetImg.Set(x, y, img.At(srcX, srcY))
		}
	}

	slog.Debug("PixelScaleCommand: encoding scaled image")

	// Encode the scaled image to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, targetImg)
	if err != nil {
		slog.Error("PixelScaleCommand: failed to encode scaled image", "error", err)
		return nil, fmt.Errorf("failed to encode scaled PNG image: %w", err)
	}

	slog.Debug("PixelScaleCommand: scaling complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// GetHeight returns the configured height (may be nil if not specified)
func (c *PixelScaleCommand) GetHeight() *int {
	return c.params.Height
}

// GetWidth returns the configured width (may be nil if not specified)
func (c *PixelScaleCommand) GetWidth() *int {
	return c.params.Width
}

// GetParams returns the typed parameters
func (c *PixelScaleCommand) GetParams() *PixelScaleParams {
	return c.params
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("PixelScaleCommand", NewPixelScaleCommand); err != nil {
		panic(fmt.Sprintf("failed to register PixelScaleCommand: %v", err))
	}
}
