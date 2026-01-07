package command

import (
	"fmt"
)

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
