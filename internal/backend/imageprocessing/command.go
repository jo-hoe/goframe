package imageprocessing

import (
	"fmt"
	"log/slog"
	"time"
)

// Command defines the interface for all image processing commands
type Command interface {
	Name() string
	Execute(imageData []byte) ([]byte, error)
}

// CommandFactory is a function type that creates a command from configuration parameters
type CommandFactory func(params map[string]any) (Command, error)

// CommandRegistry manages the registration and creation of image processing commands
type CommandRegistry struct {
	factories map[string]CommandFactory
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		factories: make(map[string]CommandFactory),
	}
}

// Register adds a command factory to the registry
func (r *CommandRegistry) Register(name string, factory CommandFactory) error {
	if name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("command factory cannot be nil")
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("command %s is already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// Create instantiates a command by name with the given parameters
func (r *CommandRegistry) Create(name string, params map[string]any) (Command, error) {
	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("unknown command: %s", name)
	}

	command, err := factory(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create command %s: %w", name, err)
	}

	return command, nil
}

// IsRegistered checks if a command with the given name is registered
func (r *CommandRegistry) IsRegistered(name string) bool {
	_, exists := r.factories[name]
	return exists
}

// GetRegisteredNames returns a list of all registered command names
func (r *CommandRegistry) GetRegisteredNames() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is a global registry instance with common commands pre-registered
var DefaultRegistry = NewCommandRegistry()

// getStringParam safely extracts a string parameter from the params map
func getStringParam(params map[string]any, key string, defaultValue string) string {
	if val, ok := params[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// getIntParam safely extracts an int parameter from the params map
func getIntParam(params map[string]any, key string, defaultValue int) int {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// validateRequiredParams checks that all required parameters are present
func validateRequiredParams(params map[string]any, required []string) error {
	for _, key := range required {
		if _, ok := params[key]; !ok {
			return fmt.Errorf("missing required parameter: %s", key)
		}
	}
	return nil
}

// CommandConfig represents a command configuration with name and parameters
type CommandConfig struct {
	Name   string
	Params map[string]any
}

// CommandInvoker executes a sequence of commands on image data
type CommandInvoker struct {
	commands []Command
}

// NewCommandInvoker creates a new command invoker
func NewCommandInvoker(commands []Command) *CommandInvoker {
	return &CommandInvoker{
		commands: commands,
	}
}

// Execute applies all commands in sequence to the image data
func (i *CommandInvoker) Execute(imageData []byte) ([]byte, error) {
	start := time.Now()

	slog.Info("starting image processing pipeline",
		"command_count", len(i.commands),
		"input_size_bytes", len(imageData))

	if len(i.commands) == 0 {
		slog.Debug("no commands to execute, returning original image")
		return imageData, nil
	}

	currentData := imageData

	for idx, command := range i.commands {
		commandStart := time.Now()

		slog.Info("executing command",
			"index", idx,
			"command_name", command.Name(),
			"input_size_bytes", len(currentData))

		// Execute the command
		processedData, err := command.Execute(currentData)
		if err != nil {
			slog.Error("command execution failed",
				"index", idx,
				"command_name", command.Name(),
				"error", err,
				"input_size_bytes", len(currentData))
			return nil, fmt.Errorf("command %s (index %d) failed: %w", command.Name(), idx, err)
		}

		commandDuration := time.Since(commandStart)
		slog.Info("command completed",
			"index", idx,
			"command_name", command.Name(),
			"duration_ms", commandDuration.Milliseconds(),
			"input_size_bytes", len(currentData),
			"output_size_bytes", len(processedData))

		currentData = processedData
	}

	totalDuration := time.Since(start)
	slog.Info("image processing pipeline completed",
		"total_duration_ms", totalDuration.Milliseconds(),
		"command_count", len(i.commands),
		"final_size_bytes", len(currentData))

	return currentData, nil
}

// ExecuteCommands applies a sequence of commands to an image in order
func ExecuteCommands(imageData []byte, commandConfigs []CommandConfig) ([]byte, error) {
	start := time.Now()

	slog.Info("starting image processing pipeline",
		"command_count", len(commandConfigs),
		"input_size_bytes", len(imageData))

	if len(commandConfigs) == 0 {
		slog.Debug("no commands configured, returning original image")
		return imageData, nil
	}

	currentData := imageData

	for i, config := range commandConfigs {
		commandStart := time.Now()

		slog.Debug("creating command",
			"index", i,
			"command_name", config.Name,
			"params", config.Params)

		// Create the command from the registry
		command, err := DefaultRegistry.Create(config.Name, config.Params)
		if err != nil {
			slog.Error("failed to create command",
				"index", i,
				"command_name", config.Name,
				"error", err)
			return nil, fmt.Errorf("failed to create command at index %d (%s): %w", i, config.Name, err)
		}

		slog.Info("executing command",
			"index", i,
			"command_name", config.Name,
			"input_size_bytes", len(currentData))

		// Execute the command
		processedData, err := command.Execute(currentData)
		if err != nil {
			slog.Error("command execution failed",
				"index", i,
				"command_name", config.Name,
				"error", err,
				"input_size_bytes", len(currentData))
			return nil, fmt.Errorf("command %s (index %d) failed: %w", config.Name, i, err)
		}

		commandDuration := time.Since(commandStart)
		slog.Info("command completed",
			"index", i,
			"command_name", config.Name,
			"duration_ms", commandDuration.Milliseconds(),
			"input_size_bytes", len(currentData),
			"output_size_bytes", len(processedData))

		currentData = processedData
	}

	totalDuration := time.Since(start)
	slog.Info("image processing pipeline completed",
		"total_duration_ms", totalDuration.Milliseconds(),
		"command_count", len(commandConfigs),
		"final_size_bytes", len(currentData))

	return currentData, nil
}
