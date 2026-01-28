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

// ColorPair represents a mapping between a device output color and a dithering color.
// - Dither: color used during quantization/error diffusion
// - Device: actual device color to map to for output
type ColorPair struct {
	Device color.RGBA
	Dither color.RGBA
}

// DitherParams represents typed parameters for dither command
type DitherParams struct {
	// PalettePairs contains ordered pairs of [Device, Dither] colors
	PalettePairs []ColorPair
}

// Defaults to black/white with identical device and dithering colors
func defaultBWPalettePairs() []ColorPair {
	return []ColorPair{
		{Device: color.RGBA{R: 0, G: 0, B: 0, A: 255}, Dither: color.RGBA{R: 0, G: 0, B: 0, A: 255}},
		{Device: color.RGBA{R: 255, G: 255, B: 255, A: 255}, Dither: color.RGBA{R: 255, G: 255, B: 255, A: 255}},
	}
}

// NewDitherParamsFromMap creates DitherParams from a generic map
func NewDitherParamsFromMap(params map[string]any) (*DitherParams, error) {
	ditherParams := &DitherParams{}

	if paletteParam, ok := params["palette"]; ok {
		pairs, err := parsePalettePairs(paletteParam)
		if err != nil {
			return nil, fmt.Errorf("invalid palette: %w", err)
		}
		if len(pairs) == 0 {
			return nil, fmt.Errorf("palette must not be empty")
		}
		ditherParams.PalettePairs = pairs
	} else {
		ditherParams.PalettePairs = defaultBWPalettePairs()
	}

	return ditherParams, nil
}

// parsePalettePairs converts the palette configuration into []ColorPair.
// Required format:
//
//	palette:
//	  - [[devR,devG,devB],[dithR,dithG,dithB]]
//	  - ...
func parsePalettePairs(paletteParam any) ([]ColorPair, error) {
	top, ok := paletteParam.([]any)
	if !ok {
		return nil, fmt.Errorf("palette must be an array")
	}

	out := make([]ColorPair, 0, len(top))
	for i, entry := range top {
		switch e := entry.(type) {
		case []any:
			switch len(e) {
			case 2:
				dev, err := toRGBTriple(e[0], i, "device")
				if err != nil {
					return nil, err
				}
				dith, err := toRGBTriple(e[1], i, "dither")
				if err != nil {
					return nil, err
				}
				out = append(out, ColorPair{
					Device: color.RGBA{R: uint8(dev[0]), G: uint8(dev[1]), B: uint8(dev[2]), A: 255},
					Dither: color.RGBA{R: uint8(dith[0]), G: uint8(dith[1]), B: uint8(dith[2]), A: 255},
				})
			default:
				return nil, fmt.Errorf("palette entry at index %d must be a pair [[dev],[dith]]", i)
			}
		default:
			return nil, fmt.Errorf("palette entry at index %d must be an array", i)
		}
	}

	return out, nil
}

// toRGBTriple validates and converts an any into a 3-int RGB triple
func toRGBTriple(val any, parentIdx int, role string) ([3]int, error) {
	arr, ok := val.([]any)
	if !ok {
		return [3]int{}, fmt.Errorf("%s color at index %d must be an array", role, parentIdx)
	}
	if len(arr) != 3 {
		return [3]int{}, fmt.Errorf("%s color at index %d must have exactly 3 values (RGB)", role, parentIdx)
	}
	res := [3]int{}
	for j, v := range arr {
		n, err := numberToByte(v, parentIdx, j)
		if err != nil {
			return [3]int{}, err
		}
		res[j] = n
	}
	return res, nil
}

// numberToByte coerces a numeric value to an int in [0,255], with helpful error messages
func numberToByte(val any, colorIdx, compIdx int) (int, error) {
	switch v := val.(type) {
	case int:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, v)
		}
		return v, nil
	case int8:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case int16:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case int32:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case int64:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case uint:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case uint8:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case uint16:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case uint32:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
	case uint64:
		iv := int(v)
		if iv < 0 || iv > 255 {
			return 0, fmt.Errorf("RGB value at color %d, component %d must be 0-255, got %d", colorIdx, compIdx, iv)
		}
		return iv, nil
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

// DitherCommand handles image dithering and maps to device colors
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

// Execute applies dithering using the dithering palette and outputs the image mapped to device colors
func (c *DitherCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("DitherCommand: dither and map",
		"input_size_bytes", len(imageData))

	// decode
	img, err := decodePNGData(imageData)
	if err != nil {
		slog.Error("DitherCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// extract palettes
	devicePalette, ditherPalette := palettesFromPairs(c.params.PalettePairs)
	if len(devicePalette) == 0 || len(ditherPalette) == 0 || len(devicePalette) != len(ditherPalette) {
		return nil, fmt.Errorf("invalid palettes: device %d, dither %d", len(devicePalette), len(ditherPalette))
	}
	if len(devicePalette) > 0 && len(ditherPalette) > 0 {
		// Log palette sizes and the first pair to verify config ingestion at runtime
		slog.Debug("DitherCommand: using configured palettes",
			"device_count", len(devicePalette),
			"dither_count", len(ditherPalette),
			"first_device", devicePalette[0],
			"first_dither", ditherPalette[0],
		)
	}

	// Optimization: if the image already contains only exact device colors (after alpha compositing over white),
	// skip dithering and mapping entirely and return the original bytes.
	if !needsDitheringAgainst(img, devicePalette) {
		slog.Debug("DitherCommand: image already matches device palette; skipping dithering")
		return imageData, nil
	}

	// perform dithering with quantization against ditherPalette, write devicePalette colors
	outImg, err := ditherAndMapFloydSteinberg(img, ditherPalette, devicePalette)
	if err != nil {
		return nil, err
	}

	// encode
	outBytes, err := encodePNGImage(outImg)
	if err != nil {
		slog.Error("DitherCommand: failed to encode mapped image", "error", err)
		return nil, fmt.Errorf("failed to encode PNG image: %w", err)
	}

	slog.Debug("DitherCommand: complete", "output_size_bytes", len(outBytes))
	return outBytes, nil
}

// decodePNGData decodes PNG bytes into an image.Image
func decodePNGData(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

// palettesFromPairs extracts device and dither palettes from ColorPair slice
func palettesFromPairs(pairs []ColorPair) ([]color.RGBA, []color.RGBA) {
	device := make([]color.RGBA, len(pairs))
	dither := make([]color.RGBA, len(pairs))
	for i, p := range pairs {
		device[i] = p.Device
		dither[i] = p.Dither
	}
	return device, dither
}

// buildPaletteSet constructs a fast lookup set for palette RGB triples
func buildPaletteSet(palette []color.RGBA) map[[3]uint8]struct{} {
	set := make(map[[3]uint8]struct{}, len(palette))
	for _, p := range palette {
		set[[3]uint8{p.R, p.G, p.B}] = struct{}{}
	}
	return set
}

// toColorPalette converts []color.RGBA to a color.Palette for paletted images
func toColorPalette(src []color.RGBA) color.Palette {
	pal := make(color.Palette, len(src))
	for i := range src {
		pal[i] = src[i]
	}
	return pal
}

// needsDitheringAgainst checks if, after alpha compositing over white, all pixels already match
// a given palette color exactly. If so, dithering can be skipped.
func needsDitheringAgainst(img image.Image, palette []color.RGBA) bool {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	paletteSet := buildPaletteSet(palette)

	// Parallel row scan with early exit as soon as a non-palette pixel is found
	found := parallelForStop(h, func(y int) bool {
		yy := bounds.Min.Y + y
		for x := 0; x < w; x++ {
			xx := bounds.Min.X + x

			r16, g16, b16, a16 := img.At(xx, yy).RGBA()
			r8 := int(uint8(r16 >> 8))
			g8 := int(uint8(g16 >> 8))
			b8 := int(uint8(b16 >> 8))
			a8 := int(uint8(a16 >> 8))

			// Composite over white background (same formula used in dithering path)
			r0, g0, b0 := compositeOverWhite(r8, g8, b8, a8)

			if _, ok := paletteSet[[3]uint8{uint8(r0), uint8(g0), uint8(b0)}]; !ok {
				return true // needs dithering
			}
		}
		return false
	})
	return found
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

// ditherAndMapFloydSteinberg applies integer-based Floyd–Steinberg error diffusion (non-serpentine)
// with nearest-color mapping in 8-bit sRGB and alpha compositing over white.
// Quantization (error target) uses ditherPalette; output pixel is written using devicePalette at the chosen index.
func ditherAndMapFloydSteinberg(img image.Image, ditherPalette, devicePalette []color.RGBA) (image.Image, error) {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Output image as paletted with device palette for faster encoding and reduced memory
	out := image.NewPaletted(bounds, toColorPalette(devicePalette))

	errCurrR := make([]int, w)
	errCurrG := make([]int, w)
	errCurrB := make([]int, w)
	errNextR := make([]int, w)
	errNextG := make([]int, w)
	errNextB := make([]int, w)

	// Iterate rows top-to-bottom, left-to-right (no serpentine)
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

			// Nearest palette index against dithering palette (Euclidean in sRGB)
			bestIdx := nearestPaletteIndex(rAdj, gAdj, bAdj, ditherPalette)
			quant := ditherPalette[bestIdx]

			// Error (unscaled) between adjusted source and quantized dither color
			er := rAdj - int(quant.R)
			eg := gAdj - int(quant.G)
			eb := bAdj - int(quant.B)

			// Set output pixel to the corresponding device color index (paletted image)
			out.SetColorIndex(xx, yy, uint8(bestIdx))

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
