package commands

import (
	"bytes"
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
