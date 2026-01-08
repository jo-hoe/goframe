package commandstructure

import (
	"testing"
	"internal/backend/commands"
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
		return &commands.OrientationCommand{
			name:   "TestCommand",
			params: &commands.OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test duplicate registration
	err = registry.Register("TestCommand", func(params map[string]any) (Command, error) {
		return &commands.OrientationCommand{
			name:   "TestCommand",
			params: &commands.OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Test empty name
	err = registry.Register("", func(params map[string]any) (Command, error) {
		return &commands.OrientationCommand{
			name:   "",
			params: &commands.OrientationParams{Orientation: "portrait"},
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
		return &commands.OrientationCommand{
			name:   "TestCommand",
			params: &commands.OrientationParams{Orientation: "portrait"},
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
		return &commands.OrientationCommand{
			name:   "TestCommand",
			params: &commands.OrientationParams{Orientation: "portrait"},
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
		return &commands.OrientationCommand{
			name:   "Command1",
			params: &commands.OrientationParams{Orientation: "portrait"},
		}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register Command1: %v", err)
	}
	err = registry.Register("Command2", func(params map[string]any) (Command, error) {
		return &commands.OrientationCommand{
			name:   "Command2",
			params: &commands.OrientationParams{Orientation: "portrait"},
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
