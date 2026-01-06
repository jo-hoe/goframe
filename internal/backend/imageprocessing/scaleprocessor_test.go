package imageprocessing

import (
	"testing"
)

func TestNewScaleProcessor_Success(t *testing.T) {
	params := map[string]any{
		"height": 800,
		"width":  600,
	}

	processor, err := NewScaleProcessor(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	scaleProc, ok := processor.(*ScaleProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ScaleProcessor")
	}

	if scaleProc.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", scaleProc.GetHeight())
	}
	if scaleProc.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", scaleProc.GetWidth())
	}
}

func TestNewScaleProcessor_MissingHeight(t *testing.T) {
	params := map[string]any{
		"width": 600,
	}

	_, err := NewScaleProcessor(params)
	if err == nil {
		t.Error("Expected error for missing height parameter")
	}
}

func TestNewScaleProcessor_MissingWidth(t *testing.T) {
	params := map[string]any{
		"height": 800,
	}

	_, err := NewScaleProcessor(params)
	if err == nil {
		t.Error("Expected error for missing width parameter")
	}
}

func TestNewScaleProcessor_InvalidHeight(t *testing.T) {
	tests := []struct {
		name   string
		height any
	}{
		{"Zero height", 0},
		{"Negative height", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]any{
				"height": tt.height,
				"width":  600,
			}

			_, err := NewScaleProcessor(params)
			if err == nil {
				t.Error("Expected error for invalid height")
			}
		})
	}
}

func TestNewScaleProcessor_InvalidWidth(t *testing.T) {
	tests := []struct {
		name  string
		width any
	}{
		{"Zero width", 0},
		{"Negative width", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]any{
				"height": 800,
				"width":  tt.width,
			}

			_, err := NewScaleProcessor(params)
			if err == nil {
				t.Error("Expected error for invalid width")
			}
		})
	}
}

func TestScaleProcessor_Type(t *testing.T) {
	processor, err := NewScaleProcessor(map[string]any{
		"height": 800,
		"width":  600,
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor.Type() != "ScaleProcessor" {
		t.Errorf("Expected type 'ScaleProcessor', got '%s'", processor.Type())
	}
}

func TestScaleProcessor_ProcessImage(t *testing.T) {
	processor, err := NewScaleProcessor(map[string]any{
		"height": 100,
		"width":  100,
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	// Test with invalid image data - should return error
	t.Run("Invalid image data", func(t *testing.T) {
		testData := []byte("test image data")
		_, err := processor.ProcessImage(testData)
		if err == nil {
			t.Error("Expected error for invalid image data, got nil")
		}
	})

	// Note: Testing with real image data would require creating/loading actual image files
	// For now, we test error handling. Integration tests with real images should be added separately.
}

func TestScaleProcessor_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("ScaleProcessor") {
		t.Error("Expected ScaleProcessor to be registered in DefaultRegistry")
	}

	// Test creating via registry
	processor, err := DefaultRegistry.Create("ScaleProcessor", map[string]any{
		"height": 1024,
		"width":  768,
	})
	if err != nil {
		t.Fatalf("Failed to create processor via registry: %v", err)
	}

	scaleProc, ok := processor.(*ScaleProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ScaleProcessor")
	}

	if scaleProc.GetHeight() != 1024 {
		t.Errorf("Expected height 1024, got %d", scaleProc.GetHeight())
	}
	if scaleProc.GetWidth() != 768 {
		t.Errorf("Expected width 768, got %d", scaleProc.GetWidth())
	}
}

func TestScaleProcessor_WithFloat64Params(t *testing.T) {
	// YAML unmarshaling often produces float64 for numbers
	params := map[string]any{
		"height": float64(800),
		"width":  float64(600),
	}

	processor, err := NewScaleProcessor(params)
	if err != nil {
		t.Fatalf("Expected no error with float64 params, got %v", err)
	}

	scaleProc, ok := processor.(*ScaleProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ScaleProcessor")
	}

	if scaleProc.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", scaleProc.GetHeight())
	}
	if scaleProc.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", scaleProc.GetWidth())
	}
}

func TestScaleProcessor_GetParams(t *testing.T) {
	processor, err := NewScaleProcessor(map[string]any{
		"height": 1920,
		"width":  1080,
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	scaleProc, ok := processor.(*ScaleProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ScaleProcessor")
	}

	params := scaleProc.GetParams()
	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Height != 1920 {
		t.Errorf("Expected height 1920, got %d", params.Height)
	}
	if params.Width != 1080 {
		t.Errorf("Expected width 1080, got %d", params.Width)
	}
}
