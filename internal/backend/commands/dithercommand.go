package commands

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"os/exec"

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
	slog.Debug("DitherCommand: attempting Python dithering",
		"input_size_bytes", len(imageData))

	// Use SPECTRA6 real-world RGB palette as in the reference gist
	paletteStr := "25,30,33;232,232,232;239,222,68;178,19,24;33,87,186;18,95,32"

	// Always use Floyd-Steinberg dithering like the reference gist
	ditherMode := "fs"

	// Try to locate a Python interpreter
	pythonBins := []string{"python3", "python"}
	var pythonPath string
	for _, bin := range pythonBins {
		if p, err := exec.LookPath(bin); err == nil {
			pythonPath = p
			break
		}
	}

	// Potential script locations (container and local dev)
	scriptCandidates := []string{
		"/app/scripts/dither.py", // in container
		"scripts/dither.py",      // local dev
		"./scripts/dither.py",    // local dev explicit
	}
	var scriptPath string
	for _, sp := range scriptCandidates {
		if _, err := os.Stat(sp); err == nil {
			scriptPath = sp
			break
		}
	}

	// If Python and script are available, try running Python dithering
	if pythonPath != "" && scriptPath != "" {
		cmd := exec.Command(pythonPath, scriptPath, "--palette", paletteStr, "--dither", ditherMode)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdin = bytes.NewReader(imageData)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err == nil && stdout.Len() > 0 {
			outBytes := stdout.Bytes()
			slog.Debug("DitherCommand: Python dithering succeeded",
				"output_size_bytes", len(outBytes))
			return outBytes, nil
		} else {
			if err != nil {
				slog.Warn("DitherCommand: Python dithering failed, falling back to Go implementation",
					"error", err, "stderr", stderr.String())
			} else {
				slog.Warn("DitherCommand: Python dithering produced empty output, falling back")
			}
		}
	} else {
		if pythonPath == "" {
			slog.Debug("DitherCommand: Python not found, using Go dithering fallback")
		} else {
			slog.Debug("DitherCommand: dithering script not found, using Go dithering fallback")
		}
	}

	// Fallback: Go-based dithering (original implementation)
	slog.Debug("DitherCommand: decoding image for Go fallback",
		"input_size_bytes", len(imageData))

	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("DitherCommand: failed to decode PNG image in fallback", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Use fixed SPECTRA6 real-world RGB palette for fallback as well
	fixedPalette := []color.Color{
		color.RGBA{R: 25, G: 30, B: 33, A: 255},
		color.RGBA{R: 232, G: 232, B: 232, A: 255},
		color.RGBA{R: 239, G: 222, B: 68, A: 255},
		color.RGBA{R: 178, G: 19, B: 24, A: 255},
		color.RGBA{R: 33, G: 87, B: 186, A: 255},
		color.RGBA{R: 18, G: 95, B: 32, A: 255},
	}

	slog.Debug("DitherCommand: creating Go ditherer",
		"palette_size", len(fixedPalette))

	// Create ditherer
	d := dither.NewDitherer(fixedPalette)
	if d == nil {
		return nil, fmt.Errorf("failed to create ditherer with palette")
	}

	// Use Floyd-Steinberg algorithm with serpentine scanning (ignore strength for similarity with gist)
	d.Matrix = dither.FloydSteinberg
	d.Serpentine = true

	slog.Debug("DitherCommand: applying Go dithering")

	// Apply dithering
	ditheredImg := d.Dither(img)

	slog.Debug("DitherCommand: encoding Go-dithered image")

	// Encode the dithered image to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, ditheredImg)
	if err != nil {
		slog.Error("DitherCommand: failed to encode dithered image", "error", err)
		return nil, fmt.Errorf("failed to encode dithered PNG image: %w", err)
	}

	slog.Debug("DitherCommand: dithering complete (Go fallback)",
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
