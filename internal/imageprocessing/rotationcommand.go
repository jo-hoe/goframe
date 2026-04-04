package imageprocessing

import (
	"fmt"
	"log/slog"

)

const (
	// Steps90 rotates by 90 degrees.
	Steps90 = 1
	// Steps180 rotates by 180 degrees.
	Steps180 = 2
	// Steps270 rotates by 270 degrees.
	Steps270 = 3
)

// RotationParams holds the typed parameters for a RotationCommand.
type RotationParams struct {
	// Steps is the number of 90-degree rotation steps (1, 2, or 3).
	Steps     int
	Clockwise bool
}

// NewRotationParamsFromMap creates RotationParams from a generic parameter map.
func NewRotationParamsFromMap(params map[string]any) (*RotationParams, error) {
	steps := GetIntParam(params, "steps", 1)
	clockwise := GetBoolParam(params, "clockwise", true)
	return NewRotationParams(steps, clockwise)
}

// NewRotationParams creates and validates RotationParams from concrete values.
func NewRotationParams(steps int, clockwise bool) (*RotationParams, error) {
	if steps < 1 || steps > 3 {
		return nil, fmt.Errorf("invalid rotation steps: %d (must be 1, 2, or 3)", steps)
	}
	return &RotationParams{Steps: steps, Clockwise: clockwise}, nil
}

// RotationCommand rotates an image by a multiple of 90 degrees.
type RotationCommand struct {
	name   string
	params *RotationParams
}

// NewRotationCommand creates a RotationCommand from a generic parameter map.
func NewRotationCommand(params map[string]any) (Command, error) {
	typedParams, err := NewRotationParamsFromMap(params)
	if err != nil {
		return nil, err
	}
	return newRotationCommandFromParams(typedParams), nil
}

// NewRotationCommandWithParams creates a RotationCommand from concrete typed parameters.
func NewRotationCommandWithParams(steps int, clockwise bool) (*RotationCommand, error) {
	typedParams, err := NewRotationParams(steps, clockwise)
	if err != nil {
		return nil, err
	}
	return newRotationCommandFromParams(typedParams), nil
}

func newRotationCommandFromParams(params *RotationParams) *RotationCommand {
	return &RotationCommand{name: "RotationCommand", params: params}
}

// Name returns the command name.
func (c *RotationCommand) Name() string {
	return c.name
}

// Execute rotates the image by the configured number of 90-degree steps.
func (c *RotationCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("RotationCommand: decoding image",
		"input_size_bytes", len(imageData),
		"steps", c.params.Steps,
		"clockwise", c.params.Clockwise)

	img, err := decodePNG(imageData)
	if err != nil {
		slog.Error("RotationCommand: failed to decode PNG image", "error", err)
		return nil, err
	}

	rotated := applyRotationSteps(img, c.params.Steps, c.params.Clockwise)

	result, err := encodePNG(rotated)
	if err != nil {
		slog.Error("RotationCommand: failed to encode image", "error", err)
		return nil, err
	}

	slog.Debug("RotationCommand: rotation complete", "output_size_bytes", len(result))
	return result, nil
}

// GetParams returns the typed parameters.
func (c *RotationCommand) GetParams() *RotationParams {
	return c.params
}

func init() {
	if err := DefaultRegistry.Register("RotationCommand", NewRotationCommand); err != nil {
		panic(fmt.Sprintf("failed to register RotationCommand: %v", err))
	}
}
