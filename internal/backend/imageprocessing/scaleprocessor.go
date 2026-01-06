package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
)

// ScaleParams represents typed parameters for scale processor
type ScaleParams struct {
	Height int
	Width  int
}

// NewScaleParamsFromMap creates ScaleParams from a generic map
func NewScaleParamsFromMap(params map[string]any) (*ScaleParams, error) {
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

	return &ScaleParams{
		Height: height,
		Width:  width,
	}, nil
}

// ScaleProcessor handles image scaling with aspect ratio preservation
type ScaleProcessor struct {
	name   string
	params *ScaleParams
}

// NewScaleProcessor creates a new scale processor from configuration parameters
func NewScaleProcessor(params map[string]any) (ImageProcessor, error) {
	typedParams, err := NewScaleParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &ScaleProcessor{
		name:   "ScaleProcessor",
		params: typedParams,
	}, nil
}

// Type returns the processor type
func (p *ScaleProcessor) Type() string {
	return p.name
}

// ProcessImage scales the image to target dimensions while preserving aspect ratio
func (p *ScaleProcessor) ProcessImage(imageData []byte) ([]byte, error) {
	slog.Debug("ScaleProcessor: decoding image",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("ScaleProcessor: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	targetWidth := p.params.Width
	targetHeight := p.params.Height

	// Calculate aspect ratios
	originalAspect := float64(originalWidth) / float64(originalHeight)
	targetAspect := float64(targetWidth) / float64(targetHeight)

	slog.Debug("ScaleProcessor: calculating scaled dimensions",
		"original_width", originalWidth,
		"original_height", originalHeight,
		"original_aspect_ratio", originalAspect,
		"target_width", targetWidth,
		"target_height", targetHeight,
		"target_aspect_ratio", targetAspect)

	// Calculate scaled dimensions that fit within target while preserving aspect ratio
	var scaledWidth, scaledHeight int
	if originalAspect > targetAspect {
		// Original is wider - scale to target width
		scaledWidth = targetWidth
		scaledHeight = int(float64(targetWidth) / originalAspect)
		slog.Debug("ScaleProcessor: original is wider, scaling to target width")
	} else {
		// Original is taller - scale to target height
		scaledHeight = targetHeight
		scaledWidth = int(float64(targetHeight) * originalAspect)
		slog.Debug("ScaleProcessor: original is taller, scaling to target height")
	}

	slog.Debug("ScaleProcessor: scaled dimensions calculated",
		"scaled_width", scaledWidth,
		"scaled_height", scaledHeight)

	// Create target image with white background
	targetImg := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(targetImg, targetImg.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Calculate position to center the scaled image
	offsetX := (targetWidth - scaledWidth) / 2
	offsetY := (targetHeight - scaledHeight) / 2

	slog.Debug("ScaleProcessor: centering image on canvas",
		"offset_x", offsetX,
		"offset_y", offsetY)

	// Scale and draw the image
	// Simple nearest-neighbor scaling
	for y := 0; y < scaledHeight; y++ {
		for x := 0; x < scaledWidth; x++ {
			// Map scaled coordinates back to original image coordinates
			srcX := int(float64(x) * float64(originalWidth) / float64(scaledWidth))
			srcY := int(float64(y) * float64(originalHeight) / float64(scaledHeight))

			// Ensure we don't go out of bounds
			if srcX >= originalWidth {
				srcX = originalWidth - 1
			}
			if srcY >= originalHeight {
				srcY = originalHeight - 1
			}

			targetImg.Set(offsetX+x, offsetY+y, img.At(srcX, srcY))
		}
	}

	slog.Debug("ScaleProcessor: encoding scaled image")

	// Encode the scaled image to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, targetImg)
	if err != nil {
		slog.Error("ScaleProcessor: failed to encode scaled image", "error", err)
		return nil, fmt.Errorf("failed to encode scaled PNG image: %w", err)
	}

	slog.Debug("ScaleProcessor: scaling complete",
		"output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// GetHeight returns the configured height
func (p *ScaleProcessor) GetHeight() int {
	return p.params.Height
}

// GetWidth returns the configured width
func (p *ScaleProcessor) GetWidth() int {
	return p.params.Width
}

// GetParams returns the typed parameters
func (p *ScaleProcessor) GetParams() *ScaleParams {
	return p.params
}

func init() {
	// Register the processor in the default registry
	if err := DefaultRegistry.Register("ScaleProcessor", NewScaleProcessor); err != nil {
		panic(fmt.Sprintf("failed to register ScaleProcessor: %v", err))
	}
}
