package commands

import (
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"testing"
)

func TestNewImageConverterCommand_Success(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		expected string
	}{
		{
			name:     "PNG target",
			params:   map[string]any{"targetType": "png"},
			expected: "png",
		},
		{
			name:     "JPEG target",
			params:   map[string]any{"targetType": "jpeg"},
			expected: "jpeg",
		},
		{
			name:     "JPG target (normalized to jpeg)",
			params:   map[string]any{"targetType": "jpg"},
			expected: "jpeg",
		},
		{
			name:     "GIF target",
			params:   map[string]any{"targetType": "gif"},
			expected: "gif",
		},
		{
			name:     "Default target",
			params:   map[string]any{},
			expected: "png",
		},
		{
			name:     "Case insensitive",
			params:   map[string]any{"targetType": "PNG"},
			expected: "png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := NewImageConverterCommand(tt.params)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			converterCmd, ok := command.(*ImageConverterCommand)
			if !ok {
				t.Fatal("Expected command to be *ImageConverterCommand")
			}

			if converterCmd.GetTargetType() != tt.expected {
				t.Errorf("Expected target type '%s', got '%s'", tt.expected, converterCmd.GetTargetType())
			}
		})
	}
}

func TestNewImageConverterCommand_InvalidTargetType(t *testing.T) {
	params := map[string]any{
		"targetType": "bmp",
	}

	_, err := NewImageConverterCommand(params)
	if err == nil {
		t.Error("Expected error for invalid target type")
	}
}

func TestImageConverterCommand_Name(t *testing.T) {
	command, err := NewImageConverterCommand(map[string]any{
		"targetType": "png",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "ImageConverterCommand" {
		t.Errorf("Expected name 'ImageConverterCommand', got '%s'", command.Name())
	}
}

func TestImageConverterCommand_Execute(t *testing.T) {
	command, err := NewImageConverterCommand(map[string]any{
		"targetType": "png",
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

func TestImageConverterCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !commandstructure.DefaultRegistry.IsRegistered("ImageConverterCommand") {
		t.Error("Expected ImageConverterCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := commandstructure.DefaultRegistry.Create("ImageConverterCommand", map[string]any{
		"targetType": "jpeg",
	})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	converterCmd, ok := command.(*ImageConverterCommand)
	if !ok {
		t.Fatal("Expected command to be *ImageConverterCommand")
	}

	if converterCmd.GetTargetType() != "jpeg" {
		t.Errorf("Expected target type 'jpeg', got '%s'", converterCmd.GetTargetType())
	}
}

func TestImageConverterCommand_GetParams(t *testing.T) {
	command, err := NewImageConverterCommand(map[string]any{
		"targetType": "gif",
	})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	converterCmd, ok := command.(*ImageConverterCommand)
	if !ok {
		t.Fatal("Expected command to be *ImageConverterCommand")
	}

	params := converterCmd.GetParams()
	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.TargetType != "gif" {
		t.Errorf("Expected target type 'gif', got '%s'", params.TargetType)
	}
}
