package imageprocessing

import (
	"fmt"
)

// ImageProcessor defines the interface for all image processors
type ImageProcessor interface {
	Type() string
	ProcessImage(imageData []byte) ([]byte, error)
}

// ProcessorFactory is a function type that creates a processor from configuration parameters
type ProcessorFactory func(params map[string]any) (ImageProcessor, error)

// ProcessorRegistry manages the registration and creation of image processors
type ProcessorRegistry struct {
	factories map[string]ProcessorFactory
}

// NewProcessorRegistry creates a new processor registry
func NewProcessorRegistry() *ProcessorRegistry {
	return &ProcessorRegistry{
		factories: make(map[string]ProcessorFactory),
	}
}

// Register adds a processor factory to the registry
func (r *ProcessorRegistry) Register(name string, factory ProcessorFactory) error {
	if name == "" {
		return fmt.Errorf("processor name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("processor factory cannot be nil")
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("processor %s is already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// Create instantiates a processor by name with the given parameters
func (r *ProcessorRegistry) Create(name string, params map[string]any) (ImageProcessor, error) {
	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("unknown processor: %s", name)
	}

	processor, err := factory(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create processor %s: %w", name, err)
	}

	return processor, nil
}

// IsRegistered checks if a processor with the given name is registered
func (r *ProcessorRegistry) IsRegistered(name string) bool {
	_, exists := r.factories[name]
	return exists
}

// GetRegisteredNames returns a list of all registered processor names
func (r *ProcessorRegistry) GetRegisteredNames() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is a global registry instance with common processors pre-registered
var DefaultRegistry = NewProcessorRegistry()

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

// ProcessorConfig represents a processor configuration with name and parameters
type ProcessorConfig struct {
	Name   string
	Params map[string]any
}

// ApplyProcessors applies a sequence of processors to an image in order
func ApplyProcessors(imageData []byte, processorConfigs []ProcessorConfig) ([]byte, error) {
	if len(processorConfigs) == 0 {
		return imageData, nil
	}

	currentData := imageData

	for i, config := range processorConfigs {
		// Create the processor from the registry
		processor, err := DefaultRegistry.Create(config.Name, config.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to create processor at index %d (%s): %w", i, config.Name, err)
		}

		// Apply the processor
		processedData, err := processor.ProcessImage(currentData)
		if err != nil {
			return nil, fmt.Errorf("processor %s (index %d) failed: %w", config.Name, i, err)
		}

		currentData = processedData
	}

	return currentData, nil
}
