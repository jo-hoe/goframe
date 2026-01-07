package imageprocessing

import (
	"testing"
)

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
			Name:   "CropCommand",
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

func TestCommandInvoker_EmptyCommandList(t *testing.T) {
	invoker := NewCommandInvoker([]Command{})
	testData := []byte("test data")
	result, err := invoker.Execute(testData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if string(result) != string(testData) {
		t.Error("Expected result to match input for empty command list")
	}
}

func TestCommandInvoker_InvalidImageData(t *testing.T) {
	// Create a simple test command
	testCmd := &OrientationCommand{
		name:   "TestCommand",
		params: &OrientationParams{Orientation: "portrait"},
	}

	invoker := NewCommandInvoker([]Command{testCmd})
	testData := []byte("invalid image data")
	_, err := invoker.Execute(testData)
	if err == nil {
		t.Error("Expected error for invalid image data")
	}
}

func TestNewCommandInvoker(t *testing.T) {
	commands := []Command{
		&OrientationCommand{
			name:   "TestCommand",
			params: &OrientationParams{Orientation: "portrait"},
		},
	}

	invoker := NewCommandInvoker(commands)
	if invoker == nil {
		t.Fatal("Expected non-nil invoker")
	}
	if len(invoker.commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(invoker.commands))
	}
}
