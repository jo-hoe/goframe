package imageprocessing

import "fmt"

// OrientationParams represents typed parameters for orientation processor
type OrientationParams struct {
	Orientation string
}

// NewOrientationParamsFromMap creates OrientationParams from a generic map
func NewOrientationParamsFromMap(params map[string]any) (*OrientationParams, error) {
	orientation := getStringParam(params, "orientation", "portrait")

	// Validate orientation value
	validOrientations := map[string]bool{
		"portrait":  true,
		"landscape": true,
	}

	if !validOrientations[orientation] {
		return nil, fmt.Errorf("invalid orientation: %s (must be 'portrait' or 'landscape')", orientation)
	}

	return &OrientationParams{
		Orientation: orientation,
	}, nil
}

// OrientationProcessor handles image orientation adjustments
type OrientationProcessor struct {
	name   string
	params *OrientationParams
}

// NewOrientationProcessor creates a new orientation processor from configuration parameters
func NewOrientationProcessor(params map[string]any) (ImageProcessor, error) {
	typedParams, err := NewOrientationParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &OrientationProcessor{
		name:   "OrientationProcessor",
		params: typedParams,
	}, nil
}

// Type returns the processor type
func (p *OrientationProcessor) Type() string {
	return p.name
}

// ProcessImage processes the image (placeholder implementation)
func (p *OrientationProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	// Placeholder: In a real scenario, this would adjust the image orientation
	// based on p.orientation
	return imageData, nil
}

// GetOrientation returns the configured orientation
func (p *OrientationProcessor) GetOrientation() string {
	return p.params.Orientation
}

// GetParams returns the typed parameters
func (p *OrientationProcessor) GetParams() *OrientationParams {
	return p.params
}

func init() {
	// Register the processor in the default registry
	if err := DefaultRegistry.Register("OrientationProcessor", NewOrientationProcessor); err != nil {
		panic(fmt.Sprintf("failed to register OrientationProcessor: %v", err))
	}
}
