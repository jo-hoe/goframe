package commands

import (
	"bytes"
	"fmt"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"image"
	"image/png"
	"log/slog"
)

// OrientationParams represents typed parameters for orientation command
type OrientationParams struct {
	Orientation      string
	RotateWhenSquare bool
	Clockwise        bool
}

// NewOrientationParamsFromMap creates OrientationParams from a generic map
func NewOrientationParamsFromMap(params map[string]any) (*OrientationParams, error) {
	orientation := commandstructure.GetStringParam(params, "orientation", "portrait")
	rotateWhenSquare := commandstructure.GetBoolParam(params, "rotateWhenSquare", false)
	clockwise := commandstructure.GetBoolParam(params, "clockwise", true)

	// Validate orientation value
	validOrientations := map[string]bool{
		"portrait":  true,
		"landscape": true,
	}

	if !validOrientations[orientation] {
		return nil, fmt.Errorf("invalid orientation: %s (must be 'portrait' or 'landscape')", orientation)
	}

	return &OrientationParams{
		Orientation:      orientation,
		RotateWhenSquare: rotateWhenSquare,
		Clockwise:        clockwise,
	}, nil
}

// OrientationCommand handles image orientation adjustments
type OrientationCommand struct {
	name   string
	params *OrientationParams
}

// NewOrientationCommand creates a new orientation command from configuration parameters
func NewOrientationCommand(params map[string]any) (commandstructure.Command, error) {
	typedParams, err := NewOrientationParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &OrientationCommand{
		name:   "OrientationCommand",
		params: typedParams,
	}, nil
}

// NewOrientationCommandWithParams creates a new orientation command from concrete typed parameters
func NewOrientationCommandWithParams(orientation string) (*OrientationCommand, error) {
	validOrientations := map[string]bool{
		"portrait":  true,
		"landscape": true,
	}

	if !validOrientations[orientation] {
		return nil, fmt.Errorf("invalid orientation: %s (must be 'portrait' or 'landscape')", orientation)
	}

	return &OrientationCommand{
		name: "OrientationCommand",
		params: &OrientationParams{
			Orientation:      orientation,
			RotateWhenSquare: false, // default: do nothing for square
			Clockwise:        true,  // default: rotate clockwise
		},
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
		"target_orientation", c.params.Orientation,
		"rotate_when_square", c.params.RotateWhenSquare,
		"clockwise", c.params.Clockwise)

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

	// Handle square images according to configuration
	if width == height {
		if !c.params.RotateWhenSquare {
			slog.Info("OrientationCommand: image is square and rotateWhenSquare=false; no rotation performed")
			return imageData, nil
		}
		// Rotate 90 degrees using configured direction (default clockwise)
		slog.Info("OrientationCommand: image is square; rotating 90 degrees", "clockwise", c.params.Clockwise)
		rotatedImg := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if c.params.Clockwise {
					// 90° clockwise: (x,y) -> (height-1-y, x)
					rotatedImg.Set(height-1-y, x, img.At(x, y))
				} else {
					// 90° counterclockwise: (x,y) -> (y, width-1-x)
					rotatedImg.Set(y, width-1-x, img.At(x, y))
				}
			}
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, rotatedImg); err != nil {
			slog.Error("OrientationCommand: failed to encode rotated image", "error", err)
			return nil, fmt.Errorf("failed to encode rotated PNG image: %w", err)
		}
		slog.Debug("OrientationCommand: rotation complete (square case)", "output_size_bytes", buf.Len())
		return buf.Bytes(), nil
	}

	// Non-square: Determine if rotation is needed to match target orientation
	isCurrentlyPortrait := height > width // strict (square handled above)
	needsPortrait := c.params.Orientation == "portrait"

	slog.Info("OrientationCommand: analyzing orientation",
		"width", width,
		"height", height,
		"currently_portrait", isCurrentlyPortrait,
		"needs_portrait", needsPortrait)

	// If already in correct orientation, return original
	if isCurrentlyPortrait == needsPortrait {
		slog.Info("OrientationCommand: already in correct orientation, no rotation needed")
		return imageData, nil
	}

	// Rotate 90 degrees in configured direction to switch between portrait and landscape
	slog.Info("OrientationCommand: rotating image 90 degrees", "clockwise", c.params.Clockwise)
	rotatedImg := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if c.params.Clockwise {
				// 90° clockwise: (x,y) -> (height-1-y, x)
				rotatedImg.Set(height-1-y, x, img.At(x, y))
			} else {
				// 90° counterclockwise: (x,y) -> (y, width-1-x)
				rotatedImg.Set(y, width-1-x, img.At(x, y))
			}
		}
	}

	slog.Debug("OrientationCommand: encoding rotated image")

	// Encode the rotated image back to PNG bytes
	var buf bytes.Buffer
	if err := png.Encode(&buf, rotatedImg); err != nil {
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
	if err := commandstructure.DefaultRegistry.Register("OrientationCommand", NewOrientationCommand); err != nil {
		panic(fmt.Sprintf("failed to register OrientationCommand: %v", err))
	}
}
