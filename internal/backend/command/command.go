package command

// Command defines the interface for all image processing commands
type Command interface {
	Name() string
	Execute(imageData []byte) ([]byte, error)
}

// CommandFactory is a function type that creates a command from configuration parameters
type CommandFactory func(params map[string]any) (Command, error)

// CommandConfig represents a command configuration with name and parameters
type CommandConfig struct {
	Name   string
	Params map[string]any
}
