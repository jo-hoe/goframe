package imageprocessing

import "fmt"

// CropProcessor handles image cropping operations
type CropProcessor struct {
	name   string
	height int
	width  int
}

// NewCropProcessor creates a new crop processor from configuration parameters
func NewCropProcessor(params map[string]interface{}) (ImageProcessor, error) {
	// Height and width are required parameters
	if err := validateRequiredParams(params, []string{"height", "width"}); err != nil {
		return nil, err
	}

	height := getIntParam(params, "height", 0)
	width := getIntParam(params, "width", 0)

	// Validate dimensions are positive
	if height <= 0 {
		return nil, fmt.Errorf("height must be positive, got %d", height)
	}
	if width <= 0 {
		return nil, fmt.Errorf("width must be positive, got %d", width)
	}

	return &CropProcessor{
		name:   "CropProcessor",
		height: height,
		width:  width,
	}, nil
}

// Type returns the processor type
func (p *CropProcessor) Type() string {
	return p.name
}

// ProcessImage processes the image (placeholder implementation)
func (p *CropProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	// Placeholder: In a real scenario, this would crop the image
	// to p.width x p.height dimensions
	return imageData, nil
}

// GetHeight returns the configured height
func (p *CropProcessor) GetHeight() int {
	return p.height
}

// GetWidth returns the configured width
func (p *CropProcessor) GetWidth() int {
	return p.width
}

func init() {
	// Register the processor in the default registry
	if err := DefaultRegistry.Register("CropProcessor", NewCropProcessor); err != nil {
		panic(fmt.Sprintf("failed to register CropProcessor: %v", err))
	}
}
