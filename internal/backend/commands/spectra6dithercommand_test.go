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

func TestNewSpectra6DitheringCommand(t *testing.T) {
	params := map[string]any{}

	cmd, err := NewSpectra6DitheringCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cmd.Name() != "Spectra6DitheringCommand" {
		t.Errorf("Expected name Spectra6DitheringCommand, got %s", cmd.Name())
	}
}

func TestSpectra6DitheringCommand_Execute(t *testing.T) {
	imageData := createTestImage(100, 100)

	cmd, err := NewSpectra6DitheringCommand(map[string]any{})
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

func TestSpectra6DitheringCommand_Execute_ColorsAreSpectra6(t *testing.T) {
	imageData := createTestImage(100, 100)

	cmd, err := NewSpectra6DitheringCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(imageData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify result is valid PNG
	img, err := png.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("Result is not valid PNG: %v", err)
	}

	// Allowed Spectra6 colors: Black, White, Red, Yellow, Blue, Green
	isAllowed := func(r8, g8, b8 uint8) bool {
		switch {
		case r8 == 0 && g8 == 0 && b8 == 0: // Black
			return true
		case r8 == 255 && g8 == 255 && b8 == 255: // White
			return true
		case r8 == 255 && g8 == 0 && b8 == 0: // Red
			return true
		case r8 == 255 && g8 == 255 && b8 == 0: // Yellow
			return true
		case r8 == 0 && g8 == 0 && b8 == 255: // Blue
			return true
		case r8 == 0 && g8 == 255 && b8 == 0: // Green
			return true
		default:
			return false
		}
	}

	// Check that the image only contains allowed colors
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Convert to 8-bit
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

			if !isAllowed(r8, g8, b8) {
				t.Errorf("Found unexpected color at (%d,%d): RGB(%d,%d,%d)", x, y, r8, g8, b8)
				return
			}
		}
	}
}

func TestSpectra6DitheringCommand_Execute_InvalidImageData(t *testing.T) {
	cmd, err := NewSpectra6DitheringCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	_, err = cmd.Execute([]byte("not a valid image"))
	if err == nil {
		t.Error("Expected error for invalid image data")
	}
}

func TestSpectra6DitheringCommand_WithRealImage(t *testing.T) {
	// Load real test image
	imageData, err := os.ReadFile("testdata/peppers.png")
	if err != nil {
		t.Fatalf("Failed to load test image: %v", err)
	}

	cmd, err := NewSpectra6DitheringCommand(map[string]any{})
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

// New test: verify skipping dithering when image contains only Spectra6 colors within tolerance
func TestSpectra6DitheringCommand_SkipIfPaletteColorsOnly(t *testing.T) {
	// Construct an image that uses only allowed Spectra6 colors exactly
	img := image.NewRGBA(image.Rect(0, 0, 6, 1))
	allowed := []color.RGBA{
		{0, 0, 0, 255},       // Black
		{255, 255, 255, 255}, // White
		{255, 0, 0, 255},     // Red
		{255, 255, 0, 255},   // Yellow
		{0, 0, 255, 255},     // Blue
		{0, 255, 0, 255},     // Green
	}
	for x := 0; x < 6; x++ {
		img.Set(x, 0, allowed[x])
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test image: %v", err)
	}
	original := buf.Bytes()

	cmd, err := NewSpectra6DitheringCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := cmd.Execute(original)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Since image contains only allowed colors, command should skip processing and return original bytes
	if !bytes.Equal(result, original) {
		t.Errorf("Expected output to equal input when image already contains Spectra6 colors")
	}

	// Still valid PNG
	if _, err := png.Decode(bytes.NewReader(result)); err != nil {
		t.Fatalf("Result is not valid PNG: %v", err)
	}
}
