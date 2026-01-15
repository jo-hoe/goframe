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

const (
	// Floyd-Steinberg diffusion constants reused across helpers
	floydSteinbergScale = 16
	wRight              = 7
	wDownLeft           = 3
	wDown               = 5
	wDownRight          = 1
)

var defaultSpectra6 = [][]int{
	{25, 30, 33},    // Black
	{232, 232, 232}, // White
	{239, 222, 68},  // Yellow
	{178, 19, 24},   // Red
	{33, 87, 186},   // Blue
	{18, 95, 32},    // Green
}

// DitherParams represents typed parameters for dither command
type DitherParams struct {
	// Palette used for dithering
	Palette [][]int
}

// NewDitherParamsFromMap creates DitherParams from a generic map
func NewDitherParamsFromMap(params map[string]any) (*DitherParams, error) {
	ditherParams := &DitherParams{}

	if paletteParam, ok := params["palette"]; ok {
		palette, err := parsePalette(paletteParam)
		if err != nil {
			return nil, fmt.Errorf("invalid palette: %w", err)
		}
		ditherParams.Palette = palette
	} else {
		ditherParams.Palette = defaultSpectra6
	}

	return ditherParams, nil
}

// parsePalette converts various palette formats to [][]int with reduced cyclomatic complexity
func parsePalette(paletteParam any) ([][]int, error) {
	switch p := paletteParam.(type) {
	case []any:
		return convertAnyPalette(p)
	case [][]int:
		// Already in correct format
		return p, nil
	default:
		return nil, fmt.Errorf("palette must be an array of RGB arrays")
	}
}

// convertAnyPalette converts a []any palette where each entry is an RGB triple to [][]int
func convertAnyPalette(p []any) ([][]int, error) {
	palette := make([][]int, len(p))
	for i, colorParam := range p {
		rgb, err := toRGB(colorParam, i)
		if err != nil {
			return nil, err
		}
		palette[i] = rgb
	}
	return palette, nil
}

// toRGB validates and converts a single palette entry to an RGB triple ([]int{R,G,B})
func toRGB(colorParam any, idx int) ([]int, error) {
	colorSlice, ok := colorParam.([]any)
	if !ok {
		return nil, fmt.Errorf("color at index %d must be an array", idx)
	}
	if len(colorSlice) != 3 {
		return nil, fmt.Errorf("color at index %d must have exactly 3 values (RGB)", idx)
	}

	rgb := make([]int, 3)
	for j, val := range colorSlice {
		n, err := numberToByte(val, idx, j)
		if err != nil {
			return nil, err
		}
		rgb[j] = n
	}
	return rgb, nil
}

// numberToByte coerces a numeric value to an int in [0,255], with helpful error messages
func numberToByte(val any, colorIdx, compIdx int) (int, error) {
	switch v := val.(type) {
	case int:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, v)
		}
		return v, nil
	case float64:
		intVal := int(v)
		if intVal < 0 || intVal > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, intVal)
		}
		return intVal, nil
	default:
		return 0, fmt.Errorf("RGB value at color %d, component %d must be a number", colorIdx, compIdx)
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
	slog.Debug("DitherCommand: dithering",
		"input_size_bytes", len(imageData))

	// decode
	img, err := decodePNGData(imageData)
	if err != nil {
		slog.Error("DitherCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// select palette
	palette, err := c.selectedPaletteRGBA()
	if err != nil {
		return nil, err
	}

	// If the image already uses only palette colors (after alpha compositing over white), skip dithering
	if !needsDithering(img, palette) {
		slog.Info("DitherCommand: image already matches palette; skipping dithering")
		return imageData, nil
	}

	// dither
	outImg, err := ditherImageFloydSteinberg(img, palette)
	if err != nil {
		return nil, err
	}

	// encode
	outBytes, err := encodePNGImage(outImg)
	if err != nil {
		slog.Error("DitherCommand: failed to encode dithered image", "error", err)
		return nil, fmt.Errorf("failed to encode dithered PNG image: %w", err)
	}

	slog.Debug("DitherCommand: dithering complete", "output_size_bytes", len(outBytes))
	return outBytes, nil
}

// decodePNGData decodes PNG bytes into an image.Image
func decodePNGData(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

// selectedPaletteRGBA returns the configured palette as []color.RGBA with simplified control flow
func (c *DitherCommand) selectedPaletteRGBA() ([]color.RGBA, error) {
	palette := c.params.Palette
	out := make([]color.RGBA, len(palette))
	for i, rgb := range palette {
		if len(rgb) != 3 {
			return nil, fmt.Errorf("palette color at index %d must have exactly 3 values (RGB)", i)
		}
		out[i] = color.RGBA{
			R: uint8(rgb[0]),
			G: uint8(rgb[1]),
			B: uint8(rgb[2]),
			A: 255,
		}
	}
	return out, nil
}

// clamp8Int ensures an int is within 0..255
func clamp8Int(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// compositeOverWhite composites unpremultiplied RGBA over white using rounding, returning 8-bit RGB
func compositeOverWhite(r8, g8, b8, a8 int) (int, int, int) {
	r0 := clamp8Int((r8*a8 + 255*(255-a8) + 127) / 255)
	g0 := clamp8Int((g8*a8 + 255*(255-a8) + 127) / 255)
	b0 := clamp8Int((b8*a8 + 255*(255-a8) + 127) / 255)
	return r0, g0, b0
}

// buildPaletteSet constructs a fast lookup set for palette RGB triples
func buildPaletteSet(palette []color.RGBA) map[[3]uint8]struct{} {
	set := make(map[[3]uint8]struct{}, len(palette))
	for _, p := range palette {
		set[[3]uint8{p.R, p.G, p.B}] = struct{}{}
	}
	return set
}

// nearestPaletteIndex returns index of the nearest palette color by Euclidean distance in sRGB
func nearestPaletteIndex(r, g, b int, palette []color.RGBA) int {
	bestIdx := 0
	bestDist := int(^uint(0) >> 1)
	for i := 0; i < len(palette); i++ {
		dr := r - int(palette[i].R)
		dg := g - int(palette[i].G)
		db := b - int(palette[i].B)
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	return bestIdx
}

// roundDiv16FloydSteinberg rounds an accumulated error scaled by 16 to nearest integer
func roundDiv16FloydSteinberg(e int) int {
	if e >= 0 {
		return (e + floydSteinbergScale/2) / floydSteinbergScale
	}
	return (e - floydSteinbergScale/2) / floydSteinbergScale
}

// distributeFloydSteinbergError applies Floyd–Steinberg error distribution from pixel (x,y)
func distributeFloydSteinbergError(x, y, w, h int, er, eg, eb int,
	errCurrR, errCurrG, errCurrB, errNextR, errNextG, errNextB []int) {
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

// needsDithering checks if, after alpha compositing over white, all pixels already match a palette color exactly.
// If so, dithering can be skipped.
func needsDithering(img image.Image, fixedPalette []color.RGBA) bool {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	paletteSet := buildPaletteSet(fixedPalette)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			xx := bounds.Min.X + x
			yy := bounds.Min.Y + y

			r16, g16, b16, a16 := img.At(xx, yy).RGBA()
			r8 := int(uint8(r16 >> 8))
			g8 := int(uint8(g16 >> 8))
			b8 := int(uint8(b16 >> 8))
			a8 := int(uint8(a16 >> 8))

			// Composite over white background (same formula used in dithering)
			r0, g0, b0 := compositeOverWhite(r8, g8, b8, a8)

			if _, ok := paletteSet[[3]uint8{uint8(r0), uint8(g0), uint8(b0)}]; !ok {
				return true // needs dithering
			}
		}
	}
	return false // all pixels already in palette
}

// ditherImageFloydSteinberg applies integer-based Floyd–Steinberg error diffusion (non-serpentine)
// with nearest-color mapping in 8-bit sRGB and alpha compositing over white.
func ditherImageFloydSteinberg(img image.Image, fixedPalette []color.RGBA) (image.Image, error) {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Output image
	out := image.NewRGBA(bounds)

	errCurrR := make([]int, w)
	errCurrG := make([]int, w)
	errCurrB := make([]int, w)
	errNextR := make([]int, w)
	errNextG := make([]int, w)
	errNextB := make([]int, w)

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
			r0, g0, b0 := compositeOverWhite(r8, g8, b8, a8)

			// Apply accumulated error (scaled by 16) with rounding to nearest
			rAdj := clamp8Int(r0 + roundDiv16FloydSteinberg(errCurrR[x]))
			gAdj := clamp8Int(g0 + roundDiv16FloydSteinberg(errCurrG[x]))
			bAdj := clamp8Int(b0 + roundDiv16FloydSteinberg(errCurrB[x]))

			// Nearest palette color (Euclidean in sRGB)
			bestIdx := nearestPaletteIndex(rAdj, gAdj, bAdj, fixedPalette)
			chosen := fixedPalette[bestIdx]
			out.Set(xx, yy, chosen)

			// Error (unscaled) between adjusted source and chosen
			er := rAdj - int(chosen.R)
			eg := gAdj - int(chosen.G)
			eb := bAdj - int(chosen.B)

			// Distribute Floyd-Steinberg error to neighbors (L->R)
			distributeFloydSteinbergError(x, y, w, h, er, eg, eb, errCurrR, errCurrG, errCurrB, errNextR, errNextG, errNextB)
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

	return out, nil
}

// encodePNGImage encodes an image.Image to PNG bytes
func encodePNGImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
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
