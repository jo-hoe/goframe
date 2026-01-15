package commands

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"

	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
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

// Execute applies dithering to the image (Go-only, mimic Python gist behavior)
func (c *DitherCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("DitherCommand: Go-only dithering (gist-like settings)",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("DitherCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Use fixed SPECTRA6 real-world RGB palette to match the Python gist
	fixedPalette := []color.RGBA{
		{R: 25, G: 30, B: 33, A: 255},
		{R: 232, G: 232, B: 232, A: 255},
		{R: 239, G: 222, B: 68, A: 255},
		{R: 178, G: 19, B: 24, A: 255},
		{R: 33, G: 87, B: 186, A: 255},
		{R: 18, G: 95, B: 32, A: 255},
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Output image
	out := image.NewRGBA(bounds)

	// Implement integer-based Floyd-Steinberg error diffusion (non-serpentine)
	// to more closely match Pillow's internal dithering (uses integer weights).
	// We maintain error buffers as integers scaled by 16.
	const fsScale = 16
	// Weights numerator for FS kernel (sum = 16): right=7, down-left=3, down=5, down-right=1
	const wRight = 7
	const wDownLeft = 3
	const wDown = 5
	const wDownRight = 1

	errCurrR := make([]int, w)
	errCurrG := make([]int, w)
	errCurrB := make([]int, w)
	errNextR := make([]int, w)
	errNextG := make([]int, w)
	errNextB := make([]int, w)

	clamp8 := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}

	// Round-to-nearest when de-scaling accumulated error (scaled by 16)
	roundDiv16 := func(e int) int {
		if e >= 0 {
			return (e + fsScale/2) / fsScale
		}
		return (e - fsScale/2) / fsScale
	}

	// Iterate rows top-to-bottom, left-to-right (no serpentine) to align with Pillow quantize dithering
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			xx := bounds.Min.X + x
			yy := bounds.Min.Y + y

			r16, g16, b16, a16 := img.At(xx, yy).RGBA()
			r8 := int(uint8(r16 >> 8))
			g8 := int(uint8(g16 >> 8))
			b8 := int(uint8(b16 >> 8))
			a8 := int(uint8(a16 >> 8))

			// Composite over white background (unpremultiplied) with rounding
			r0 := clamp8((r8*a8 + 255*(255-a8) + 127) / 255)
			g0 := clamp8((g8*a8 + 255*(255-a8) + 127) / 255)
			b0 := clamp8((b8*a8 + 255*(255-a8) + 127) / 255)

			// Apply accumulated error with rounding to nearest (errors are scaled by 16)
			rAdj := clamp8(r0 + roundDiv16(errCurrR[x]))
			gAdj := clamp8(g0 + roundDiv16(errCurrG[x]))
			bAdj := clamp8(b0 + roundDiv16(errCurrB[x]))

			// Nearest palette color (Euclidean in sRGB)
			bestIdx := 0
			bestDist := int(^uint(0) >> 1)
			for i := 0; i < len(fixedPalette); i++ {
				pr := int(fixedPalette[i].R)
				pg := int(fixedPalette[i].G)
				pb := int(fixedPalette[i].B)
				dr := rAdj - pr
				dg := gAdj - pg
				db := bAdj - pb
				dist := dr*dr + dg*dg + db*db
				if dist < bestDist {
					bestDist = dist
					bestIdx = i
				}
			}

			chosen := fixedPalette[bestIdx]
			out.Set(xx, yy, chosen)

			// Error (unscaled)
			er := rAdj - int(chosen.R)
			eg := gAdj - int(chosen.G)
			eb := bAdj - int(chosen.B)

			// Distribute FS error to neighbors (L->R)
			if x+1 < w {
				errCurrR[x+1] += er * wRight
				errCurrG[x+1] += eg * wRight
				errCurrB[x+1] += eb * wRight
			}
			if y+1 < h {
				if x-1 >= 0 {
					errNextR[x-1] += er * wDownLeft
					errNextG[x-1] += eg * wDownLeft
					errNextB[x-1] += eb * wDownLeft
				}
				errNextR[x] += er * wDown
				errNextG[x] += eg * wDown
				errNextB[x] += eb * wDown
				if x+1 < w {
					errNextR[x+1] += er * wDownRight
					errNextG[x+1] += eg * wDownRight
					errNextB[x+1] += eb * wDownRight
				}
			}
		}

		// Move next-row errors to current and clear next
		errCurrR, errNextR = errNextR, errCurrR
		errCurrG, errNextG = errNextG, errCurrG
		errCurrB, errNextB = errNextB, errCurrB
		for i := 0; i < w; i++ {
			errNextR[i] = 0
			errNextG[i] = 0
			errNextB[i] = 0
		}
	}

	// Encode the dithered image to PNG bytes
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
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
