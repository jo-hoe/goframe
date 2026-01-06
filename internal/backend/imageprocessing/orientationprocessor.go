package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log/slog"
)

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

// ProcessImage rotates the image based on the configured orientation
func (p *OrientationProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	slog.Debug("OrientationProcessor: decoding image",
		"input_size_bytes", len(imageData),
		"target_orientation", p.params.Orientation)

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("OrientationProcessor: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Determine if rotation is needed
	isCurrentlyPortrait := height >= width
	needsPortrait := p.params.Orientation == "portrait"

	slog.Debug("OrientationProcessor: analyzing orientation",
		"width", width,
		"height", height,
		"currently_portrait", isCurrentlyPortrait,
		"needs_portrait", needsPortrait)

	// If already in correct orientation, return original
	if isCurrentlyPortrait == needsPortrait {
		slog.Debug("OrientationProcessor: already in correct orientation, no rotation needed")
		return imageData, nil
	}

	slog.Debug("OrientationProcessor: rotating image 90 degrees clockwise")

	// Rotate 90 degrees clockwise to switch between portrait and landscape
	rotatedImg := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Rotate 90 degrees clockwise: (x,y) -> (height-1-y, x)
			rotatedImg.Set(height-1-y, x, img.At(x, y))
		}
	}

	slog.Debug("OrientationProcessor: encoding rotated image")

	// Encode the rotated image back to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, rotatedImg)
	if err != nil {
		slog.Error("OrientationProcessor: failed to encode rotated image", "error", err)
		return nil, fmt.Errorf("failed to encode rotated PNG image: %w", err)
	}

	slog.Debug("OrientationProcessor: rotation complete",
		"output_size_bytes", buf.Len(),
		"new_width", height,
		"new_height", width)

	return buf.Bytes(), nil
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
