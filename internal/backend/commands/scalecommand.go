package commands

import (
	"bytes"
	"fmt"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
)

// ScaleParams represents typed parameters for scale command
type ScaleParams struct {
	Height       int
	Width        int
	EdgeGradient bool
}

// NewScaleParamsFromMap creates ScaleParams from a generic map
func NewScaleParamsFromMap(params map[string]any) (*ScaleParams, error) {
	// Validate required parameters exist
	if err := commandstructure.ValidateRequiredParams(params, []string{"height", "width"}); err != nil {
		return nil, err
	}

	height := commandstructure.GetIntParam(params, "height", 0)
	width := commandstructure.GetIntParam(params, "width", 0)
	edgeGradient := commandstructure.GetBoolParam(params, "edgeGradient", false)

	// Validate dimensions are positive
	if height <= 0 {
		return nil, fmt.Errorf("height must be positive, got %d", height)
	}
	if width <= 0 {
		return nil, fmt.Errorf("width must be positive, got %d", width)
	}

	return &ScaleParams{
		Height:       height,
		Width:        width,
		EdgeGradient: edgeGradient,
	}, nil
}

// ScaleCommand handles image scaling with aspect ratio preservation
type ScaleCommand struct {
	name   string
	params *ScaleParams
}

// NewScaleCommand creates a new scale command from configuration parameters
func NewScaleCommand(params map[string]any) (commandstructure.Command, error) {
	typedParams, err := NewScaleParamsFromMap(params)
	if err != nil {
		return nil, err
	}

	return &ScaleCommand{
		name:   "ScaleCommand",
		params: typedParams,
	}, nil
}

// NewScaleCommandWithParams creates a new scale command from concrete typed parameters
func NewScaleCommandWithParams(height, width int) (*ScaleCommand, error) {
	if height <= 0 {
		return nil, fmt.Errorf("height must be positive, got %d", height)
	}
	if width <= 0 {
		return nil, fmt.Errorf("width must be positive, got %d", width)
	}

	return &ScaleCommand{
		name: "ScaleCommand",
		params: &ScaleParams{
			Height:       height,
			Width:        width,
			EdgeGradient: false,
		},
	}, nil
}

// Name returns the command name
func (c *ScaleCommand) Name() string {
	return c.name
}

// Execute scales the image to target dimensions while preserving aspect ratio
func (c *ScaleCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("ScaleCommand: decoding image",
		"input_size_bytes", len(imageData))

	// Decode the PNG image
	img, err := decodePNG(imageData)
	if err != nil {
		slog.Error("ScaleCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	targetWidth := c.params.Width
	targetHeight := c.params.Height

	// If target matches original dimensions, skip processing
	if targetWidth == originalWidth && targetHeight == originalHeight {
		slog.Debug("ScaleCommand: target dimensions equal original; skipping scaling")
		return imageData, nil
	}

	// Calculate aspect ratios for debugging
	originalAspect := float64(originalWidth) / float64(originalHeight)
	targetAspect := float64(targetWidth) / float64(targetHeight)
	slog.Debug("ScaleCommand: calculating scaled dimensions",
		"original_width", originalWidth,
		"original_height", originalHeight,
		"original_aspect_ratio", originalAspect,
		"target_width", targetWidth,
		"target_height", targetHeight,
		"target_aspect_ratio", targetAspect)

	// Compute scaled dimensions with aspect ratio preserved
	scaledWidth, scaledHeight := computeScaledDimensions(originalWidth, originalHeight, targetWidth, targetHeight)
	slog.Debug("ScaleCommand: scaled dimensions calculated",
		"scaled_width", scaledWidth,
		"scaled_height", scaledHeight)

	// Create target canvas and center placement
	targetImg := createTargetCanvas(targetWidth, targetHeight, color.RGBA{255, 255, 255, 255})
	offsetX, offsetY := computeCenterOffset(targetWidth, targetHeight, scaledWidth, scaledHeight)
	slog.Debug("ScaleCommand: centering image on canvas",
		"offset_x", offsetX,
		"offset_y", offsetY)

	// Build index maps and draw scaled image
	xMap, yMap := buildIndexMaps(originalWidth, originalHeight, scaledWidth, scaledHeight)
	drawScaledNearest(targetImg, img, offsetX, offsetY, scaledWidth, scaledHeight, xMap, yMap)

	// Optional: Fill padding areas with gradient from image edge colors to black/white border
	if c.params.EdgeGradient && (offsetX > 0 || offsetY > 0) {
		fillEdgeGradientPadding(targetImg, offsetX, offsetY, scaledWidth, scaledHeight)
	}

	slog.Debug("ScaleCommand: encoding scaled image")

	// Encode the scaled image to PNG bytes
	out, err := encodePNG(targetImg)
	if err != nil {
		slog.Error("ScaleCommand: failed to encode scaled image", "error", err)
		return nil, fmt.Errorf("failed to encode scaled PNG image: %w", err)
	}

	slog.Debug("ScaleCommand: scaling complete",
		"output_size_bytes", len(out))

	return out, nil
}

// GetHeight returns the configured height
func (c *ScaleCommand) GetHeight() int {
	return c.params.Height
}

// GetWidth returns the configured width
func (c *ScaleCommand) GetWidth() int {
	return c.params.Width
}

// GetParams returns the typed parameters
func (c *ScaleCommand) GetParams() *ScaleParams {
	return c.params
}

// Helper functions extracted for maintainability
func decodePNG(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

func computeScaledDimensions(originalWidth, originalHeight, targetWidth, targetHeight int) (int, int) {
	originalAspect := float64(originalWidth) / float64(originalHeight)
	targetAspect := float64(targetWidth) / float64(targetHeight)
	if originalAspect > targetAspect {
		// Original is wider - scale to target width
		scaledWidth := targetWidth
		scaledHeight := int(float64(targetWidth) / originalAspect)
		return scaledWidth, scaledHeight
	}
	// Original is taller - scale to target height
	scaledHeight := targetHeight
	scaledWidth := int(float64(targetHeight) * originalAspect)
	return scaledWidth, scaledHeight
}

func createTargetCanvas(w, h int, bg color.Color) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	return dst
}

func computeCenterOffset(targetWidth, targetHeight, scaledWidth, scaledHeight int) (int, int) {
	return (targetWidth - scaledWidth) / 2, (targetHeight - scaledHeight) / 2
}

func buildIndexMaps(originalWidth, originalHeight, scaledWidth, scaledHeight int) ([]int, []int) {
	xMap := make([]int, scaledWidth)
	yMap := make([]int, scaledHeight)
	for x := 0; x < scaledWidth; x++ {
		xMap[x] = int(float64(x) * float64(originalWidth) / float64(scaledWidth))
		if xMap[x] >= originalWidth {
			xMap[x] = originalWidth - 1
		}
	}
	for y := 0; y < scaledHeight; y++ {
		yMap[y] = int(float64(y) * float64(originalHeight) / float64(scaledHeight))
		if yMap[y] >= originalHeight {
			yMap[y] = originalHeight - 1
		}
	}
	return xMap, yMap
}

func drawScaledNearest(dst *image.RGBA, src image.Image, offsetX, offsetY, scaledWidth, scaledHeight int, xMap, yMap []int) {
	parallelFor(scaledHeight, func(y int) {
		for x := 0; x < scaledWidth; x++ {
			srcX := xMap[x]
			srcY := yMap[y]
			dst.Set(offsetX+x, offsetY+y, src.At(srcX, srcY))
		}
	})
}

func fillEdgeGradientPadding(targetImg *image.RGBA, offsetX, offsetY, scaledWidth, scaledHeight int) {
	targetBounds := targetImg.Bounds()
	targetWidth := targetBounds.Dx()
	targetHeight := targetBounds.Dy()

	imgX0 := offsetX
	imgY0 := offsetY
	imgX1 := offsetX + scaledWidth - 1
	imgY1 := offsetY + scaledHeight - 1

	// Compute per-side gradient targets (black/white) using average edge luminance
	leftTarget, rightTarget, topTarget, bottomTarget := computeBandTargets(targetImg, imgX0, imgX1, imgY0, imgY1, targetWidth, targetHeight)

	// Left band [0, imgX0)
	if imgX0 > 0 {
		tAtX := computeLinearTFunc(0, imgX0-1, true)
		fillVerticalBand(targetImg, 0, imgX0, imgX0, imgY0, imgY1, leftTarget, tAtX)
	}

	// Right band (imgX1, targetWidth-1]
	rbStart := imgX1 + 1
	if rbStart < targetWidth {
		tAtX := computeLinearTFunc(rbStart, targetWidth-1, false)
		fillVerticalBand(targetImg, rbStart, targetWidth, imgX1, imgY0, imgY1, rightTarget, tAtX)
	}

	// Top band [0, imgY0) over [imgX0..imgX1]
	if imgY0 > 0 {
		tAtY := computeLinearTFunc(0, imgY0-1, true)
		fillHorizontalBand(targetImg, 0, imgY0, imgY0, imgX0, imgX1, topTarget, tAtY)
	}

	// Bottom band (imgY1, targetHeight-1] over [imgX0..imgX1]
	bbStart := imgY1 + 1
	if bbStart < targetHeight {
		tAtY := computeLinearTFunc(bbStart, targetHeight-1, false)
		fillHorizontalBand(targetImg, bbStart, targetHeight, imgY1, imgX0, imgX1, bottomTarget, tAtY)
	}
}

// computeBandTargets determines the black/white target per band using average luminance.
func computeBandTargets(img *image.RGBA, imgX0, imgX1, imgY0, imgY1, targetWidth, targetHeight int) (left, right, top, bottom color.RGBA) {
	left = color.RGBA{255, 255, 255, 255}
	right = color.RGBA{255, 255, 255, 255}
	top = color.RGBA{255, 255, 255, 255}
	bottom = color.RGBA{255, 255, 255, 255}

	if imgX0 > 0 {
		l := avgEdgeLuminanceColumn(img, imgX0, imgY0, imgY1)
		left = chooseBWTargetFromLuma(l)
	}
	if imgX1 < targetWidth-1 {
		l := avgEdgeLuminanceColumn(img, imgX1, imgY0, imgY1)
		right = chooseBWTargetFromLuma(l)
	}
	if imgY0 > 0 {
		l := avgEdgeLuminanceRow(img, imgY0, imgX0, imgX1)
		top = chooseBWTargetFromLuma(l)
	}
	if imgY1 < targetHeight-1 {
		l := avgEdgeLuminanceRow(img, imgY1, imgX0, imgX1)
		bottom = chooseBWTargetFromLuma(l)
	}
	return
}

// fillVerticalBand fills a vertical padding band (left/right) with a gradient to the given target.
func fillVerticalBand(img *image.RGBA, xStart, xEnd int, edgeX, imgY0, imgY1 int, target color.RGBA, tAtX func(x int) float64) {
	h := img.Bounds().Dy()
	for y := 0; y < h; y++ {
		ey := clampInt(y, imgY0, imgY1)
		edge := img.RGBAAt(edgeX, ey)
		for x := xStart; x < xEnd; x++ {
			t := tAtX(x)
			c := blendEdgeToTarget(edge, target, t)
			img.SetRGBA(x, y, c)
		}
	}
}

// fillHorizontalBand fills a horizontal padding band (top/bottom) with a gradient to the given target.
func fillHorizontalBand(img *image.RGBA, yStart, yEnd int, edgeY, imgX0, imgX1 int, target color.RGBA, tAtY func(y int) float64) {
	for y := yStart; y < yEnd; y++ {
		t := tAtY(y)
		for x := imgX0; x <= imgX1; x++ {
			edge := img.RGBAAt(x, edgeY)
			c := blendEdgeToTarget(edge, target, t)
			img.SetRGBA(x, y, c)
		}
	}
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	bb := img.Bounds()
	// Pre-grow buffer to reduce re-allocations; rough heuristic: 1 byte per pixel
	buf.Grow(bb.Dx() * bb.Dy())
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func lerp8(a, b uint8, t float64) uint8 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return uint8(float64(a)*(1.0-t) + float64(b)*t + 0.5)
}

// blendEdgeToTarget builds a color by blending edge color towards target by factor t in [0..1].
func blendEdgeToTarget(edge, target color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: lerp8(edge.R, target.R, t),
		G: lerp8(edge.G, target.G, t),
		B: lerp8(edge.B, target.B, t),
		A: 255,
	}
}

// computeLinearTFunc returns a function that maps an integer coordinate i in [start..end]
// to a normalized position t in [0..1] across the interval. When invert is true,
// t=0 at end and t=1 at start (reverse fade), otherwise t=0 at start and t=1 at end.
func computeLinearTFunc(start, end int, invert bool) func(i int) float64 {
	denom := end - start + 1
	if denom < 1 {
		denom = 1
	}
	if invert {
		return func(i int) float64 {
			return float64(end-i) / float64(denom)
		}
	}
	return func(i int) float64 {
		return float64(i-start) / float64(denom)
	}
}

// chooseBWTargetFromLuma selects black or white given a luminance value [0..255].
func chooseBWTargetFromLuma(y float64) color.RGBA {
	if y < 127.5 {
		return color.RGBA{0, 0, 0, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

// avgEdgeLuminanceInterval computes average luminance over an integer interval [start..end],
// sampling colors via the provided callback. Returns 0 when end < start.
func avgEdgeLuminanceInterval(start, end int, sample func(i int) color.RGBA) float64 {
	if end < start {
		return 0
	}
	sum := 0.0
	n := 0
	for i := start; i <= end; i++ {
		c := sample(i)
		sum += 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
		n++
	}
	return sum / float64(n)
}

// avgEdgeLuminanceColumn computes average luminance along a column x from y0..y1 inclusive.
func avgEdgeLuminanceColumn(img *image.RGBA, x, y0, y1 int) float64 {
	return avgEdgeLuminanceInterval(y0, y1, func(y int) color.RGBA {
		return img.RGBAAt(x, y)
	})
}

// avgEdgeLuminanceRow computes average luminance along a row y from x0..x1 inclusive.
func avgEdgeLuminanceRow(img *image.RGBA, y, x0, x1 int) float64 {
	return avgEdgeLuminanceInterval(x0, x1, func(x int) color.RGBA {
		return img.RGBAAt(x, y)
	})
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("ScaleCommand", NewScaleCommand); err != nil {
		panic(fmt.Sprintf("failed to register ScaleCommand: %v", err))
	}
}
