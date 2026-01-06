package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log/slog"
)

// CropParams represents typed parameters for crop processor
type CropParams struct {
	Height int
	Width  int
}

// NewCropParamsFromMap creates CropParams from a generic map
func NewCropParamsFromMap(params map[string]any) (*CropParams, error) {
	// Validate required parameters exist
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

	return &CropParams{
		Height: height,
		Width:  width,
	}, nil
}

// CropProcessor handles image cropping operations
type CropProcessor struct {
	name   string
	params *CropParams
}

// NewCropProcessor creates a new crop processor from configuration parameters
func NewCropProcessor(params map[string]any) (ImageProcessor, error) {
	typedParams, err := NewCropParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &CropProcessor{
		name:   "CropProcessor",
		params: typedParams,
	}, nil
}

// Type returns the processor type
func (p *CropProcessor) Type() string {
	return p.name
}

// ProcessImage crops the image to the configured dimensions
func (p *CropProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	slog.Debug("CropProcessor: decoding image",
		"input_size_bytes", len(imageData))
	
	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("CropProcessor: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()
	
	slog.Debug("CropProcessor: image decoded",
		"original_width", originalWidth,
		"original_height", originalHeight,
		"target_width", p.params.Width,
		"target_height", p.params.Height)

	// Calculate crop dimensions (center crop)
	cropWidth := p.params.Width
	cropHeight := p.params.Height

	// If requested dimensions are larger than original, return original
	if cropWidth >= originalWidth && cropHeight >= originalHeight {
		slog.Debug("CropProcessor: no crop needed, dimensions already smaller or equal")
		return imageData, nil
	}

	// Limit crop dimensions to original size
	if cropWidth > originalWidth {
		slog.Debug("CropProcessor: limiting crop width to original width",
			"requested", cropWidth,
			"limited_to", originalWidth)
		cropWidth = originalWidth
	}
	if cropHeight > originalHeight {
		slog.Debug("CropProcessor: limiting crop height to original height",
			"requested", cropHeight,
			"limited_to", originalHeight)
		cropHeight = originalHeight
	}

	// Calculate crop rectangle (center crop)
	x0 := (originalWidth - cropWidth) / 2
	y0 := (originalHeight - cropHeight) / 2
	
	slog.Debug("CropProcessor: performing center crop",
		"crop_x", x0,
		"crop_y", y0,
		"crop_width", cropWidth,
		"crop_height", cropHeight)

	// Create a new image with the cropped region
	croppedImg := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))
	for y := 0; y < cropHeight; y++ {
		for x := 0; x < cropWidth; x++ {
			croppedImg.Set(x, y, img.At(x0+x, y0+y))
		}
	}

	slog.Debug("CropProcessor: encoding cropped image")
	
	// Encode the cropped image back to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, croppedImg)
	if err != nil {
		slog.Error("CropProcessor: failed to encode cropped image", "error", err)
		return nil, fmt.Errorf("failed to encode cropped PNG image: %w", err)
	}
	
	slog.Debug("CropProcessor: crop complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// GetHeight returns the configured height
func (p *CropProcessor) GetHeight() int {
	return p.params.Height
}

// GetWidth returns the configured width
func (p *CropProcessor) GetWidth() int {
	return p.params.Width
}

// GetParams returns the typed parameters
func (p *CropProcessor) GetParams() *CropParams {
	return p.params
}

func init() {
	// Register the processor in the default registry
	if err := DefaultRegistry.Register("CropProcessor", NewCropProcessor); err != nil {
		panic(fmt.Sprintf("failed to register CropProcessor: %v", err))
	}
}
