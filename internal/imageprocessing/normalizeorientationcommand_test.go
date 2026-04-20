package imageprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeJPEGWithOrientation creates a minimal JPEG with an injected EXIF orientation tag.
// The pixel content is a 4x2 RGBA image with a red top-left pixel for orientation verification.
func makeJPEGWithOrientation(t *testing.T, orientation NormalizeOrientation) []byte {
	t.Helper()
	raw := makeOrientationTestImage()
	jpegBytes := encodeAsJPEG(t, raw)
	return injectNormalizeOrientation(t, jpegBytes, orientation)
}

func makeOrientationTestImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255}) // red top-left marker
	return img
}

func encodeAsJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// injectNormalizeOrientation splices a minimal EXIF APP1 segment carrying the given
// orientation tag into a JPEG byte slice directly after the SOI marker.
func injectNormalizeOrientation(t *testing.T, jpegBytes []byte, o NormalizeOrientation) []byte {
	t.Helper()
	app1 := buildExifAPP1(o)
	out := make([]byte, 0, 2+len(app1)+len(jpegBytes)-2)
	out = append(out, jpegBytes[:2]...) // SOI
	out = append(out, app1...)
	out = append(out, jpegBytes[2:]...) // rest of original JPEG
	return out
}

// buildExifAPP1 constructs a minimal big-endian EXIF APP1 segment with only the
// Orientation tag in IFD0.
func buildExifAPP1(o NormalizeOrientation) []byte {
	// TIFF header (big-endian): "MM" + magic 42 + IFD0 offset (8)
	// IFD0: 1 entry — Orientation (0x0112), type SHORT (3), count 1, value uint16
	ifd := []byte{
		0x00, 0x01, // entry count = 1
		0x01, 0x12, // tag = 0x0112 (Orientation)
		0x00, 0x03, // type = SHORT
		0x00, 0x00, 0x00, 0x01, // count = 1
		0x00, byte(o), // value (big-endian uint16, high byte always 0 for 1-8)
		0x00, 0x00, // padding to fill value field to 4 bytes
		0x00, 0x00, 0x00, 0x00, // next IFD offset = 0 (none)
	}
	tiff := []byte{
		'M', 'M', // big-endian
		0x00, 0x2A, // TIFF magic
		0x00, 0x00, 0x00, 0x08, // IFD0 at offset 8
	}
	tiff = append(tiff, ifd...)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	segLen := uint16(2 + len(payload)) // length field includes itself
	header := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen)}
	return append(header, payload...)
}

// --- ReadJPEGOrientation tests ---

func TestReadJPEGOrientation_Normal(t *testing.T) {
	data := makeJPEGWithOrientation(t, NormalizeOrientationNormal)
	got, err := ReadJPEGOrientation(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != NormalizeOrientationNormal {
		t.Errorf("expected %d, got %d", NormalizeOrientationNormal, got)
	}
}

func TestReadJPEGOrientation_Rotate180(t *testing.T) {
	data := makeJPEGWithOrientation(t, NormalizeOrientationRotate180)
	got, err := ReadJPEGOrientation(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != NormalizeOrientationRotate180 {
		t.Errorf("expected %d, got %d", NormalizeOrientationRotate180, got)
	}
}

func TestReadJPEGOrientation_AllValues(t *testing.T) {
	for o := NormalizeOrientationNormal; o <= NormalizeOrientationRotate90CCW; o++ {
		data := makeJPEGWithOrientation(t, o)
		got, err := ReadJPEGOrientation(data)
		if err != nil {
			t.Fatalf("orientation %d: unexpected error: %v", o, err)
		}
		if got != o {
			t.Errorf("orientation %d: got %d", o, got)
		}
	}
}

func TestReadJPEGOrientation_NotJPEG(t *testing.T) {
	_, err := ReadJPEGOrientation([]byte("not a jpeg"))
	if err == nil {
		t.Error("expected error for non-JPEG input")
	}
}

func TestReadJPEGOrientation_NoExif(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	got, err := ReadJPEGOrientation(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != NormalizeOrientationNormal {
		t.Errorf("expected Normal for JPEG without EXIF, got %d", got)
	}
}

// --- ApplyOrientation tests ---

func TestApplyOrientation_Normal(t *testing.T) {
	img := makeOrientationTestImage()
	result := ApplyOrientation(img, NormalizeOrientationNormal)
	if result != img {
		t.Error("Normal orientation should return the same image instance")
	}
}

func TestApplyOrientation_Rotate180(t *testing.T) {
	img := makeOrientationTestImage() // 4w x 2h, red at (0,0)
	result := ApplyOrientation(img, NormalizeOrientationRotate180)
	b := result.Bounds()
	if b.Dx() != 4 || b.Dy() != 2 {
		t.Errorf("180° should preserve dimensions, got %dx%d", b.Dx(), b.Dy())
	}
	// red (0,0) should move to bottom-right (3,1)
	if result.At(3, 1) != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red at (3,1) after 180°, got %v", result.At(3, 1))
	}
}

func TestApplyOrientation_Rotate90CW(t *testing.T) {
	img := makeOrientationTestImage() // 4w x 2h, red at (0,0)
	result := ApplyOrientation(img, NormalizeOrientationRotate90CW)
	b := result.Bounds()
	// 90° CW swaps dimensions: 4x2 → 2x4
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("90° CW should produce 2x4, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestApplyOrientation_Rotate90CCW(t *testing.T) {
	img := makeOrientationTestImage() // 4w x 2h
	result := ApplyOrientation(img, NormalizeOrientationRotate90CCW)
	b := result.Bounds()
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("90° CCW should produce 2x4, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestApplyOrientation_FlipHorizontal(t *testing.T) {
	img := makeOrientationTestImage() // 4w x 2h, red at (0,0)
	result := ApplyOrientation(img, NormalizeOrientationFlipHorizontal)
	// red should move from (0,0) to (3,0)
	if result.At(3, 0) != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red at (3,0) after flip horizontal, got %v", result.At(3, 0))
	}
}

// --- NormalizeOrientationCommand tests ---

func TestNormalizeOrientationCommand_Name(t *testing.T) {
	cmd, err := NewNormalizeOrientationCommandWithParams()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name() != "NormalizeOrientationCommand" {
		t.Errorf("expected 'NormalizeOrientationCommand', got %q", cmd.Name())
	}
}

func TestNormalizeOrientationCommand_NoExif_ReturnsUnchanged(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	input := buf.Bytes()

	cmd, _ := NewNormalizeOrientationCommandWithParams()
	out, err := cmd.Execute(input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Equal(out, input) {
		t.Error("expected unchanged output for JPEG without EXIF")
	}
}

func TestNormalizeOrientationCommand_NotJPEG_ReturnsUnchanged(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	input := buf.Bytes()

	cmd, _ := NewNormalizeOrientationCommandWithParams()
	out, err := cmd.Execute(input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Equal(out, input) {
		t.Error("expected unchanged output for PNG input")
	}
}

func TestNormalizeOrientationCommand_Rotate180_CorrectsDimensions(t *testing.T) {
	input := makeJPEGWithOrientation(t, NormalizeOrientationRotate180)

	cmd, _ := NewNormalizeOrientationCommandWithParams()
	out, err := cmd.Execute(input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result is not valid PNG: %v", err)
	}
	b := result.Bounds()
	if b.Dx() != 4 || b.Dy() != 2 {
		t.Errorf("expected 4x2 after 180° correction, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestNormalizeOrientationCommand_Rotate90CW_SwapsDimensions(t *testing.T) {
	input := makeJPEGWithOrientation(t, NormalizeOrientationRotate90CW)

	cmd, _ := NewNormalizeOrientationCommandWithParams()
	out, err := cmd.Execute(input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result is not valid PNG: %v", err)
	}
	b := result.Bounds()
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("expected 2x4 after 90° CW correction, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestNormalizeOrientationCommand_RegisteredInDefaultRegistry(t *testing.T) {
	if !DefaultRegistry.IsRegistered("NormalizeOrientationCommand") {
		t.Error("expected NormalizeOrientationCommand to be registered in DefaultRegistry")
	}
	cmd, err := DefaultRegistry.Create("NormalizeOrientationCommand", map[string]any{})
	if err != nil {
		t.Fatalf("Create via registry: %v", err)
	}
	if cmd.Name() != "NormalizeOrientationCommand" {
		t.Errorf("unexpected name: %q", cmd.Name())
	}
}

func TestNewNormalizeOrientationCommand_FromMap(t *testing.T) {
	cmd, err := NewNormalizeOrientationCommand(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name() != "NormalizeOrientationCommand" {
		t.Errorf("expected 'NormalizeOrientationCommand', got %q", cmd.Name())
	}
}
