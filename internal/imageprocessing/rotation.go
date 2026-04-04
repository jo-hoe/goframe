package imageprocessing

import (
	"image"
)

// rotate90 rotates an image by exactly 90 degrees.
// If clockwise is true the rotation is clockwise, otherwise counterclockwise.
func rotate90(img image.Image, clockwise bool) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if clockwise {
				// (x,y) -> (h-1-y, x)
				dst.Set(h-1-y, x, img.At(x, y))
			} else {
				// (x,y) -> (y, w-1-x)
				dst.Set(y, w-1-x, img.At(x, y))
			}
		}
	}
	return dst
}

// applyRotationSteps applies steps × 90-degree rotations to img.
func applyRotationSteps(img image.Image, steps int, clockwise bool) image.Image {
	for range steps {
		img = rotate90(img, clockwise)
	}
	return img
}
