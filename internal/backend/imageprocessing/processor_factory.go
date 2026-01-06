package imageprocessing

import (
	"fmt"
	"log/slog"
	"time"
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
	start := time.Now()
	
	slog.Info("starting image processing pipeline",
		"processor_count", len(processorConfigs),
		"input_size_bytes", len(imageData))
	
	if len(processorConfigs) == 0 {
		slog.Debug("no processors configured, returning original image")
		return imageData, nil
	}
	
	currentData := imageData
	
	for i, config := range processorConfigs {
		processorStart := time.Now()
		
		slog.Debug("creating processor",
			"index", i,
			"processor_name", config.Name,
			"params", config.Params)
		
		// Create the processor from the registry
		processor, err := DefaultRegistry.Create(config.Name, config.Params)
		if err != nil {
			slog.Error("failed to create processor",
				"index", i,
				"processor_name", config.Name,
				"error", err)
			return nil, fmt.Errorf("failed to create processor at index %d (%s): %w", i, config.Name, err)
		}
		
		slog.Info("applying processor",
			"index", i,
			"processor_name", config.Name,
			"input_size_bytes", len(currentData))
		
		// Apply the processor
		processedData, err := processor.ProcessImage(currentData)
		if err != nil {
			slog.Error("processor execution failed",
				"index", i,
				"processor_name", config.Name,
				"error", err,
				"input_size_bytes", len(currentData))
			return nil, fmt.Errorf("processor %s (index %d) failed: %w", config.Name, i, err)
		}
		
		processorDuration := time.Since(processorStart)
		slog.Info("processor completed",
			"index", i,
			"processor_name", config.Name,
			"duration_ms", processorDuration.Milliseconds(),
			"input_size_bytes", len(currentData),
			"output_size_bytes", len(processedData))
		
		currentData = processedData
	}
	
	totalDuration := time.Since(start)
	slog.Info("image processing pipeline completed",
		"total_duration_ms", totalDuration.Milliseconds(),
		"processor_count", len(processorConfigs),
		"final_size_bytes", len(currentData))
	
	return currentData, nil
}
