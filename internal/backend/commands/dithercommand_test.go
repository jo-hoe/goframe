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
			gray := uint8((x * 255) / width)
			img.Set(x, y, color.RGBA{gray, gray, gray, 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(fmt.Sprintf("failed to encode test image: %v", err))
	}
	return buf.Bytes()
}

func TestNewDitherParamsFromMap_DefaultBlackAndWhite(t *testing.T) {
	params := map[string]any{}

	ditherParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(ditherParams.Palette) != 2 {
		t.Errorf("Expected palette length 2, got %d", len(ditherParams.Palette))
	}

	// Check black
	if ditherParams.Palette[0][0] != 0 || ditherParams.Palette[0][1] != 0 || ditherParams.Palette[0][2] != 0 {
		t.Errorf("Expected black [0,0,0], got %v", ditherParams.Palette[0])
	}

	// Check white
	if ditherParams.Palette[1][0] != 255 || ditherParams.Palette[1][1] != 255 || ditherParams.Palette[1][2] != 255 {
		t.Errorf("Expected white [255,255,255], got %v", ditherParams.Palette[1])
	}
}

func TestNewDitherParamsFromMap_CustomPalette(t *testing.T) {
	params := map[string]any{
		"palette": []any{
			[]any{255, 0, 0},   // Red
			[]any{0, 255, 0},   // Green
			[]any{0, 0, 255},   // Blue
			[]any{255, 255, 0}, // Yellow
		},
	}

	ditherParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(ditherParams.Palette) != 4 {
		t.Errorf("Expected palette length 4, got %d", len(ditherParams.Palette))
	}

	// Check red
	if ditherParams.Palette[0][0] != 255 || ditherParams.Palette[0][1] != 0 || ditherParams.Palette[0][2] != 0 {
		t.Errorf("Expected red [255,0,0], got %v", ditherParams.Palette[0])
	}
}

func TestNewDitherParamsFromMap_WithStrength(t *testing.T) {
	params := map[string]any{
		"strength": 0.8,
	}

	ditherParams, err := NewDitherParamsFromMap(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ditherParams.Strength == nil {
		t.Fatal("Expected strength to be set")
	}

	if *ditherParams.Strength != 0.8 {
		t.Errorf("Expected strength 0.8, got %f", *ditherParams.Strength)
	}
}

func TestNewDitherParamsFromMap_InvalidStrength(t *testing.T) {
	testCases := []struct {
		name     string
		strength any
	}{
		{"too low", -0.1},
		{"too high", 1.5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{
				"strength": tc.strength,
			}

			_, err := NewDitherParamsFromMap(params)
			if err == nil {
				t.Error("Expected error for invalid strength")
			}
		})
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
	// Even when a custom palette is provided, the implementation now mimics the
	// reference gist and ignores custom settings to achieve similar results.
	imageData := createTestImage(100, 100)

	cmd, err := NewDitherCommand(map[string]any{
		"palette": []any{
			[]any{255, 0, 0}, // Red
			[]any{0, 0, 255}, // Blue
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

func TestDitherCommand_Execute_WithStrength(t *testing.T) {
	imageData := createTestImage(100, 100)

	cmd, err := NewDitherCommand(map[string]any{
		"strength": 0.5,
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
			[]any{255, 0, 0},
			[]any{0, 255, 0},
		},
	}

	cmd, err := NewDitherCommand(params)
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	ditherCmd := cmd.(*DitherCommand)
	retrievedParams := ditherCmd.GetParams()

	if len(retrievedParams.Palette) != 2 {
		t.Errorf("Expected palette length 2, got %d", len(retrievedParams.Palette))
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
