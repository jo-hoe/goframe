package imageprocessing

import (
	"testing"
)

func TestNewCommandRegistry(t *testing.T) {
	registry := NewCommandRegistry()
	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if registry.factories == nil {
		t.Fatal("Expected non-nil factories map")
	}
}

func TestCommandRegistry_Register(t *testing.T) {
	registry := NewCommandRegistry()

	// Test successful registration
	err := registry.Register("TestCommand", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "TestCommand",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test duplicate registration
	err = registry.Register("TestCommand", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "TestCommand",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Test empty name
	err = registry.Register("", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
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

func TestCommandRegistry_Create(t *testing.T) {
	registry := NewCommandRegistry()

	// Register a test command
	err := registry.Register("TestCommand", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "TestCommand",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register command: %v", err)
	}

	// Test creating registered command
	command, err := registry.Create("TestCommand", nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if command == nil {
		t.Fatal("Expected non-nil command")
	}
	if command.Name() != "TestCommand" {
		t.Errorf("Expected command name 'TestCommand', got '%s'", command.Name())
	}

	// Test creating unregistered command
	_, err = registry.Create("UnknownCommand", nil)
	if err == nil {
		t.Error("Expected error for unknown command")
	}
}

func TestCommandRegistry_IsRegistered(t *testing.T) {
	registry := NewCommandRegistry()

	// Register a test command
	err := registry.Register("TestCommand", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "TestCommand",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register command: %v", err)
	}

	// Test registered command
	if !registry.IsRegistered("TestCommand") {
		t.Error("Expected TestCommand to be registered")
	}

	// Test unregistered command
	if registry.IsRegistered("UnknownCommand") {
		t.Error("Expected UnknownCommand to not be registered")
	}
}

func TestCommandRegistry_GetRegisteredNames(t *testing.T) {
	registry := NewCommandRegistry()

	// Test empty registry
	names := registry.GetRegisteredNames()
	if len(names) != 0 {
		t.Errorf("Expected 0 registered names, got %d", len(names))
	}

	// Register commands
	err := registry.Register("Command1", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "Command1",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register Command1: %v", err)
	}
	err = registry.Register("Command2", func(params map[string]any) (Command, error) {
		return &OrientationCommand{
			name:   "Command2",
			params: &OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register Command2: %v", err)
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
	if !nameMap["Command1"] || !nameMap["Command2"] {
		t.Error("Expected both Command1 and Command2 to be in registered names")
	}
}

func TestDefaultRegistry_HasCommands(t *testing.T) {
	// Verify default registry has all commands registered
	expectedCommands := []string{
		"OrientationCommand",
		"CropCommand",
		"ScaleCommand",
		"ImageConverterCommand",
	}

	for _, cmdName := range expectedCommands {
		if !DefaultRegistry.IsRegistered(cmdName) {
			t.Errorf("Expected %s to be registered in DefaultRegistry", cmdName)
		}
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

func TestExecuteCommands_EmptyList(t *testing.T) {
	testData := []byte("test data")
	result, err := ExecuteCommands(testData, []CommandConfig{})

	if err != nil {
		t.Errorf("Expected no error for empty command list, got %v", err)
	}

	if string(result) != string(testData) {
		t.Error("Expected result to match input for empty command list")
	}
}

func TestExecuteCommands_UnknownCommand(t *testing.T) {
	testData := []byte("test data")
	configs := []CommandConfig{
		{
			Name:   "UnknownCommand",
			Params: map[string]any{},
		},
	}

	_, err := ExecuteCommands(testData, configs)
	if err == nil {
		t.Error("Expected error for unknown command")
	}
}

func TestExecuteCommands_InvalidCommandConfig(t *testing.T) {
	testData := []byte("test data")
	configs := []CommandConfig{
		{
			Name: "CropCommand",
			Params: map[string]any{
				// Missing required height and width
			},
		},
	}

	_, err := ExecuteCommands(testData, configs)
	if err == nil {
		t.Error("Expected error for invalid command configuration")
	}
}

func TestCommandInvoker(t *testing.T) {
	// Create a simple test command
	testCmd := &OrientationCommand{
		name:   "TestCommand",
		params: &OrientationParams{Orientation: "portrait"},
	}

	// Test with empty command list
	t.Run("Empty command list", func(t *testing.T) {
		invoker := NewCommandInvoker([]Command{})
		testData := []byte("test data")
		result, err := invoker.Execute(testData)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if string(result) != string(testData) {
			t.Error("Expected result to match input for empty command list")
		}
	})

	// Test with invalid image data - should return error
	t.Run("Invalid image data", func(t *testing.T) {
		invoker := NewCommandInvoker([]Command{testCmd})
		testData := []byte("invalid image data")
		_, err := invoker.Execute(testData)
		if err == nil {
			t.Error("Expected error for invalid image data")
		}
	})
}
