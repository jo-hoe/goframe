package imagevalidation

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func mustPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func mustJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func TestValidate(t *testing.T) {
	png := mustPNG(t)
	jpg := mustJPEG(t)
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)

	cases := []struct {
		name     string
		data     []byte
		maxBytes int64
		wantErr  bool
	}{
		{"valid png", png, 0, false},
		{"valid jpeg", jpg, 0, false},
		{"valid svg", svg, 0, false},
		{"empty", nil, 0, true},
		{"garbage", []byte("this is not an image at all"), 0, true},
		{"too large", png, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.data, tc.maxBytes)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil {
				if _, ok := AsValidationError(err); !ok {
					t.Fatalf("expected *ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestLooksLikeSVG(t *testing.T) {
	if !looksLikeSVG([]byte("  <SVG>")) {
		t.Fatal("expected uppercase <SVG> to be detected")
	}
	if looksLikeSVG([]byte("plain text")) {
		t.Fatal("plain text should not look like svg")
	}
	if looksLikeSVG([]byte{0xFF, 0xD8, 0xFF}) {
		t.Fatal("binary should not look like svg")
	}
}
