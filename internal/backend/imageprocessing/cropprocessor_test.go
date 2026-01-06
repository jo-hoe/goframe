package imageprocessing

import (
	"testing"
)

func TestNewCropProcessor_Success(t *testing.T) {
	params := map[string]interface{}{
		"height": 1600,
		"width":  1200,
	}

	processor, err := NewCropProcessor(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	cropProc, ok := processor.(*CropProcessor)
	if !ok {
		t.Fatal("Expected processor to be *CropProcessor")
	}

	if cropProc.GetHeight() != 1600 {
		t.Errorf("Expected height 1600, got %d", cropProc.GetHeight())
	}
	if cropProc.GetWidth() != 1200 {
		t.Errorf("Expected width 1200, got %d", cropProc.GetWidth())
	}
}

func TestNewCropProcessor_MissingHeight(t *testing.T) {
	params := map[string]interface{}{
		"width": 1200,
	}

	_, err := NewCropProcessor(params)
	if err == nil {
		t.Error("Expected error for missing height parameter")
	}
}

func TestNewCropProcessor_MissingWidth(t *testing.T) {
	params := map[string]interface{}{
		"height": 1600,
	}

	_, err := NewCropProcessor(params)
	if err == nil {
		t.Error("Expected error for missing width parameter")
	}
}

func TestNewCropProcessor_InvalidHeight(t *testing.T) {
	tests := []struct {
		name   string
		height interface{}
	}{
		{"Zero height", 0},
		{"Negative height", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"height": tt.height,
				"width":  1200,
			}

			_, err := NewCropProcessor(params)
			if err == nil {
				t.Error("Expected error for invalid height")
			}
		})
	}
}

func TestNewCropProcessor_InvalidWidth(t *testing.T) {
	tests := []struct {
		name  string
		width interface{}
	}{
		{"Zero width", 0},
		{"Negative width", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"height": 1600,
				"width":  tt.width,
			}

			_, err := NewCropProcessor(params)
			if err == nil {
				t.Error("Expected error for invalid width")
			}
		})
	}
}

func TestCropProcessor_Name(t *testing.T) {
	processor, err := NewCropProcessor(map[string]interface{}{
		"height": 1600,
		"width":  1200,
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor.Type() != "CropProcessor" {
		t.Errorf("Expected type 'CropProcessor', got '%s'", processor.Type())
	}
}

func TestCropProcessor_ProcessImage(t *testing.T) {
	processor, err := NewCropProcessor(map[string]interface{}{
		"height": 1600,
		"width":  1200,
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	testData := []byte("test image data")
	result, err := processor.ProcessImage(testData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Since this is a placeholder implementation, it should return the input unchanged
	if string(result) != string(testData) {
		t.Error("Expected result to match input data")
	}
}

func TestCropProcessor_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("CropProcessor") {
		t.Error("Expected CropProcessor to be registered in DefaultRegistry")
	}

	// Test creating via registry
	processor, err := DefaultRegistry.Create("CropProcessor", map[string]interface{}{
		"height": 800,
		"width":  600,
	})
	if err != nil {
		t.Fatalf("Failed to create processor via registry: %v", err)
	}

	cropProc, ok := processor.(*CropProcessor)
	if !ok {
		t.Fatal("Expected processor to be *CropProcessor")
	}

	if cropProc.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", cropProc.GetHeight())
	}
	if cropProc.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", cropProc.GetWidth())
	}
}

func TestCropProcessor_WithFloat64Params(t *testing.T) {
	// YAML unmarshaling often produces float64 for numbers
	params := map[string]interface{}{
		"height": float64(1600),
		"width":  float64(1200),
	}

	processor, err := NewCropProcessor(params)
	if err != nil {
		t.Fatalf("Expected no error with float64 params, got %v", err)
	}

	cropProc, ok := processor.(*CropProcessor)
	if !ok {
		t.Fatal("Expected processor to be *CropProcessor")
	}

	if cropProc.GetHeight() != 1600 {
		t.Errorf("Expected height 1600, got %d", cropProc.GetHeight())
	}
	if cropProc.GetWidth() != 1200 {
		t.Errorf("Expected width 1200, got %d", cropProc.GetWidth())
	}
}
