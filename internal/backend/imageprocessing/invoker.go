package imageprocessing

import (
	"fmt"
	"log/slog"
	"time"
)

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