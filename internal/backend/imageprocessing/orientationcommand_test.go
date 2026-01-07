package imageprocessing

import (
	"testing"
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
	if !DefaultRegistry.IsRegistered("OrientationCommand") {
		t.Error("Expected OrientationCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := DefaultRegistry.Create("OrientationCommand", map[string]any{
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
