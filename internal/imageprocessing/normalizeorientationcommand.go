package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"log/slog"
)

// NormalizeOrientationParams holds the typed parameters for an NormalizeOrientationCommand.
// Currently parameter-free; the struct exists for API consistency and future extension.
type NormalizeOrientationParams struct{}

// NewNormalizeOrientationParamsFromMap creates NormalizeOrientationParams from a generic parameter map.
func NewNormalizeOrientationParamsFromMap(_ map[string]any) (*NormalizeOrientationParams, error) {
	return NewNormalizeOrientationParams()
}

// NewNormalizeOrientationParams creates NormalizeOrientationParams.
func NewNormalizeOrientationParams() (*NormalizeOrientationParams, error) {
	return &NormalizeOrientationParams{}, nil
}

// NormalizeOrientationCommand reads the EXIF orientation tag from the input bytes,
// applies the corresponding pixel transform, and returns a PNG whose pixels are
// visually upright regardless of what the original EXIF tag indicated.
// Non-JPEG input (PNG, SVG, BMP, TIFF, WebP, GIF) is returned unchanged — only
// JPEG carries EXIF orientation in practice.
type NormalizeOrientationCommand struct {
	name   string
	params *NormalizeOrientationParams
}

// NewNormalizeOrientationCommand creates an NormalizeOrientationCommand from a generic parameter map.
func NewNormalizeOrientationCommand(params map[string]any) (Command, error) {
	typedParams, err := NewNormalizeOrientationParamsFromMap(params)
	if err != nil {
		return nil, err
	}
	return newNormalizeOrientationCommandFromParams(typedParams), nil
}

// NewNormalizeOrientationCommandWithParams creates an NormalizeOrientationCommand from concrete typed parameters.
func NewNormalizeOrientationCommandWithParams() (*NormalizeOrientationCommand, error) {
	typedParams, err := NewNormalizeOrientationParams()
	if err != nil {
		return nil, err
	}
	return newNormalizeOrientationCommandFromParams(typedParams), nil
}

func newNormalizeOrientationCommandFromParams(params *NormalizeOrientationParams) *NormalizeOrientationCommand {
	return &NormalizeOrientationCommand{name: "NormalizeOrientationCommand", params: params}
}

// Name returns the command name.
func (c *NormalizeOrientationCommand) Name() string {
	return c.name
}

// Execute reads the EXIF orientation from imageData, applies the corresponding pixel
// transform, and returns the corrected image encoded as PNG.
// If imageData is not a JPEG or carries no orientation tag, it is returned unchanged.
func (c *NormalizeOrientationCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("NormalizeOrientationCommand: reading EXIF orientation",
		"input_size_bytes", len(imageData))

	orientation := readOrientationSilently(imageData)

	slog.Info("NormalizeOrientationCommand: orientation read", "orientation", orientation)

	if orientation == NormalizeOrientationNormal {
		slog.Debug("NormalizeOrientationCommand: no transform needed, returning input unchanged")
		return imageData, nil
	}

	return applyOrientationAndEncode(imageData, orientation)
}

// GetParams returns the typed parameters.
func (c *NormalizeOrientationCommand) GetParams() *NormalizeOrientationParams {
	return c.params
}

// readOrientationSilently returns the EXIF orientation or NormalizeOrientationNormal on any error.
func readOrientationSilently(imageData []byte) NormalizeOrientation {
	orientation, err := ReadJPEGOrientation(imageData)
	if err != nil {
		slog.Debug("NormalizeOrientationCommand: no EXIF orientation found", "reason", err)
		return NormalizeOrientationNormal
	}
	return orientation
}

// applyOrientationAndEncode decodes imageData, applies the orientation transform, and encodes as PNG.
func applyOrientationAndEncode(imageData []byte, orientation NormalizeOrientation) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("NormalizeOrientationCommand: failed to decode image", "error", err)
		return nil, fmt.Errorf("NormalizeOrientationCommand: decode failed: %w", err)
	}

	corrected := ApplyOrientation(toRGBA(img), orientation)

	result, err := encodePNG(corrected)
	if err != nil {
		slog.Error("NormalizeOrientationCommand: failed to encode image", "error", err)
		return nil, fmt.Errorf("NormalizeOrientationCommand: encode failed: %w", err)
	}

	slog.Debug("NormalizeOrientationCommand: transform applied",
		"orientation", orientation,
		"output_size_bytes", len(result))
	return result, nil
}

func init() {
	if err := DefaultRegistry.Register("NormalizeOrientationCommand", NewNormalizeOrientationCommand); err != nil {
		panic(fmt.Sprintf("failed to register NormalizeOrientationCommand: %v", err))
	}
}
