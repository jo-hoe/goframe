package config

// CommandConfig maps a command name to its parameters.
// Parameters are declared inline in YAML alongside the name field.
//
// Example:
//
//	commands:
//	  - name: ScaleCommand
//	    height: 1920
//	    width: 1080
type CommandConfig struct {
	Name   string         `yaml:"name"`
	Params map[string]any `yaml:",inline"`
}
