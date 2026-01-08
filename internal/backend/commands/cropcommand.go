package commands

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
)

// CropParams represents typed parameters for crop command
type CropParams struct {
	Height int
	Width  int
}

// NewCropParamsFromMap creates CropParams from a generic map
func NewCropParamsFromMap(params map[string]any) (*CropParams, error) {
	// Validate required parameters exist
	if err := commandstructure.ValidateRequiredParams(params, []string{"height", "width"}); err != nil {
		return nil, err
	}

	height := commandstructure.GetIntParam(params, "height", 0)
	width := commandstructure.GetIntParam(params, "width", 0)

	// Validate dimensions are positive
	if height <= 0 {
		return nil, fmt.Errorf("height must be positive, got %d", height)
	}
	if width <= 0 {
		return nil, fmt.Errorf("width must be positive, got %d", width)
	}

	return &CropParams{
		Height: height,
		Width:  width,
	}, nil
}

// CropCommand handles image cropping operations
type CropCommand struct {
	name   string
	params *CropParams
}

// NewCropCommand creates a new crop command from configuration parameters
func NewCropCommand(params map[string]any) (commandstructure.Command, error) {
	typedParams, err := NewCropParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &CropCommand{
		name:   "CropCommand",
		params: typedParams,
	}, nil
}

// Name returns the command name
func (c *CropCommand) Name() string {
	return c.name
}

// Execute crops the image to the configured dimensions
func (c *CropCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("CropCommand: decoding image",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("CropCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	slog.Debug("CropCommand: image decoded",
		"original_width", originalWidth,
		"original_height", originalHeight,
		"target_width", c.params.Width,
		"target_height", c.params.Height)

	// Calculate crop dimensions (center crop)
	cropWidth := c.params.Width
	cropHeight := c.params.Height

	// If requested dimensions are larger than original, return original
	if cropWidth >= originalWidth && cropHeight >= originalHeight {
		slog.Debug("CropCommand: no crop needed, dimensions already smaller or equal")
		return imageData, nil
	}

	// Limit crop dimensions to original size
	if cropWidth > originalWidth {
		slog.Debug("CropCommand: limiting crop width to original width",
			"requested", cropWidth,
			"limited_to", originalWidth)
		cropWidth = originalWidth
	}
	if cropHeight > originalHeight {
		slog.Debug("CropCommand: limiting crop height to original height",
			"requested", cropHeight,
			"limited_to", originalHeight)
		cropHeight = originalHeight
	}

	// Calculate crop rectangle (center crop)
	x0 := (originalWidth - cropWidth) / 2
	y0 := (originalHeight - cropHeight) / 2

	slog.Debug("CropCommand: performing center crop",
		"crop_x", x0,
		"crop_y", y0,
		"crop_width", cropWidth,
		"crop_height", cropHeight)

	// Create a new image with the cropped region
	croppedImg := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	for y := 0; y < cropHeight; y++ {
		for x := 0; x < cropWidth; x++ {
			croppedImg.Set(x, y, img.At(x0+x, y0+y))
		}
	}

	slog.Debug("CropCommand: encoding cropped image")

	// Encode the cropped image back to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, croppedImg)
	if err != nil {
		slog.Error("CropCommand: failed to encode cropped image", "error", err)
		return nil, fmt.Errorf("failed to encode cropped PNG image: %w", err)
	}

	slog.Debug("CropCommand: crop complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// GetHeight returns the configured height
func (c *CropCommand) GetHeight() int {
	return c.params.Height
}

// GetWidth returns the configured width
func (c *CropCommand) GetWidth() int {
	return c.params.Width
}

// GetParams returns the typed parameters
func (c *CropCommand) GetParams() *CropParams {
	return c.params
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("CropCommand", NewCropCommand); err != nil {
		panic(fmt.Sprintf("failed to register CropCommand: %v", err))
	}
}
