package imageprocessing

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
)

// NormalizeOrientation represents the EXIF orientation tag value (1–8).
type NormalizeOrientation uint16

const (
	NormalizeOrientationNormal            NormalizeOrientation = 1 // No transform needed
	NormalizeOrientationFlipHorizontal    NormalizeOrientation = 2 // Flip left-right
	NormalizeOrientationRotate180         NormalizeOrientation = 3 // Rotate 180°
	NormalizeOrientationFlipVertical      NormalizeOrientation = 4 // Flip top-bottom
	NormalizeOrientationTranspose         NormalizeOrientation = 5 // Rotate 90° CW + flip horizontal
	NormalizeOrientationRotate90CW        NormalizeOrientation = 6 // Rotate 90° CW
	NormalizeOrientationTransverse        NormalizeOrientation = 7 // Rotate 90° CCW + flip horizontal
	NormalizeOrientationRotate90CCW       NormalizeOrientation = 8 // Rotate 90° CCW
)

// ReadJPEGOrientation parses the EXIF orientation tag from raw JPEG bytes.
// Returns NormalizeOrientationNormal when the tag is absent or the data is not a JPEG.
func ReadJPEGOrientation(data []byte) (NormalizeOrientation, error) {
	if !isJPEG(data) {
		return NormalizeOrientationNormal, fmt.Errorf("data is not a JPEG")
	}
	orientation, err := scanJPEGForOrientation(data)
	if err != nil {
		return NormalizeOrientationNormal, err
	}
	return orientation, nil
}

// ApplyOrientation transforms img so that the image content is upright,
// equivalent to what a viewer would show after honouring the EXIF tag.
// Returns the original image unchanged for NormalizeOrientationNormal.
func ApplyOrientation(img image.Image, o NormalizeOrientation) image.Image {
	switch o {
	case NormalizeOrientationNormal:
		return img
	case NormalizeOrientationFlipHorizontal:
		return flipHorizontal(img)
	case NormalizeOrientationRotate180:
		return applyRotationSteps(img, Steps180, true)
	case NormalizeOrientationFlipVertical:
		return flipVertical(img)
	case NormalizeOrientationTranspose:
		return flipHorizontal(applyRotationSteps(img, Steps90, true))
	case NormalizeOrientationRotate90CW:
		return applyRotationSteps(img, Steps90, true)
	case NormalizeOrientationTransverse:
		return flipHorizontal(applyRotationSteps(img, Steps90, false))
	case NormalizeOrientationRotate90CCW:
		return applyRotationSteps(img, Steps90, false)
	default:
		return img
	}
}

// isJPEG returns true when data begins with the JPEG SOI marker (0xFF 0xD8).
func isJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}

// scanJPEGForOrientation walks JPEG segments looking for APP1 / EXIF.
func scanJPEGForOrientation(data []byte) (NormalizeOrientation, error) {
	pos := 2 // skip SOI
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return NormalizeOrientationNormal, nil // lost sync, treat as absent
		}
		marker := data[pos+1]
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		payload := data[pos+4 : pos+2+segLen]

		if marker == 0xE1 { // APP1
			if o, ok, err := parseExifAPP1(payload); err != nil {
				return NormalizeOrientationNormal, err
			} else if ok {
				return o, nil
			}
		}

		pos += 2 + segLen
	}
	return NormalizeOrientationNormal, nil
}

// parseExifAPP1 attempts to extract the orientation tag from an APP1 payload.
// Returns (orientation, true, nil) when found, (0, false, nil) when absent.
func parseExifAPP1(payload []byte) (NormalizeOrientation, bool, error) {
	// APP1 payload starts with "Exif\x00\x00" (6 bytes) then a TIFF header.
	const exifHeader = "Exif\x00\x00"
	if len(payload) < len(exifHeader)+8 {
		return 0, false, nil
	}
	if string(payload[:len(exifHeader)]) != exifHeader {
		return 0, false, nil
	}
	tiff := payload[len(exifHeader):]
	return parseTIFFOrientation(tiff)
}

// parseTIFFOrientation reads the orientation tag from a TIFF-encoded IFD0.
func parseTIFFOrientation(tiff []byte) (NormalizeOrientation, bool, error) {
	if len(tiff) < 8 {
		return 0, false, nil
	}
	bo, err := tiffByteOrder(tiff)
	if err != nil {
		return 0, false, err
	}

	ifdOffset := bo.Uint32(tiff[4:8])
	return readIFD0Orientation(tiff, ifdOffset, bo)
}

// tiffByteOrder returns the byte order declared in the TIFF header.
func tiffByteOrder(tiff []byte) (binary.ByteOrder, error) {
	switch string(tiff[:2]) {
	case "II":
		return binary.LittleEndian, nil
	case "MM":
		return binary.BigEndian, nil
	default:
		return nil, fmt.Errorf("invalid TIFF byte-order marker: %q", tiff[:2])
	}
}

// readIFD0Orientation scans IFD0 entries for the Orientation tag (0x0112).
func readIFD0Orientation(tiff []byte, ifdOffset uint32, bo binary.ByteOrder) (NormalizeOrientation, bool, error) {
	const ifdEntrySize = 12
	const tagOrientation = uint16(0x0112)

	if uint32(len(tiff)) < ifdOffset+2 {
		return 0, false, nil
	}
	entryCount := bo.Uint16(tiff[ifdOffset : ifdOffset+2])

	for i := uint16(0); i < entryCount; i++ {
		entryOffset := ifdOffset + 2 + uint32(i)*ifdEntrySize
		if uint32(len(tiff)) < entryOffset+ifdEntrySize {
			break
		}
		tag := bo.Uint16(tiff[entryOffset : entryOffset+2])
		if tag != tagOrientation {
			continue
		}
		val := bo.Uint16(tiff[entryOffset+8 : entryOffset+10])
		if val < 1 || val > 8 {
			return NormalizeOrientationNormal, true, nil
		}
		return NormalizeOrientation(val), true, nil
	}
	return 0, false, nil
}

// flipHorizontal mirrors img left-to-right.
func flipHorizontal(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.X-1-x+b.Min.X, y, img.At(x, y))
		}
	}
	return dst
}

// flipVertical mirrors img top-to-bottom.
func flipVertical(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, b.Max.Y-1-y+b.Min.Y, img.At(x, y))
		}
	}
	return dst
}

// toRGBA converts any image.Image to *image.RGBA for uniform pixel access.
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}
