package commands

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func loadPeppers(b *testing.B) []byte {
	data, err := os.ReadFile("testdata/peppers.png")
	if err != nil {
		b.Fatalf("failed to load test image: %v", err)
	}
	return data
}

func BenchmarkPngConverterCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)
	command, err := NewPngConverterCommand(map[string]any{})
	if err != nil {
		b.Fatalf("failed to create PngConverterCommand: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := command.Execute(imageData); err != nil {
			b.Fatalf("execute failed: %v", err)
		}
	}
}

func BenchmarkScaleCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)

	cases := []struct {
		name   string
		height int
		width  int
	}{
		{"100x100", 100, 100},
		{"300x300", 300, 300},
		{"1024x768", 768, 1024},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			command, err := NewScaleCommand(map[string]any{
				"height": tc.height,
				"width":  tc.width,
			})
			if err != nil {
				b.Fatalf("failed to create ScaleCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkPixelScaleCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)

	cases := []struct {
		name        string
		heightParam any
		widthParam  any
	}{
		{"WidthOnly-300", nil, 300},
		{"HeightOnly-300", 300, nil},
		{"Both-640x480", 480, 640},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			params := map[string]any{}
			if tc.heightParam != nil {
				params["height"] = tc.heightParam
			}
			if tc.widthParam != nil {
				params["width"] = tc.widthParam
			}

			command, err := NewPixelScaleCommand(params)
			if err != nil {
				b.Fatalf("failed to create PixelScaleCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkOrientationCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		b.Fatalf("failed to decode image for orientation inspection: %v", err)
	}
	bounds := img.Bounds()
	isPortrait := bounds.Dy() >= bounds.Dx()

	// Force rotation by choosing the opposite of the current orientation
	target := "portrait"
	if isPortrait {
		target = "landscape"
	}

	command, err := NewOrientationCommand(map[string]any{
		"orientation": target,
	})
	if err != nil {
		b.Fatalf("failed to create OrientationCommand: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := command.Execute(imageData); err != nil {
			b.Fatalf("execute failed: %v", err)
		}
	}
}

func BenchmarkDitherCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)

	b.Run("DefaultPalette", func(b *testing.B) {
		command, err := NewDitherCommand(map[string]any{})
		if err != nil {
			b.Fatalf("failed to create DitherCommand: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := command.Execute(imageData); err != nil {
				b.Fatalf("execute failed: %v", err)
			}
		}
	})

	b.Run("DefaultPalette-Strength-0.8", func(b *testing.B) {
		command, err := NewDitherCommand(map[string]any{
			"strength": 0.8,
		})
		if err != nil {
			b.Fatalf("failed to create DitherCommand: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := command.Execute(imageData); err != nil {
				b.Fatalf("execute failed: %v", err)
			}
		}
	})
}

func BenchmarkCropCommand_Execute(b *testing.B) {
	imageData := loadPeppers(b)

	cases := []struct {
		name   string
		height int
		width  int
	}{
		{"100x100", 100, 100},
		{"300x300", 300, 300},
		{"500x500", 500, 500},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			command, err := NewCropCommand(map[string]any{
				"height": tc.height,
				"width":  tc.width,
			})
			if err != nil {
				b.Fatalf("failed to create CropCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

// ========== Large-image synthetic benchmarks ==========

// makeLargePNG creates a synthetic PNG image of given size with a simple gradient.
// Larger images better expose parallel speedups.
func makeLargePNG(b *testing.B, width, height int) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Simple gradient fill
	for y := 0; y < height; y++ {
		yy := uint8((y * 255) / height) // #nosec G115 -- computed gradient is in 0..255 for 0<=y<height
		for x := 0; x < width; x++ {
			xx := uint8((x * 255) / width) // #nosec G115 -- computed gradient is in 0..255 for 0<=x<width
			img.Set(x, y, color.RGBA{R: xx, G: yy, B: (xx + yy) / 2, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatalf("failed to encode synthetic PNG: %v", err)
	}
	return buf.Bytes()
}

func BenchmarkScaleCommand_Execute_Large(b *testing.B) {
	// 4000x3000 landscape synthetic image
	imageData := makeLargePNG(b, 4000, 3000)

	cases := []struct {
		name   string
		height int
		width  int
	}{
		{"1920x1080", 1080, 1920},
		{"3000x2000", 2000, 3000},
		{"800x600", 600, 800},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			command, err := NewScaleCommand(map[string]any{
				"height": tc.height,
				"width":  tc.width,
			})
			if err != nil {
				b.Fatalf("failed to create ScaleCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkPixelScaleCommand_Execute_Large(b *testing.B) {
	imageData := makeLargePNG(b, 4000, 3000)

	cases := []struct {
		name        string
		heightParam any
		widthParam  any
	}{
		{"WidthOnly-1920", nil, 1920},
		{"HeightOnly-2000", 2000, nil},
		{"Both-2560x1440", 1440, 2560},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			params := map[string]any{}
			if tc.heightParam != nil {
				params["height"] = tc.heightParam
			}
			if tc.widthParam != nil {
				params["width"] = tc.widthParam
			}

			command, err := NewPixelScaleCommand(params)
			if err != nil {
				b.Fatalf("failed to create PixelScaleCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkCropCommand_Execute_Large(b *testing.B) {
	imageData := makeLargePNG(b, 4000, 3000)

	cases := []struct {
		name   string
		height int
		width  int
	}{
		{"2000x2000", 2000, 2000},
		{"3500x2500", 2500, 3500},
		{"800x1200", 1200, 800},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			command, err := NewCropCommand(map[string]any{
				"height": tc.height,
				"width":  tc.width,
			})
			if err != nil {
				b.Fatalf("failed to create CropCommand: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := command.Execute(imageData); err != nil {
					b.Fatalf("execute failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkOrientationCommand_Execute_Large(b *testing.B) {
	// Use landscape synthetic image; force rotation to portrait to ensure work is done
	imageData := makeLargePNG(b, 4000, 3000)

	command, err := NewOrientationCommand(map[string]any{
		"orientation": "portrait",
	})
	if err != nil {
		b.Fatalf("failed to create OrientationCommand: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := command.Execute(imageData); err != nil {
			b.Fatalf("execute failed: %v", err)
		}
	}
}
