package commands

import (
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"testing"
)

func TestNewPngConverterCommand_Success(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{
			name:   "No parameters needed",
			params: map[string]any{},
		},
		{
			name:   "With empty parameters",
			params: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := NewPngConverterCommand(tt.params)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			converterCmd, ok := command.(*PngConverterCommand)
			if !ok {
				t.Fatal("Expected command to be *PngConverterCommand")
			}

			if converterCmd.Name() != "PngConverterCommand" {
				t.Errorf("Expected name 'PngConverterCommand', got '%s'", converterCmd.Name())
			}
		})
	}
}

func TestPngConverterCommand_Name(t *testing.T) {
	command, err := NewPngConverterCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	if command.Name() != "PngConverterCommand" {
		t.Errorf("Expected name 'PngConverterCommand', got '%s'", command.Name())
	}
}

func TestPngConverterCommand_Execute_InvalidImage(t *testing.T) {
	command, err := NewPngConverterCommand(map[string]any{})
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

func TestPngConverterCommand_Execute_AlreadyPng(t *testing.T) {
	command, err := NewPngConverterCommand(map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command: %v", err)
	}

	// PNG signature: 0x89 'P' 'N' 'G' 0x0D 0x0A 0x1A 0x0A followed by some data
	pngSignature := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}

	result, err := command.Execute(pngSignature)
	if err != nil {
		t.Fatalf("Expected no error for valid PNG signature, got %v", err)
	}

	// Should return the same data without conversion
	if len(result) != len(pngSignature) {
		t.Errorf("Expected result length %d, got %d", len(pngSignature), len(result))
	}
}

func TestPngConverterCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !commandstructure.DefaultRegistry.IsRegistered("PngConverterCommand") {
		t.Error("Expected PngConverterCommand to be registered in DefaultRegistry")
	}

	// Test creating via registry
	command, err := commandstructure.DefaultRegistry.Create("PngConverterCommand", map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create command via registry: %v", err)
	}

	converterCmd, ok := command.(*PngConverterCommand)
	if !ok {
		t.Fatal("Expected command to be *PngConverterCommand")
	}

	if converterCmd.Name() != "PngConverterCommand" {
		t.Errorf("Expected name 'PngConverterCommand', got '%s'", converterCmd.Name())
	}
}

func TestHasCorrectPngSignature(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "Valid PNG signature",
			data:     []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00},
			expected: true,
		},
		{
			name:     "Invalid signature",
			data:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: false,
		},
		{
			name:     "Too short",
			data:     []byte{0x89, 'P', 'N', 'G'},
			expected: false,
		},
		{
			name:     "Empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "JPEG signature",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCorrectPngSignature(tt.data)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
