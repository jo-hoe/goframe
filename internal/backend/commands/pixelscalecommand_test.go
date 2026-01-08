package commands

import (
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"testing"
)

func TestNewPixelScaleCommand_BothDimensions(t *testing.T) {
	params := map[string]any{
		"height": 800,
		"width":  600,
	}

	command, err := NewPixelScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	if scaleCmd.GetHeight() == nil || *scaleCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %v", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() == nil || *scaleCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %v", scaleCmd.GetWidth())
	}
}

func TestNewPixelScaleCommand_OnlyHeight(t *testing.T) {
	params := map[string]any{
		"height": 800,
	}

	command, err := NewPixelScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	if scaleCmd.GetHeight() == nil || *scaleCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %v", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() != nil {
		t.Errorf("Expected width to be nil, got %v", *scaleCmd.GetWidth())
	}
}

func TestNewPixelScaleCommand_OnlyWidth(t *testing.T) {
	params := map[string]any{
		"width": 600,
	}

	command, err := NewPixelScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	if scaleCmd.GetHeight() != nil {
		t.Errorf("Expected height to be nil, got %v", *scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() == nil || *scaleCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %v", scaleCmd.GetWidth())
	}
}

func TestNewPixelScaleCommand_MissingBothDimensions(t *testing.T) {
	params := map[string]any{}

	_, err := NewPixelScaleCommand(params)
	if err == nil {
		t.Error("Expected error when both dimensions are missing")
	}
}

func TestNewPixelScaleCommand_InvalidHeight(t *testing.T) {
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
			}

			_, err := NewPixelScaleCommand(params)
			if err == nil {
				t.Error("Expected error for invalid height")
			}
		})
	}
}

func TestNewPixelScaleCommand_InvalidWidth(t *testing.T) {
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
				"width": tt.width,
			}

			_, err := NewPixelScaleCommand(params)
			if err == nil {
				t.Error("Expected error for invalid width")
			}
		})
	}
}

func TestPixelScaleCommand_Name(t *testing.T) {
	command, err := NewPixelScaleCommand(map[string]any{
		"height": 800,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "PixelScaleCommand" {
		t.Errorf("Expected name 'PixelScaleCommand', got '%s'", command.Name())
	}
}

func TestPixelScaleCommand_Execute_InvalidImage(t *testing.T) {
	command, err := NewPixelScaleCommand(map[string]any{
		"height": 100,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	// Test with invalid image data - should return error
	testData := []byte("not a valid image")
	_, err = command.Execute(testData)
	if err == nil {
		t.Error("Expected error for invalid image data, got nil")
	}
}

func TestPixelScaleCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !commandstructure.DefaultRegistry.IsRegistered("PixelScaleCommand") {
		t.Error("Expected PixelScaleCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry with only height
	command, err := commandstructure.DefaultRegistry.Create("PixelScaleCommand", map[string]any{
		"height": 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	if scaleCmd.GetHeight() == nil || *scaleCmd.GetHeight() != 1024 {
		t.Errorf("Expected height 1024, got %v", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() != nil {
		t.Errorf("Expected width to be nil, got %v", *scaleCmd.GetWidth())
	}
}

func TestPixelScaleCommand_WithFloat64Params(t *testing.T) {
	// YAML unmarshaling often produces float64 for numbers
	params := map[string]any{
		"height": float64(800),
		"width":  float64(600),
	}

	command, err := NewPixelScaleCommand(params)
	if err != nil {
		t.Fatalf("Expected no error with float64 params, got %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	if scaleCmd.GetHeight() == nil || *scaleCmd.GetHeight() != 800 {
		t.Errorf("Expected height 800, got %v", scaleCmd.GetHeight())
	}
	if scaleCmd.GetWidth() == nil || *scaleCmd.GetWidth() != 600 {
		t.Errorf("Expected width 600, got %v", scaleCmd.GetWidth())
	}
}

func TestPixelScaleCommand_GetParams(t *testing.T) {
	command, err := NewPixelScaleCommand(map[string]any{
		"height": 1920,
		"width":  1080,
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	scaleCmd, ok := command.(*PixelScaleCommand)
	if !ok {
		t.Fatal("Expected command to be *PixelScaleCommand")
	}

	params := scaleCmd.GetParams()
	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Height == nil || *params.Height != 1920 {
		t.Errorf("Expected height 1920, got %v", params.Height)
	}
	if params.Width == nil || *params.Width != 1080 {
		t.Errorf("Expected width 1080, got %v", params.Width)
	}
}

func TestPixelScaleCommand_PartialParams(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{
			name:        "Only height provided",
			params:      map[string]any{"height": 500},
			expectError: false,
		},
		{
			name:        "Only width provided",
			params:      map[string]any{"width": 300},
			expectError: false,
		},
		{
			name:        "Both provided",
			params:      map[string]any{"height": 500, "width": 300},
			expectError: false,
		},
		{
			name:        "Neither provided",
			params:      map[string]any{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPixelScaleCommand(tt.params)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
