package imageprocessing

import (
	"testing"
)

func TestNewScaleCommand_Success(t *testing.T) {
	params := map[string]any{
		"height": 800,
		"width":  600,
	}

	command, err := NewScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	scaleCmd, ok := command.(*ScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *ScaleCommand")
	}

	if scaleCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", scaleCmd.GetWidth())
	}
}

func TestNewScaleCommand_MissingHeight(t *testing.T) {
	params := map[string]any{
		"width": 600,
	}

	_, err := NewScaleCommand(params)
	if err == nil {
		t.Error("Expected error for missing height parameter")
	}
}

func TestNewScaleCommand_MissingWidth(t *testing.T) {
	params := map[string]any{
		"height": 800,
	}

	_, err := NewScaleCommand(params)
	if err == nil {
		t.Error("Expected error for missing width parameter")
	}
}

func TestNewScaleCommand_InvalidHeight(t *testing.T) {
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

			_, err := NewScaleCommand(params)
			if err == nil {
				t.Error("Expected error for invalid height")
			}
		})
	}
}

func TestNewScaleCommand_InvalidWidth(t *testing.T) {
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

			_, err := NewScaleCommand(params)
			if err == nil {
				t.Error("Expected error for invalid width")
			}
		})
	}
}

func TestScaleCommand_Name(t *testing.T) {
	command, err := NewScaleCommand(map[string]any{
		"height": 800,
		"width":  600,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "ScaleCommand" {
		t.Errorf("Expected name 'ScaleCommand', got '%s'", command.Name())
	}
}

func TestScaleCommand_Execute(t *testing.T) {
	command, err := NewScaleCommand(map[string]any{
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

func TestScaleCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("ScaleCommand") {
		t.Error("Expected ScaleCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := DefaultRegistry.Create("ScaleCommand", map[string]any{
		"height": 1024,
		"width":  768,
	})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	scaleCmd, ok := command.(*ScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *ScaleCommand")
	}

	if scaleCmd.GetHeight() != 1024 {
		t.Errorf("Expected height 1024, got %d", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() != 768 {
		t.Errorf("Expected width 768, got %d", scaleCmd.GetWidth())
	}
}

func TestScaleCommand_WithFloat64Params(t *testing.T) {
	// YAML unmarshaling often produces float64 for numbers
	params := map[string]any{
		"height": float64(800),
		"width":  float64(600),
	}

	command, err := NewScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error with float64 params, got %v", err)
	}

	scaleCmd, ok := command.(*ScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *ScaleCommand")
	}

	if scaleCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %d", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %d", scaleCmd.GetWidth())
	}
}

func TestScaleCommand_GetParams(t *testing.T) {
	command, err := NewScaleCommand(map[string]any{
		"height": 1920,
		"width":  1080,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	scaleCmd, ok := command.(*ScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *ScaleCommand")
	}

	params := scaleCmd.GetParams()
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
