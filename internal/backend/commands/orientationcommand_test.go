package commands

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
)

func TestNewOrientationCommand_Success(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		expected string
	}{
		{
			name:     "Portrait orientation",
			params:   map[string]any{"orientation": "portrait"},
			expected: "portrait",
		},
		{
			name:     "Landscape orientation",
			params:   map[string]any{"orientation": "landscape"},
			expected: "landscape",
		},
		{
			name:     "Default orientation",
			params:   map[string]any{},
			expected: "portrait",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := NewOrientationCommand(tt.params)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			orientationCmd, ok := command.(*OrientationCommand)
			if !ok {
				t.Fatal("Expected command to be *OrientationCommand")
			}

			if orientationCmd.GetOrientation() != tt.expected {
				t.Errorf("Expected orientation '%s', got '%s'", tt.expected, orientationCmd.GetOrientation())
			}
		})
	}
}

// Helpers for square image tests
func makeSquarePNGWithPattern(size int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Initialize all pixels to mid gray
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, color.RGBA{128, 128, 128, 255})
		}
	}

	// Distinctive corners and center to detect rotation
	img.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})               // top-left: red
	img.SetRGBA(size-1, 0, color.RGBA{0, 255, 0, 255})          // top-right: green
	img.SetRGBA(0, size-1, color.RGBA{0, 0, 255, 255})          // bottom-left: blue
	img.SetRGBA(size-1, size-1, color.RGBA{255, 255, 255, 255}) // bottom-right: white
	img.SetRGBA(size/2, size/2, color.RGBA{0, 0, 0, 255})       // center: black

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestOrientationCommand_Square_NoRotate_Default(t *testing.T) {
	// Default rotateWhenSquare=false; no rotation expected on square images
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("failed to build test PNG: %v", err)
	}

	cmd, err := NewOrientationCommand(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}

	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should return original bytes without re-encoding/rotation
	if !bytes.Equal(out, data) {
		t.Fatalf("expected identical bytes when rotateWhenSquare=false")
	}
}

func TestOrientationCommand_Square_Rotate_Clockwise_Default(t *testing.T) {
	// rotateWhenSquare=true; clockwise default true
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("failed to build test PNG: %v", err)
	}

	cmd, err := NewOrientationCommand(map[string]any{
		"orientation":      "portrait",
		"rotateWhenSquare": true,
	})
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}

	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decoded result is not valid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 3 || b.Dy() != 3 {
		t.Fatalf("expected 3x3 image, got %dx%d", b.Dx(), b.Dy())
	}

	// Clockwise mapping (x,y)->(h-1-y, x) with h=3:
	// (0,0) red -> (2,0)
	r := img.At(2, 0)
	if r != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("expected red at (2,0) after clockwise rotate, got %v", r)
	}
	// (2,0) green -> (2,2)
	g := img.At(2, 2)
	if g != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("expected green at (2,2) after clockwise rotate, got %v", g)
	}
}

func TestOrientationCommand_Square_Rotate_CounterClockwise(t *testing.T) {
	// rotateWhenSquare=true; clockwise=false => counterclockwise
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("failed to build test PNG: %v", err)
	}

	cmd, err := NewOrientationCommand(map[string]any{
		"orientation":      "portrait",
		"rotateWhenSquare": true,
		"clockwise":        false,
	})
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}

	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decoded result is not valid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 3 || b.Dy() != 3 {
		t.Fatalf("expected 3x3 image, got %dx%d", b.Dx(), b.Dy())
	}

	// Counterclockwise mapping (x,y)->(y, w-1-x) with w=3:
	// (0,0) red -> (0,2)
	r := img.At(0, 2)
	if r != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("expected red at (0,2) after counterclockwise rotate, got %v", r)
	}
}

func TestNewOrientationCommand_InvalidOrientation(t *testing.T) {
	params := map[string]any{
		"orientation": "invalid",
	}

	_, err := NewOrientationCommand(params)
	if err == nil {
		t.Error("Expected error for invalid orientation")
	}
}

func TestOrientationCommand_Name(t *testing.T) {
	command, err := NewOrientationCommand(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "OrientationCommand" {
		t.Errorf("Expected name 'OrientationCommand', got '%s'", command.Name())
	}
}

func TestOrientationCommand_Execute(t *testing.T) {
	command, err := NewOrientationCommand(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	// Test with invalid image data - should return error
	t.Run("Invalid image data", func(t *testing.T) {
		testData := []byte("test image data")
		_, err := command.Execute(testData)
		if err == nil {
			t.Error("Expected error for invalid image data, got nil")
		}
	})

	// Note: Testing with real image data would require creating/loading actual image files
	// For now, we test error handling. Integration tests with real images should be added separately.
}

func TestOrientationCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !commandstructure.DefaultRegistry.IsRegistered("OrientationCommand") {
		t.Error("Expected OrientationCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := commandstructure.DefaultRegistry.Create("OrientationCommand", map[string]any{
		"orientation": "landscape",
	})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	orientationCmd, ok := command.(*OrientationCommand)
	if !ok {
		t.Fatal("Expected command to be *OrientationCommand")
	}

	if orientationCmd.GetOrientation() != "landscape" {
		t.Errorf("Expected orientation 'landscape', got '%s'", orientationCmd.GetOrientation())
	}
}

func TestOrientationCommand_WithRealImage(t *testing.T) {
	// Load real test image
	imageData, err := os.ReadFile("testdata/peppers.png")
	if err != nil {
		t.Fatalf("Failed to load test image: %v", err)
	}

	command, err := NewOrientationCommand(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	result, err := command.Execute(imageData)
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
