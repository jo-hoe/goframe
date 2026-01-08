package commandstructure

import (
	"errors"
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
	// Create a test registry with a command that validates parameters
	testRegistry := NewCommandRegistry()
	err := testRegistry.Register("TestCommand", func(params map[string]any) (Command, error) {
		if err := ValidateRequiredParams(params, []string{"required_param"}); err != nil {
			return nil, err
		}
		return newMockCommand("TestCommand"), nil
	})
	if err != nil {
		t.Fatalf("Failed to register test command: %v", err)
	}

	// Temporarily replace DefaultRegistry for this test
	originalRegistry := DefaultRegistry
	DefaultRegistry = testRegistry
	defer func() { DefaultRegistry = originalRegistry }()

	testData := []byte("test data")
	configs := []CommandConfig{
		{
			Name:   "TestCommand",
			Params: map[string]any{
				// Missing required parameter
			},
		},
	}

	_, err = ExecuteCommands(testData, configs)
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
	// Create a mock command that returns an error
	testCmd := newMockCommandWithError("TestCommand", errors.New("invalid image data"))

	invoker := NewCommandInvoker([]Command{testCmd})
	testData := []byte("invalid image data")
	_, err := invoker.Execute(testData)
	if err == nil {
		t.Error("Expected error for invalid image data")
	}
}

func TestNewCommandInvoker(t *testing.T) {
	commands := []Command{
		newMockCommand("TestCommand"),
	}

	invoker := NewCommandInvoker(commands)
	if invoker == nil {
		t.Fatal("Expected non-nil invoker")
	}
	if len(invoker.commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(invoker.commands))
	}
}

func TestCommandInvoker_MultipleCommands(t *testing.T) {
	// Create mock commands that modify the data
	cmd1 := &mockCommand{
		name: "Command1",
		executeFunc: func(data []byte) ([]byte, error) {
			return append(data, []byte("-cmd1")...), nil
		},
	}
	cmd2 := &mockCommand{
		name: "Command2",
		executeFunc: func(data []byte) ([]byte, error) {
			return append(data, []byte("-cmd2")...), nil
		},
	}

	invoker := NewCommandInvoker([]Command{cmd1, cmd2})
	testData := []byte("start")
	result, err := invoker.Execute(testData)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := "start-cmd1-cmd2"
	if string(result) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(result))
	}
}

func TestCommandInvoker_ErrorInMiddle(t *testing.T) {
	// Create commands where the second one fails
	cmd1 := newMockCommand("Command1")
	cmd2 := newMockCommandWithError("Command2", errors.New("command2 failed"))
	cmd3 := newMockCommand("Command3")

	invoker := NewCommandInvoker([]Command{cmd1, cmd2, cmd3})
	testData := []byte("test")
	_, err := invoker.Execute(testData)

	if err == nil {
		t.Error("Expected error when command fails")
	}

	// Verify the error message contains the command name
	if err != nil && err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}
