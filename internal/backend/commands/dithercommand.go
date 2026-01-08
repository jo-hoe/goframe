package commands

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"log/slog"

	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"github.com/makeworld-the-better-one/dither/v2"
)

// DitherParams represents typed parameters for dither command
type DitherParams struct {
	Palette  [][]int  // RGB color palette, e.g. [[0,0,0], [255,255,255]]
	Strength *float32 // Optional strength for error diffusion (0.0-1.0)
}

// NewDitherParamsFromMap creates DitherParams from a generic map
func NewDitherParamsFromMap(params map[string]any) (*DitherParams, error) {
	ditherParams := &DitherParams{}

	// Parse palette (optional, defaults to black and white)
	if paletteParam, ok := params["palette"]; ok {
		palette, err := parsePalette(paletteParam)
		if err != nil {
			return nil, fmt.Errorf("invalid palette: %w", err)
		}
		ditherParams.Palette = palette
	} else {
		// Default to black and white palette
		ditherParams.Palette = [][]int{
			{0, 0, 0},       // Black
			{255, 255, 255}, // White
		}
	}

	// Parse strength (optional)
	if strengthParam, ok := params["strength"]; ok {
		if strength, ok := strengthParam.(float64); ok {
			if strength < 0 || strength > 1 {
				return nil, fmt.Errorf("strength must be between 0 and 1, got %f", strength)
			}
			strength32 := float32(strength)
			ditherParams.Strength = &strength32
		} else if strengthInt, ok := strengthParam.(int); ok {
			strength := float64(strengthInt)
			if strength < 0 || strength > 1 {
				return nil, fmt.Errorf("strength must be between 0 and 1, got %f", strength)
			}
			strength32 := float32(strength)
			ditherParams.Strength = &strength32
		} else {
			return nil, fmt.Errorf("strength must be a number")
		}
	}

	return ditherParams, nil
}

// parsePalette converts various palette formats to [][]int
func parsePalette(paletteParam any) ([][]int, error) {
	switch p := paletteParam.(type) {
	case []any:
		palette := make([][]int, len(p))
		for i, colorParam := range p {
			colorSlice, ok := colorParam.([]any)
			if !ok {
				return nil, fmt.Errorf("color at index %d must be an array", i)
			}
			if len(colorSlice) != 3 {
				return nil, fmt.Errorf("color at index %d must have exactly 3 values (RGB)", i)
			}

			rgb := make([]int, 3)
			for j, val := range colorSlice {
				switch v := val.(type) {
				case int:
					if v < 0 || v > 255 {
						return nil, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", i, j, v)
					}
					rgb[j] = v
				case float64:
					intVal := int(v)
					if intVal < 0 || intVal > 255 {
						return nil, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", i, j, intVal)
					}
					rgb[j] = intVal
				default:
					return nil, fmt.Errorf("RGB value at color %d, component %d must be a number", i, j)
				}
			}
			palette[i] = rgb
		}
		return palette, nil
	case [][]int:
		// Already in correct format
		return p, nil
	default:
		return nil, fmt.Errorf("palette must be an array of RGB arrays")
	}
}

// DitherCommand handles image dithering
type DitherCommand struct {
	name   string
	params *DitherParams
}

// NewDitherCommand creates a new dither command from configuration parameters
func NewDitherCommand(params map[string]any) (commandstructure.Command, error) {
	typedParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &DitherCommand{
		name:   "DitherCommand",
		params: typedParams,
	}, nil
}

// Name returns the command name
func (c *DitherCommand) Name() string {
	return c.name
}

// Execute applies dithering to the image
func (c *DitherCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("DitherCommand: decoding image",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("DitherCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Convert palette to color.Color slice
	palette := make([]color.Color, len(c.params.Palette))
	for i, rgb := range c.params.Palette {
		palette[i] = color.RGBA{
			R: uint8(rgb[0]),
			G: uint8(rgb[1]),
			B: uint8(rgb[2]),
			A: 255,
		}
	}

	slog.Debug("DitherCommand: creating ditherer",
		"palette_size", len(palette))

	// Create ditherer
	d := dither.NewDitherer(palette)
	if d == nil {
		return nil, fmt.Errorf("failed to create ditherer with palette")
	}

	// Use FloydSteinberg algorithm with serpentine scanning
	if c.params.Strength != nil {
		d.Matrix = dither.ErrorDiffusionStrength(dither.FloydSteinberg, *c.params.Strength)
	} else {
		d.Matrix = dither.FloydSteinberg
	}
	d.Serpentine = true

	slog.Debug("DitherCommand: applying dithering")

	// Apply dithering
	ditheredImg := d.Dither(img)

	slog.Debug("DitherCommand: encoding dithered image")

	// Encode the dithered image to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, ditheredImg)
	if err != nil {
		slog.Error("DitherCommand: failed to encode dithered image", "error", err)
		return nil, fmt.Errorf("failed to encode dithered PNG image: %w", err)
	}

	slog.Debug("DitherCommand: dithering complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// GetParams returns the typed parameters
func (c *DitherCommand) GetParams() *DitherParams {
	return c.params
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("DitherCommand", NewDitherCommand); err != nil {
		panic(fmt.Sprintf("failed to register DitherCommand: %v", err))
	}
}
