package imageprocessing

import (
	"testing"
)

func TestNewProcessorRegistry(t *testing.T) {
	registry := NewProcessorRegistry()
	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if registry.factories == nil {
		t.Fatal("Expected non-nil factories map")
	}
}

func TestProcessorRegistry_Register(t *testing.T) {
	registry := NewProcessorRegistry()

	// Test successful registration
	err := registry.Register("TestProcessor", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "TestProcessor", orientation: "portrait"}, nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test duplicate registration
	err = registry.Register("TestProcessor", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "TestProcessor", orientation: "portrait"}, nil
	})
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Test empty name
	err = registry.Register("", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "", orientation: "portrait"}, nil
	})
	if err == nil {
		t.Error("Expected error for empty name")
	}

	// Test nil factory
	err = registry.Register("NilFactory", nil)
	if err == nil {
		t.Error("Expected error for nil factory")
	}
}

func TestProcessorRegistry_Create(t *testing.T) {
	registry := NewProcessorRegistry()

	// Register a test processor
	err := registry.Register("TestProcessor", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "TestProcessor", orientation: "portrait"}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}

	// Test creating registered processor
	processor, err := registry.Create("TestProcessor", nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if processor == nil {
		t.Fatal("Expected non-nil processor")
	}
	if processor.Type() != "TestProcessor" {
		t.Errorf("Expected processor type 'TestProcessor', got '%s'", processor.Type())
	}

	// Test creating unregistered processor
	_, err = registry.Create("UnknownProcessor", nil)
	if err == nil {
		t.Error("Expected error for unknown processor")
	}
}

func TestProcessorRegistry_IsRegistered(t *testing.T) {
	registry := NewProcessorRegistry()

	// Register a test processor
	err := registry.Register("TestProcessor", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "TestProcessor", orientation: "portrait"}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}

	// Test registered processor
	if !registry.IsRegistered("TestProcessor") {
		t.Error("Expected TestProcessor to be registered")
	}

	// Test unregistered processor
	if registry.IsRegistered("UnknownProcessor") {
		t.Error("Expected UnknownProcessor to not be registered")
	}
}

func TestProcessorRegistry_GetRegisteredNames(t *testing.T) {
	registry := NewProcessorRegistry()

	// Test empty registry
	names := registry.GetRegisteredNames()
	if len(names) != 0 {
		t.Errorf("Expected 0 registered names, got %d", len(names))
	}

	// Register processors
	err := registry.Register("Processor1", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "Processor1", orientation: "portrait"}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register Processor1: %v", err)
	}
	err = registry.Register("Processor2", func(params map[string]any) (ImageProcessor, error) {
		return &OrientationProcessor{name: "Processor2", orientation: "portrait"}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register Processor2: %v", err)
	}

	names = registry.GetRegisteredNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 registered names, got %d", len(names))
	}

	// Verify names are present
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}
	if !nameMap["Processor1"] || !nameMap["Processor2"] {
		t.Error("Expected both Processor1 and Processor2 to be in registered names")
	}
}

func TestDefaultRegistry_HasProcessors(t *testing.T) {
	// Verify default registry has OrientationProcessor and CropProcessor registered
	if !DefaultRegistry.IsRegistered("OrientationProcessor") {
		t.Error("Expected OrientationProcessor to be registered in DefaultRegistry")
	}
	if !DefaultRegistry.IsRegistered("CropProcessor") {
		t.Error("Expected CropProcessor to be registered in DefaultRegistry")
	}
}

func TestGetStringParam(t *testing.T) {
	params := map[string]any{
		"key1": "value1",
		"key2": 123,
	}

	// Test existing string parameter
	if val := getStringParam(params, "key1", "default"); val != "value1" {
		t.Errorf("Expected 'value1', got '%s'", val)
	}

	// Test non-string parameter
	if val := getStringParam(params, "key2", "default"); val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}

	// Test non-existent parameter
	if val := getStringParam(params, "key3", "default"); val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}
}

func TestGetIntParam(t *testing.T) {
	params := map[string]any{
		"key1": 123,
		"key2": int64(456),
		"key3": float64(789),
		"key4": "not-an-int",
	}

	// Test int parameter
	if val := getIntParam(params, "key1", 0); val != 123 {
		t.Errorf("Expected 123, got %d", val)
	}

	// Test int64 parameter
	if val := getIntParam(params, "key2", 0); val != 456 {
		t.Errorf("Expected 456, got %d", val)
	}

	// Test float64 parameter
	if val := getIntParam(params, "key3", 0); val != 789 {
		t.Errorf("Expected 789, got %d", val)
	}

	// Test non-int parameter
	if val := getIntParam(params, "key4", 999); val != 999 {
		t.Errorf("Expected 999, got %d", val)
	}

	// Test non-existent parameter
	if val := getIntParam(params, "key5", 999); val != 999 {
		t.Errorf("Expected 999, got %d", val)
	}
}

func TestValidateRequiredParams(t *testing.T) {
	params := map[string]any{
		"param1": "value1",
		"param2": 123,
	}

	// Test all required params present
	err := validateRequiredParams(params, []string{"param1", "param2"})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test missing required param
	err = validateRequiredParams(params, []string{"param1", "param3"})
	if err == nil {
		t.Error("Expected error for missing required param")
	}

	// Test no required params
	err = validateRequiredParams(params, []string{})
	if err != nil {
		t.Errorf("Expected no error for empty required list, got %v", err)
	}
}
