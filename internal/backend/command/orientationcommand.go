package command

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log/slog"
)

// OrientationParams represents typed parameters for orientation command
type OrientationParams struct {
	Orientation string
}

// NewOrientationParamsFromMap creates OrientationParams from a generic map
func NewOrientationParamsFromMap(params map[string]any) (*OrientationParams, error) {
	orientation := getStringParam(params, "orientation", "portrait")

	// Validate orientation value
	validOrientations := map[string]bool{
		"portrait":  true,
		"landscape": true,
	}

	if !validOrientations[orientation] {
		return nil, fmt.Errorf("invalid orientation: %s (must be 'portrait' or 'landscape')", orientation)
	}

	return &OrientationParams{
		Orientation: orientation,
	}, nil
}

// OrientationCommand handles image orientation adjustments
type OrientationCommand struct {
	name   string
	params *OrientationParams
}

// NewOrientationCommand creates a new orientation command from configuration parameters
func NewOrientationCommand(params map[string]any) (Command, error) {
	typedParams, err := NewOrientationParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &OrientationCommand{
		name:   "OrientationCommand",
		params: typedParams,
	}, nil
}

// Name returns the command name
func (c *OrientationCommand) Name() string {
	return c.name
}

// Execute rotates the image based on the configured orientation
func (c *OrientationCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("OrientationCommand: decoding image",
		"input_size_bytes", len(imageData),
		"target_orientation", c.params.Orientation)

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("OrientationCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Determine if rotation is needed
	isCurrentlyPortrait := height >= width
	needsPortrait := c.params.Orientation == "portrait"

	slog.Debug("OrientationCommand: analyzing orientation",
		"width", width,
		"height", height,
		"currently_portrait", isCurrentlyPortrait,
		"needs_portrait", needsPortrait)

	// If already in correct orientation, return original
	if isCurrentlyPortrait == needsPortrait {
		slog.Debug("OrientationCommand: already in correct orientation, no rotation needed")
		return imageData, nil
	}

	slog.Debug("OrientationCommand: rotating image 90 degrees clockwise")

	// Rotate 90 degrees clockwise to switch between portrait and landscape
	rotatedImg := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Rotate 90 degrees clockwise: (x,y) -> (height-1-y, x)
			rotatedImg.Set(height-1-y, x, img.At(x, y))
		}
	}

	slog.Debug("OrientationCommand: encoding rotated image")

	// Encode the rotated image back to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, rotatedImg)
	if err != nil {
		slog.Error("OrientationCommand: failed to encode rotated image", "error", err)
		return nil, fmt.Errorf("failed to encode rotated PNG image: %w", err)
	}

	slog.Debug("OrientationCommand: rotation complete",
		"output_size_bytes", buf.Len(),
		"new_width", height,
		"new_height", width)

	return buf.Bytes(), nil
}

// GetOrientation returns the configured orientation
func (c *OrientationCommand) GetOrientation() string {
	return c.params.Orientation
}

// GetParams returns the typed parameters
func (c *OrientationCommand) GetParams() *OrientationParams {
	return c.params
}

func init() {
	// Register the command in the default registry
	if err := DefaultRegistry.Register("OrientationCommand", NewOrientationCommand); err != nil {
		panic(fmt.Sprintf("failed to register OrientationCommand: %v", err))
	}
}
