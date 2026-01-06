package imageprocessing

import (
	"testing"
)

func TestNewImageConverterProcessor_Success(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]any
		expected   string
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
			processor, err := NewImageConverterProcessor(tt.params)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			converterProc, ok := processor.(*ImageConverterProcessor)
			if !ok {
				t.Fatal("Expected processor to be *ImageConverterProcessor")
			}

			if converterProc.GetTargetType() != tt.expected {
				t.Errorf("Expected target type '%s', got '%s'", tt.expected, converterProc.GetTargetType())
			}
		})
	}
}

func TestNewImageConverterProcessor_InvalidTargetType(t *testing.T) {
	params := map[string]any{
		"targetType": "bmp",
	}

	_, err := NewImageConverterProcessor(params)
	if err == nil {
		t.Error("Expected error for invalid target type")
	}
}

func TestImageConverterProcessor_Type(t *testing.T) {
	processor, err := NewImageConverterProcessor(map[string]any{
		"targetType": "png",
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor.Type() != "ImageConverterProcessor" {
		t.Errorf("Expected type 'ImageConverterProcessor', got '%s'", processor.Type())
	}
}

func TestImageConverterProcessor_ProcessImage(t *testing.T) {
	processor, err := NewImageConverterProcessor(map[string]any{
		"targetType": "png",
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

func TestImageConverterProcessor_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("ImageConverterProcessor") {
		t.Error("Expected ImageConverterProcessor to be registered in DefaultRegistry")
	}

	// Test creating via registry
	processor, err := DefaultRegistry.Create("ImageConverterProcessor", map[string]any{
		"targetType": "jpeg",
	})
	if err != nil {
		t.Fatalf("Failed to create processor via registry: %v", err)
	}

	converterProc, ok := processor.(*ImageConverterProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ImageConverterProcessor")
	}

	if converterProc.GetTargetType() != "jpeg" {
		t.Errorf("Expected target type 'jpeg', got '%s'", converterProc.GetTargetType())
	}
}

func TestImageConverterProcessor_GetParams(t *testing.T) {
	processor, err := NewImageConverterProcessor(map[string]any{
		"targetType": "gif",
	})
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	converterProc, ok := processor.(*ImageConverterProcessor)
	if !ok {
		t.Fatal("Expected processor to be *ImageConverterProcessor")
	}

	params := converterProc.GetParams()
	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.TargetType != "gif" {
		t.Errorf("Expected target type 'gif', got '%s'", params.TargetType)
	}
}
