package imageprocessing

import (
	"testing"
)

func TestNewCropCommand_Success(t *testing.T) {
	params := map[string]any{
		"height": 1600,
		"width":  1200,
	}

	command, err := NewCropCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	cropCmd, ok := command.(*CropCommand)
	if !ok {
		t.Fatal("Expected command to be *CropCommand")
	}

	if cropCmd.GetHeight() != 1600 {
		t.Errorf("Expected height 1600, got %d", cropCmd.GetHeight())
	}
	if cropCmd.GetWidth() != 1200 {
		t.Errorf("Expected width 1200, got %d", cropCmd.GetWidth())
	}
}

func TestNewCropCommand_MissingHeight(t *testing.T) {
	params := map[string]any{
		"width": 1200,
	}

	_, err := NewCropCommand(params)
	if err == nil {
		t.Error("Expected error for missing height parameter")
	}
}

func TestNewCropCommand_MissingWidth(t *testing.T) {
	params := map[string]any{
		"height": 1600,
	}

	_, err := NewCropCommand(params)
	if err == nil {
		t.Error("Expected error for missing width parameter")
	}
}

func TestNewCropCommand_InvalidHeight(t *testing.T) {
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
				"width":  1200,
			}

			_, err := NewCropCommand(params)
			if err == nil {
				t.Error("Expected error for invalid height")
			}
		})
	}
}

func TestNewCropCommand_InvalidWidth(t *testing.T) {
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
				"height": 1600,
				"width":  tt.width,
			}

			_, err := NewCropCommand(params)
			if err == nil {
				t.Error("Expected error for invalid width")
			}
		})
	}
}

func TestCropCommand_Name(t *testing.T) {
	command, err := NewCropCommand(map[string]any{
		"height": 1600,
		"width":  1200,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "CropCommand" {
		t.Errorf("Expected name 'CropCommand', got '%s'", command.Name())
	}
}

func TestCropCommand_Execute(t *testing.T) {
	command, err := NewCropCommand(map[string]any{
		"height": 100,
		"width":  100,
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

func TestCropCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("CropCommand") {
		t.Error("Expected CropCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := DefaultRegistry.Create("CropCommand", map[string]any{
		"height": 800,
		"width":  600,
	})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	cropCmd, ok := command.(*CropCommand)
	if !ok {
		t.Fatal("Expected command to be *CropCommand")
	}

	if cropCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", cropCmd.GetHeight())
	}
	if cropCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", cropCmd.GetWidth())
	}
}

func TestCropCommand_WithFloat64Params(t *testing.T) {
	// YAML unmarshaling often produces float64 for numbers
	params := map[string]any{
		"height": float64(1600),
		"width":  float64(1200),
	}

	command, err := NewCropCommand(params)
	if err != nil {
		t.Fatalf("Expected no error with float64 params, got %v", err)
	}

	cropCmd, ok := command.(*CropCommand)
	if !ok {
		t.Fatal("Expected command to be *CropCommand")
	}

	if cropCmd.GetHeight() != 1600 {
		t.Errorf("Expected height 1600, got %d", cropCmd.GetHeight())
	}
	if cropCmd.GetWidth() != 1200 {
		t.Errorf("Expected width 1200, got %d", cropCmd.GetWidth())
	}
}
