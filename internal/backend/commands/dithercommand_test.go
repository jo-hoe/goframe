package commands

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// createTestImage creates a simple test image with a gradient
func createTestImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Create a gradient
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			gray := uint8((x * 255) / width) //nolint:gosec // computed gradient is in 0..255 for 0<=x<width
			img.Set(x, y, color.RGBA{gray, gray, gray, 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(fmt.Sprintf("failed to encode test image: %v", err))
	}
	return buf.Bytes()
}

func TestNewDitherParamsFromMap_DefaultBW(t *testing.T) {
	params := map[string]any{}

	ditherParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(ditherParams.PalettePairs) != 2 {
		t.Errorf("Expected default BW palette length 2, got %d", len(ditherParams.PalettePairs))
	}

	// Expect device and dither both Black and White
	blackPair := ditherParams.PalettePairs[0]
	if blackPair.Device.R != 0 || blackPair.Device.G != 0 || blackPair.Device.B != 0 {
		t.Errorf("Expected device black [0,0,0], got %v", blackPair.Device)
	}
	if blackPair.Dither.R != 0 || blackPair.Dither.G != 0 || blackPair.Dither.B != 0 {
		t.Errorf("Expected dither black [0,0,0], got %v", blackPair.Dither)
	}

	whitePair := ditherParams.PalettePairs[1]
	if whitePair.Device.R != 255 || whitePair.Device.G != 255 || whitePair.Device.B != 255 {
		t.Errorf("Expected device white [255,255,255], got %v", whitePair.Device)
	}
	if whitePair.Dither.R != 255 || whitePair.Dither.G != 255 || whitePair.Dither.B != 255 {
		t.Errorf("Expected dither white [255,255,255], got %v", whitePair.Dither)
	}
}

func TestNewDitherParamsFromMap_CustomPalette(t *testing.T) {
	params := map[string]any{
		"palette": []any{
			[]any{[]any{255, 0, 0}, []any{255, 0, 0}},     // Red (device==dither)
			[]any{[]any{0, 255, 0}, []any{0, 255, 0}},     // Green (device==dither)
			[]any{[]any{0, 0, 255}, []any{0, 0, 255}},     // Blue (device==dither)
			[]any{[]any{255, 255, 0}, []any{255, 255, 0}}, // Yellow (device==dither)
		},
	}

	ditherParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(ditherParams.PalettePairs) != 4 {
		t.Errorf("Expected palette length 4, got %d", len(ditherParams.PalettePairs))
	}

	// Check red device and dither
	first := ditherParams.PalettePairs[0]
	if first.Device.R != 255 || first.Device.G != 0 || first.Device.B != 0 {
		t.Errorf("Expected device red [255,0,0], got %v", first.Device)
	}
	if first.Dither.R != 255 || first.Dither.G != 0 || first.Dither.B != 0 {
		t.Errorf("Expected dither red [255,0,0], got %v", first.Dither)
	}
}

func TestNewDitherParamsFromMap_InvalidPalette(t *testing.T) {
	testCases := []struct {
		name    string
		palette any
	}{
		{"not array", "invalid"},
		{"wrong rgb length", []any{[]any{255, 0}}},
		{"invalid rgb value", []any{[]any{256, 0, 0}}},
		{"negative rgb value", []any{[]any{-1, 0, 0}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{
				"palette": tc.palette,
			}

			_, err := NewDitherParamsFromMap(params)
			if err == nil {
				t.Error("Expected error for invalid palette")
			}
		})
	}
}

func TestNewDitherCommand(t *testing.T) {
	params := map[string]any{}

	cmd, err := NewDitherCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cmd.Name() != "DitherCommand" {
		t.Errorf("Expected name DitherCommand, got %s", cmd.Name())
	}
}

func TestDitherCommand_Execute(t *testing.T) {
	imageData := createTestImage(100, 100)

	cmd, err := NewDitherCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	// Verify result is valid PNG
	_, err = png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Errorf("Result is not valid PNG: %v", err)
	}
}

func TestDitherCommand_Execute_WithCustomPalette(t *testing.T) {
	// When a custom palette is provided, the command uses it.
	imageData := createTestImage(100, 100)

	cmd, err := NewDitherCommand(map[string]any{
		"palette": []any{
			[]any{[]any{255, 0, 0}, []any{255, 0, 0}}, // Red (device==dither)
			[]any{[]any{0, 0, 255}, []any{0, 0, 255}}, // Blue (device==dither)
		},
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	// Verify result is valid PNG
	_, err = png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Errorf("Result is not valid PNG: %v", err)
	}
}

func TestDitherCommand_Execute_InvalidImageData(t *testing.T) {
	cmd, err := NewDitherCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	_, err = cmd.Execute([]byte("not a valid image"))
	if err == nil {
		t.Error("Expected error for invalid image data")
	}
}

func TestDitherCommand_GetParams(t *testing.T) {
	params := map[string]any{
		"palette": []any{
			[]any{[]any{255, 0, 0}, []any{255, 0, 0}},
			[]any{[]any{0, 255, 0}, []any{0, 255, 0}},
		},
	}

	cmd, err := NewDitherCommand(params)
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	ditherCmd := cmd.(*DitherCommand)
	retrievedParams := ditherCmd.GetParams()

	if len(retrievedParams.PalettePairs) != 2 {
		t.Errorf("Expected palette length 2, got %d", len(retrievedParams.PalettePairs))
	}
}

func TestDitherCommand_WithRealImage(t *testing.T) {
	// Load real test image
	imageData, err := os.ReadFile("testdata/peppers.png")
	if err != nil {
		t.Fatalf("Failed to load test image: %v", err)
	}

	cmd, err := NewDitherCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	// Verify result is valid PNG
	_, err = png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Errorf("Result is not valid PNG: %v", err)
	}
}

// Ensures the output image contains only device colors defined by the palette pairs,
// even when dithering uses different colors for quantization.
func TestDitherCommand_OutputContainsOnlyDeviceColors(t *testing.T) {
	// Simple grayscale gradient input
	imageData := createTestImage(64, 64)

	// Configure palette pairs with distinct device and dither colors
	params := map[string]any{
		"palette": []any{
			[]any{[]any{0, 0, 0}, []any{25, 30, 33}},          // Device black, dither dark gray
			[]any{[]any{255, 255, 255}, []any{232, 232, 232}}, // Device white, dither off-white
			[]any{[]any{255, 255, 0}, []any{239, 222, 68}},    // Device yellow, dither yellow-ish
			[]any{[]any{0, 0, 255}, []any{33, 87, 186}},       // Device blue, dither blue-ish
			[]any{[]any{255, 0, 0}, []any{178, 19, 24}},       // Device red, dither red-ish
			[]any{[]any{0, 255, 0}, []any{18, 95, 32}},        // Device green, dither green-ish
		},
	}

	cmd, err := NewDitherCommand(params)
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	outImg, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("Failed to decode output png: %v", err)
	}

	// Build set of allowed device colors
	deviceSet := map[[3]uint8]struct{}{
		{0, 0, 0}:       {},
		{255, 255, 255}: {},
		{255, 255, 0}:   {},
		{0, 0, 255}:     {},
		{255, 0, 0}:     {},
		{0, 255, 0}:     {},
	}

	b := outImg.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r16, g16, b16, _ := outImg.At(x, y).RGBA()
			key := [3]uint8{uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8)} //nolint:gosec // values are 16-bit components; shifting >>8 yields 0..255 before conversion
			if _, ok := deviceSet[key]; !ok {
				t.Fatalf("Found non-device color at (%d,%d): %v", x, y, key)
			}
		}
	}
}

// If the image already contains only exact device colors (after alpha-over-white),
// dithering is skipped and the original bytes are returned unchanged.
func TestDitherCommand_SkipWhenAlreadyDeviceColors(t *testing.T) {
	// Create an image that uses only the default device palette (black and white)
	w, h := 16, 16
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode device-only test image: %v", err)
	}
	imageData := buf.Bytes()

	// Use default BW palette pairs
	cmd, err := NewDitherCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !bytes.Equal(result, imageData) {
		t.Fatalf("Expected command to skip processing and return the original bytes unchanged")
	}
}

func TestDitherCommand_Execute_Atkinson(t *testing.T) {
	imageData := createTestImage(64, 64)

	cmd, err := NewDitherCommand(map[string]any{
		"ditheringAlgorithm": "atkinson",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	// Verify result is valid PNG
	_, err = png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Errorf("Result is not valid PNG: %v", err)
	}
}

func TestNewDitherCommand_InvalidDitheringAlgorithm(t *testing.T) {
	_, err := NewDitherCommand(map[string]any{
		"ditheringAlgorithm": "bogus",
	})
	if err == nil {
		t.Error("Expected error for invalid ditheringAlgorithm")
	}
}
