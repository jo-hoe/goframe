package commands

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"github.com/makeworld-the-better-one/dither/v2"
)

// Tolerance (wiggle) for considering a color "close enough" to Spectra6 palette.
// If a pixel's RGB is within this tolerance of any allowed Spectra6 color, we consider it allowed.
const spectra6ColorTolerance = 8

// Spectra6DitheringCommand handles image dithering using a fixed Spectra 6 palette
type Spectra6DitheringCommand struct {
	name string
}

// NewSpectra6DitheringCommand creates a new command instance (no configurable params)
func NewSpectra6DitheringCommand(params map[string]any) (commandstructure.Command, error) {
	return &Spectra6DitheringCommand{
		name: "Spectra6DitheringCommand",
	}, nil
}

// Name returns the command name
func (c *Spectra6DitheringCommand) Name() string {
	return c.name
}

/*
Execute applies dithering to the image.

Pre-check:
  - Decode the image and verify it only contains Spectra6 colors (Black, White, Red, Yellow, Blue, Green)
    within a defined tolerance. If so, skip dithering and return the original image unchanged.

Primary path:
- Use Node-based epdoptimize CLI with Spectra 6 defaults (palette + device colors).

Fallback path:
  - If Node or CLI is not available (e.g., in local tests), fall back to Go dithering
    using a fixed Spectra 6 palette with Floyd-Steinberg error diffusion and serpentine scanning.
*/
func (c *Spectra6DitheringCommand) Execute(imageData []byte) ([]byte, error) {
	slog.Debug("Spectra6DitheringCommand: starting execution", "input_size_bytes", len(imageData))

	// Decode once to perform pre-check and potentially reuse for Go fallback
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		slog.Error("Spectra6DitheringCommand: failed to decode PNG image", "error", err)
		return nil, fmt.Errorf("failed to decode PNG image: %w", err)
	}

	// Pre-check: if already only contains Spectra6 colors within tolerance, do nothing
	if isSpectra6Image(img, spectra6ColorTolerance) {
		slog.Info("Spectra6DitheringCommand: image already Spectra6 within tolerance, skipping dithering")
		return imageData, nil
	}

	// Try Node CLI first
	if out, err := c.tryEpdoptimizeCLI(imageData); err == nil {
		slog.Debug("Spectra6DitheringCommand: epdoptimize CLI succeeded", "output_size_bytes", len(out))
		return out, nil
	} else {
		slog.Warn("Spectra6DitheringCommand: using Go fallback", "reason", err)
	}

	// Fallback to Go dithering (reuse decoded image)
	return c.goDither(img)
}

// tryEpdoptimizeCLI attempts to run the Node-based epdoptimize CLI
func (c *Spectra6DitheringCommand) tryEpdoptimizeCLI(imageData []byte) ([]byte, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	cliPath := "/app/tools/epdoptimize-cli/index.mjs"
	if _, err := os.Stat(cliPath); err != nil {
		return nil, fmt.Errorf("cli not found at %s: %w", cliPath, err)
	}

	tmpDir, err := os.MkdirTemp("", "epdoptimize-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input.png")
	outPath := filepath.Join(tmpDir, "output.png")

	if err := os.WriteFile(inPath, imageData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write temp input image: %w", err)
	}

	// Align with Go fallback by explicitly setting Floyd-Steinberg matrix and serpentine scanning
	cmd := exec.Command(nodePath, cliPath, "--input", inPath, "--output", outPath, "--matrix", "floydSteinberg", "--serpentine", "true")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("epdoptimize CLI failed: %w; stderr: %s", err, stderr.String())
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read epdoptimize output: %w", err)
	}

	return outBytes, nil
}

// goDither performs dithering using the Go library as a fallback
func (c *Spectra6DitheringCommand) goDither(img image.Image) ([]byte, error) {
	slog.Debug("Spectra6DitheringCommand: creating ditherer (fallback)", "palette_size", len(spectra6PaletteColors()))

	d := dither.NewDitherer(spectra6PaletteColors())
	if d == nil {
		return nil, fmt.Errorf("failed to create ditherer with palette")
	}

	// Floyd-Steinberg error diffusion with serpentine scanning
	d.Matrix = dither.FloydSteinberg
	d.Serpentine = true

	slog.Debug("Spectra6DitheringCommand: applying dithering (fallback)")

	ditheredImg := d.Dither(img)

	slog.Debug("Spectra6DitheringCommand: encoding dithered image (fallback)")

	var buf bytes.Buffer
	if err := png.Encode(&buf, ditheredImg); err != nil {
		slog.Error("Spectra6DitheringCommand: failed to encode dithered image", "error", err)
		return nil, fmt.Errorf("failed to encode dithered PNG image: %w", err)
	}

	slog.Debug("Spectra6DitheringCommand: fallback dithering complete", "output_size_bytes", buf.Len())

	return buf.Bytes(), nil
}

// spectra6PaletteColors returns the fixed Spectra 6 palette as []color.Color.
// Default palette for Go should be: Black, White, Red, Yellow, Blue, Green.
func spectra6PaletteColors() []color.Color {
	return []color.Color{
		color.RGBA{0, 0, 0, 255},       // Black
		color.RGBA{255, 255, 255, 255}, // White
		color.RGBA{255, 0, 0, 255},     // Red
		color.RGBA{255, 255, 0, 255},   // Yellow
		color.RGBA{0, 0, 255, 255},     // Blue
		color.RGBA{0, 255, 0, 255},     // Green
	}
}

// isSpectra6Image checks whether all pixels in the image are within tolerance of Spectra6 palette colors.
func isSpectra6Image(img image.Image, tolerance int) bool {
	allowed := [][3]uint8{
		{0, 0, 0},       // Black
		{255, 255, 255}, // White
		{255, 0, 0},     // Red
		{255, 255, 0},   // Yellow
		{0, 0, 255},     // Blue
		{0, 255, 0},     // Green
	}

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(bl>>8)

			if !isAllowedColor(r8, g8, b8, allowed, tolerance) {
				return false
			}
		}
	}
	return true
}

func isAllowedColor(r8, g8, b8 uint8, allowed [][3]uint8, tol int) bool {
	for _, a := range allowed {
		if withinTolerance(r8, a[0], tol) &&
			withinTolerance(g8, a[1], tol) &&
			withinTolerance(b8, a[2], tol) {
			return true
		}
	}
	return false
}

func withinTolerance(v uint8, target uint8, tol int) bool {
	diff := int(v) - int(target)
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}

func init() {
	// Register the command in the default registry
	if err := commandstructure.DefaultRegistry.Register("Spectra6DitheringCommand", NewSpectra6DitheringCommand); err != nil {
		panic(fmt.Sprintf("failed to register Spectra6DitheringCommand: %v", err))
	}
}
