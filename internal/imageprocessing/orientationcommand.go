package imageprocessing

import (
	"fmt"
	"image"
	"log/slog"

)

// OrientationParams represents typed parameters for an OrientationCommand.
type OrientationParams struct {
	Orientation      string
	RotateWhenSquare bool
	Clockwise        bool
}

// NewOrientationParamsFromMap creates OrientationParams from a generic map.
func NewOrientationParamsFromMap(params map[string]any) (*OrientationParams, error) {
	orientation := GetStringParam(params, "orientation", "portrait")
	rotateWhenSquare := GetBoolParam(params, "rotateWhenSquare", false)
	clockwise := GetBoolParam(params, "clockwise", true)
	return NewOrientationParams(orientation, rotateWhenSquare, clockwise)
}

// NewOrientationParams creates and validates OrientationParams from concrete values.
func NewOrientationParams(orientation string, rotateWhenSquare bool, clockwise bool) (*OrientationParams, error) {
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

// OrientationCommand adjusts an image to match a target orientation (portrait/landscape).
type OrientationCommand struct {
	name   string
	params *OrientationParams
}

// NewOrientationCommand creates an OrientationCommand from a generic parameter map.
func NewOrientationCommand(params map[string]any) (Command, error) {
	typedParams, err := NewOrientationParamsFromMap(params)
	if err != nil {
		return nil, err
	}
	return newOrientationCommandFromParams(typedParams), nil
}

// NewOrientationCommandWithParams creates an OrientationCommand from concrete typed parameters.
func NewOrientationCommandWithParams(orientation string) (*OrientationCommand, error) {
	typedParams, err := NewOrientationParams(orientation, false, true)
	if err != nil {
		return nil, err
	}
	return newOrientationCommandFromParams(typedParams), nil
}

func newOrientationCommandFromParams(params *OrientationParams) *OrientationCommand {
	return &OrientationCommand{name: "OrientationCommand", params: params}
}

// Name returns the command name.
func (c *OrientationCommand) Name() string {
	return c.name
}

// Execute rotates the image if necessary to match the configured orientation.
func (c *OrientationCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("OrientationCommand: decoding image",
		"input_size_bytes", len(imageData),
		"target_orientation", c.params.Orientation,
		"rotate_when_square", c.params.RotateWhenSquare,
		"clockwise", c.params.Clockwise)

	img, err := decodePNG(imageData)
	if err != nil {
		slog.Error("OrientationCommand: failed to decode PNG image", "error", err)
		return nil, err
	}

	b := img.Bounds()
	width, height := b.Dx(), b.Dy()

	if width == height {
		return c.executeSquare(imageData, img)
	}
	return c.executeNonSquare(imageData, img, width, height)
}

func (c *OrientationCommand) executeSquare(original []byte, img image.Image) ([]byte, error) {
	if !c.params.RotateWhenSquare {
		slog.Info("OrientationCommand: image is square and rotateWhenSquare=false; no rotation performed")
		return original, nil
	}
	slog.Info("OrientationCommand: image is square; rotating 90 degrees", "clockwise", c.params.Clockwise)
	return c.encodeRotated(img)
}

func (c *OrientationCommand) executeNonSquare(original []byte, img image.Image, width, height int) ([]byte, error) {
	isCurrentlyPortrait := height > width
	needsPortrait := c.params.Orientation == "portrait"

	slog.Info("OrientationCommand: analyzing orientation",
		"width", width,
		"height", height,
		"currently_portrait", isCurrentlyPortrait,
		"needs_portrait", needsPortrait)

	if isCurrentlyPortrait == needsPortrait {
		slog.Info("OrientationCommand: already in correct orientation, no rotation needed")
		return original, nil
	}

	slog.Info("OrientationCommand: rotating image 90 degrees", "clockwise", c.params.Clockwise)
	return c.encodeRotated(img)
}

// encodeRotated applies one 90-degree rotation and encodes the result.
func (c *OrientationCommand) encodeRotated(img image.Image) ([]byte, error) {
	rotated := applyRotationSteps(img, Steps90, c.params.Clockwise)
	result, err := encodePNG(rotated)
	if err != nil {
		slog.Error("OrientationCommand: failed to encode rotated image", "error", err)
		return nil, err
	}
	slog.Debug("OrientationCommand: rotation complete", "output_size_bytes", len(result))
	return result, nil
}

// GetOrientation returns the configured orientation.
func (c *OrientationCommand) GetOrientation() string {
	return c.params.Orientation
}

// GetParams returns the typed parameters.
func (c *OrientationCommand) GetParams() *OrientationParams {
	return c.params
}

func init() {
	if err := DefaultRegistry.Register("OrientationCommand", NewOrientationCommand); err != nil {
		panic(fmt.Sprintf("failed to register OrientationCommand: %v", err))
	}
}
