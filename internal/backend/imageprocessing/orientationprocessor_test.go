package imageprocessing

import (
	"testing"
)

func TestNewOrientationProcessor_Success(t *testing.T) {
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
			processor, err := NewOrientationProcessor(tt.params)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			orientationProc, ok := processor.(*OrientationProcessor)
			if !ok {
				t.Fatal("Expected processor to be *OrientationProcessor")
			}

			if orientationProc.GetOrientation() != tt.expected {
				t.Errorf("Expected orientation '%s', got '%s'", tt.expected, orientationProc.GetOrientation())
			}
		})
	}
}

func TestNewOrientationProcessor_InvalidOrientation(t *testing.T) {
	params := map[string]any{
		"orientation": "invalid",
	}

	_, err := NewOrientationProcessor(params)
	if err == nil {
		t.Error("Expected error for invalid orientation")
	}
}

func TestOrientationProcessor_Name(t *testing.T) {
	processor, err := NewOrientationProcessor(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor.Type() != "OrientationProcessor" {
		t.Errorf("Expected type 'OrientationProcessor', got '%s'", processor.Type())
	}
}

func TestOrientationProcessor_ProcessImage(t *testing.T) {
	processor, err := NewOrientationProcessor(map[string]any{
		"orientation": "portrait",
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

func TestOrientationProcessor_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("OrientationProcessor") {
		t.Error("Expected OrientationProcessor to be registered in DefaultRegistry")
	}

	// Test creating via registry
	processor, err := DefaultRegistry.Create("OrientationProcessor", map[string]any{
		"orientation": "landscape",
	})
	if err != nil {
		t.Fatalf("Failed to create processor via registry: %v", err)
	}

	orientationProc, ok := processor.(*OrientationProcessor)
	if !ok {
		t.Fatal("Expected processor to be *OrientationProcessor")
	}

	if orientationProc.GetOrientation() != "landscape" {
		t.Errorf("Expected orientation 'landscape', got '%s'", orientationProc.GetOrientation())
	}
}
