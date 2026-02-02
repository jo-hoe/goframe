package commands

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"strings"

	"github.com/jo-hoe/goframe/internal/backend/commandstructure"

	_ "image/gif"
	_ "image/jpeg"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// hasCorrectPngSignature checks whether the provided data begins with a valid PNG signature
func hasCorrectPngSignature(data []byte) bool {
	// PNG signature: 0x89 'P' 'N' 'G' 0x0D 0x0A 0x1A 0x0A
	if len(data) < 8 {
		return false
	}
	expected := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	return bytes.Equal(data[:8], expected)
}

// PngConverterCommand handles image format conversion to PNG
type PngConverterCommand struct {
	name              string
	svgFallbackWidth  int
	svgFallbackHeight int
}

// NewPngConverterCommand creates a new PNG converter command
func NewPngConverterCommand(params map[string]any) (commandstructure.Command, error) {
	// Read optional SVG fallback dimensions (used only when SVG lacks explicit size)
	w := commandstructure.GetIntParam(params, "svgFallbackWidth", 0)
	h := commandstructure.GetIntParam(params, "svgFallbackHeight", 0)

	return &PngConverterCommand{
		name:              "PngConverterCommand",
		svgFallbackWidth:  w,
		svgFallbackHeight: h,
	}, nil
}

// NewPngConverterCommandDirect creates a new PNG converter command directly (no parameters needed)
func NewPngConverterCommandDirect() *PngConverterCommand {
	return &PngConverterCommand{
		name:              "PngConverterCommand",
		svgFallbackWidth:  0,
		svgFallbackHeight: 0,
	}
}

// Name returns the command name
func (c *PngConverterCommand) Name() string {
	return c.name
}

func (c *PngConverterCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("PngConverterCommand: start",
		"input_size_bytes", len(imageData),
		"svg_fallback_width", c.svgFallbackWidth,
		"svg_fallback_height", c.svgFallbackHeight)

	// If input is already PNG, return original bytes (no scaling for raster formats here)
	if hasCorrectPngSignature(imageData) {
		slog.Debug("PngConverterCommand: PNG detected; returning original bytes")
		return imageData, nil
	}

	// Handle SVG input explicitly
	if isSVGData(imageData) {
		return c.convertSVG(imageData)
	}

	// Decode raster image (supports multiple formats via imported decoders)
	img, currentFormat, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("PngConverterCommand: failed to decode image", "error", err)
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	slog.Debug("PngConverterCommand: decoded raster image",
		"current_format", currentFormat,
		"orig_width", img.Bounds().Dx(),
		"orig_height", img.Bounds().Dy())

	// Encode decoded raster image directly to PNG (no scaling here)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		slog.Error("PngConverterCommand: failed to encode image to PNG", "error", err)
		return nil, fmt.Errorf("failed to encode image to PNG: %w", err)
	}
	slog.Debug("PngConverterCommand: raster conversion complete", "output_size_bytes", buf.Len())
	return buf.Bytes(), nil
}

func (c *PngConverterCommand) convertSVG(imageData []byte) ([]byte, error) {
	slog.Debug("PngConverterCommand: detected SVG input; determining render size")

	// Try to extract explicit width/height from SVG; if missing, use fallback
	if w, h, ok := parseSvgExplicitSize(imageData); ok {
		slog.Debug("PngConverterCommand: SVG has explicit size", "width", w, "height", h)
		out, err := renderSVGToPNG(imageData, w, h)
		if err != nil {
			slog.Error("PngConverterCommand: failed to render SVG (explicit size)", "error", err)
			return nil, fmt.Errorf("failed to render SVG to PNG: %w", err)
		}
		slog.Debug("PngConverterCommand: SVG render complete", "output_size_bytes", len(out))
		return out, nil
	}

	fw := c.svgFallbackWidth
	fh := c.svgFallbackHeight
	if fw <= 0 || fh <= 0 {
		slog.Error("PngConverterCommand: SVG fallback size not set; cannot render SVG without explicit size")
		return nil, fmt.Errorf("SVG fallback size not set; cannot render SVG without explicit size")
	}
	slog.Debug("PngConverterCommand: SVG lacks explicit size; using fallback", "width", fw, "height", fh)
	out, err := renderSVGToPNG(imageData, fw, fh)
	if err != nil {
		slog.Error("PngConverterCommand: failed to render SVG (fallback size)", "error", err)
		return nil, fmt.Errorf("failed to render SVG to PNG: %w", err)
	}
	slog.Debug("PngConverterCommand: SVG render complete", "output_size_bytes", len(out))
	return out, nil
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("PngConverterCommand", NewPngConverterCommand); err != nil {
		panic(fmt.Sprintf("failed to register PngConverterCommand: %v", err))
	}
}

// parseSvgExplicitSize attempts to extract width and height attributes from the SVG.
// Returns width, height, and ok=true if both are found and parseable.
func parseSvgExplicitSize(data []byte) (int, int, bool) {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	s := strings.ToLower(string(data[:n]))
	// Find <svg ...> start
	i := strings.Index(s, "<svg")
	if i < 0 {
		return 0, 0, false
	}
	// Limit to the start tag portion up to '>'
	j := strings.Index(s[i:], ">")
	if j < 0 {
		j = len(s)
	} else {
		j = i + j
	}
	tag := s[i:j]

	w, wOk := parseNumericAttr(tag, "width")
	h, hOk := parseNumericAttr(tag, "height")
	if wOk && hOk && w > 0 && h > 0 {
		return w, h, true
	}
	// If no explicit width/height, do not treat viewBox as pixel size; use fallback.
	return 0, 0, false
}

// parseNumericAttr extracts the leading numeric value of an attribute (e.g., width="123px").
// Returns the integer value and ok=true if found.
func parseNumericAttr(tag, attr string) (int, bool) {
	key := attr + "="
	pos := strings.Index(tag, key)
	if pos < 0 {
		// Try with spaces and quotes variations
		pos = strings.Index(tag, attr)
		if pos < 0 {
			return 0, false
		}
	}
	// Find first quote after the attr name
	q := strings.Index(tag[pos:], "\"")
	single := strings.Index(tag[pos:], "'")
	start := -1
	quoteChar := byte(0)
	if q >= 0 && (single < 0 || q < single) {
		start = pos + q + 1
		quoteChar = '"'
	} else if single >= 0 {
		start = pos + single + 1
		quoteChar = '\''
	}
	if start < 0 || start >= len(tag) {
		return 0, false
	}
	// Read until matching quote
	end := strings.IndexByte(tag[start:], quoteChar)
	val := tag[start:]
	if end >= 0 {
		val = tag[start : start+end]
	}
	// Extract leading number
	num := 0
	found := false
	for i := 0; i < len(val); i++ {
		ch := val[i]
		if ch >= '0' && ch <= '9' {
			found = true
			num = num*10 + int(ch-'0')
		} else if found {
			break
		}
	}
	if !found || num <= 0 {
		return 0, false
	}
	return num, true
}

// isSVGData performs a lightweight detection of SVG content from raw bytes.
// It checks for "<svg" tag or SVG namespace in the initial portion of the data.
func isSVGData(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Only inspect the first ~4KB for detection
	n := len(data)
	if n > 4096 {
		n = 4096
	}
	header := bytes.ToLower(bytes.TrimSpace(data[:n]))
	return bytes.HasPrefix(header, []byte("<svg")) ||
		bytes.Contains(header, []byte("<svg")) ||
		bytes.Contains(header, []byte("xmlns=\"http://www.w3.org/2000/svg\"")) ||
		bytes.Contains(header, []byte("xmlns='http://www.w3.org/2000/svg'"))
}

// renderSVGToPNG renders an SVG byte slice into a PNG with the given target dimensions.
func renderSVGToPNG(svgData []byte, targetW, targetH int) ([]byte, error) {
	if targetW <= 0 || targetH <= 0 {
		return nil, fmt.Errorf("invalid target dimensions for SVG rendering: %dx%d", targetW, targetH)
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svgData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SVG: %w", err)
	}

	// Set drawing target rectangle
	icon.SetTarget(0, 0, float64(targetW), float64(targetH))

	// Prepare target canvas (white background)
	dst := createTargetCanvas(targetW, targetH, color.RGBA{255, 255, 255, 255})

	// Rasterize SVG into the target canvas
	scanner := rasterx.NewScannerGV(targetW, targetH, dst, dst.Bounds())
	dasher := rasterx.NewDasher(targetW, targetH, scanner)
	icon.Draw(dasher, 1.0)

	// Encode to PNG
	var buf bytes.Buffer
	buf.Grow(targetW * targetH)
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("failed to encode rendered SVG as PNG: %w", err)
	}
	return buf.Bytes(), nil
}
