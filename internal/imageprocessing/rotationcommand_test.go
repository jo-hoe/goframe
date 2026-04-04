package imageprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

)

func TestNewRotationParams_Valid(t *testing.T) {
	for _, steps := range []int{Steps90, Steps180, Steps270} {
		p, err := NewRotationParams(steps, true)
		if err != nil {
			t.Fatalf("steps=%d: expected no error, got %v", steps, err)
		}
		if p.Steps != steps {
			t.Errorf("steps=%d: got Steps=%d", steps, p.Steps)
		}
	}
}

func TestNewRotationParams_Invalid(t *testing.T) {
	for _, steps := range []int{0, 4, -1} {
		_, err := NewRotationParams(steps, true)
		if err == nil {
			t.Errorf("steps=%d: expected error, got nil", steps)
		}
	}
}

func TestNewRotationCommand_FromMap(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]any
		wantSteps int
		wantCW    bool
	}{
		{
			name:      "defaults",
			params:    map[string]any{},
			wantSteps: Steps90,
			wantCW:    true,
		},
		{
			name:      "180 counterclockwise",
			params:    map[string]any{"steps": 2, "clockwise": false},
			wantSteps: Steps180,
			wantCW:    false,
		},
		{
			name:      "270 clockwise",
			params:    map[string]any{"steps": 3},
			wantSteps: Steps270,
			wantCW:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := NewRotationCommand(tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			rc, ok := cmd.(*RotationCommand)
			if !ok {
				t.Fatal("expected *RotationCommand")
			}
			if rc.GetParams().Steps != tt.wantSteps {
				t.Errorf("want steps=%d, got %d", tt.wantSteps, rc.GetParams().Steps)
			}
			if rc.GetParams().Clockwise != tt.wantCW {
				t.Errorf("want clockwise=%v, got %v", tt.wantCW, rc.GetParams().Clockwise)
			}
		})
	}
}

func TestNewRotationCommand_InvalidSteps(t *testing.T) {
	_, err := NewRotationCommand(map[string]any{"steps": 0})
	if err == nil {
		t.Error("expected error for steps=0")
	}
}

func TestRotationCommand_Name(t *testing.T) {
	cmd, err := NewRotationCommandWithParams(Steps90, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name() != "RotationCommand" {
		t.Errorf("expected 'RotationCommand', got '%s'", cmd.Name())
	}
}

func TestRotationCommand_Execute_InvalidData(t *testing.T) {
	cmd, _ := NewRotationCommandWithParams(Steps90, true)
	_, err := cmd.Execute([]byte("not a png"))
	if err == nil {
		t.Error("expected error for invalid PNG data")
	}
}

func TestRotationCommand_Execute_90Clockwise(t *testing.T) {
	// 3x3 pattern: top-left red, top-right green, bottom-left blue, bottom-right white
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("makeSquarePNGWithPattern: %v", err)
	}

	cmd, _ := NewRotationCommandWithParams(Steps90, true)
	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result is not valid PNG: %v", err)
	}

	// 90° clockwise: (x,y) -> (h-1-y, x), h=3
	// (0,0) red -> (2,0)
	if img.At(2, 0) != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red at (2,0), got %v", img.At(2, 0))
	}
	// (2,0) green -> (2,2)
	if img.At(2, 2) != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("expected green at (2,2), got %v", img.At(2, 2))
	}
}

func TestRotationCommand_Execute_90CounterClockwise(t *testing.T) {
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("makeSquarePNGWithPattern: %v", err)
	}

	cmd, _ := NewRotationCommandWithParams(Steps90, false)
	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result is not valid PNG: %v", err)
	}

	// 90° counterclockwise: (x,y) -> (y, w-1-x), w=3
	// (0,0) red -> (0,2)
	if img.At(0, 2) != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red at (0,2), got %v", img.At(0, 2))
	}
}

func TestRotationCommand_Execute_180(t *testing.T) {
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("makeSquarePNGWithPattern: %v", err)
	}

	cmd, _ := NewRotationCommandWithParams(Steps180, true)
	out, err := cmd.Execute(data)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result is not valid PNG: %v", err)
	}

	// 180°: (0,0) red -> (2,2)
	if img.At(2, 2) != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red at (2,2) after 180°, got %v", img.At(2, 2))
	}
	// (2,2) white -> (0,0)
	if img.At(0, 0) != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("expected white at (0,0) after 180°, got %v", img.At(0, 0))
	}
}

func TestRotationCommand_Execute_270ClockwiseEqualsCounterClockwise90(t *testing.T) {
	data, err := makeSquarePNGWithPattern(3)
	if err != nil {
		t.Fatalf("makeSquarePNGWithPattern: %v", err)
	}

	cw270, _ := NewRotationCommandWithParams(Steps270, true)
	ccw90, _ := NewRotationCommandWithParams(Steps90, false)

	outCW, err := cw270.Execute(data)
	if err != nil {
		t.Fatalf("Execute CW270: %v", err)
	}
	outCCW, err := ccw90.Execute(data)
	if err != nil {
		t.Fatalf("Execute CCW90: %v", err)
	}

	if !bytes.Equal(outCW, outCCW) {
		t.Error("270° clockwise should produce same result as 90° counterclockwise")
	}
}

func TestRotationCommand_Execute_NonSquare(t *testing.T) {
	// Use a 2x4 image (portrait): after 90° CW it should be 4x2 (landscape)
	img := makeRectPNG(t, 2, 4)
	cmd, _ := NewRotationCommandWithParams(Steps90, true)
	out, err := cmd.Execute(img)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	decoded, _ := png.Decode(bytes.NewReader(out))
	b := decoded.Bounds()
	if b.Dx() != 4 || b.Dy() != 2 {
		t.Errorf("expected 4x2 after 90° CW rotation of 2x4, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestRotationCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("RotationCommand") {
		t.Error("expected RotationCommand to be registered in DefaultRegistry")
	}

	cmd, err := DefaultRegistry.Create("RotationCommand", map[string]any{"steps": 2})
	if err != nil {
		t.Fatalf("Create via registry: %v", err)
	}

	rc, ok := cmd.(*RotationCommand)
	if !ok {
		t.Fatal("expected *RotationCommand from registry")
	}
	if rc.GetParams().Steps != Steps180 {
		t.Errorf("expected steps=2, got %d", rc.GetParams().Steps)
	}
}

func TestRotationCommand_WithRealImage(t *testing.T) {
	imageData, err := os.ReadFile("testdata/peppers.png")
	if err != nil {
		t.Fatalf("failed to load test image: %v", err)
	}

	for _, steps := range []int{Steps90, Steps180, Steps270} {
		t.Run("", func(t *testing.T) {
			cmd, _ := NewRotationCommandWithParams(steps, true)
			result, err := cmd.Execute(imageData)
			if err != nil {
				t.Fatalf("steps=%d Execute failed: %v", steps, err)
			}
			if _, err := png.Decode(bytes.NewReader(result)); err != nil {
				t.Errorf("steps=%d result is not valid PNG: %v", steps, err)
			}
		})
	}
}

// makeRectPNG creates a minimal valid PNG of the given width × height.
func makeRectPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("makeRectPNG encode: %v", err)
	}
	return buf.Bytes()
}
