package main

import (
	"bytes"
	"image"
	_ "image/png"

	"github.com/lxn/walk"
)

// loadWalkIcon decodes the embedded logo.png and returns a *walk.Icon.
// The image is scaled to 32x32 (nearest-neighbour) so it looks sharp in the tray.
func loadWalkIcon() (*walk.Icon, error) {
	img, _, err := image.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return nil, err
	}
	return walk.NewIconFromImage(resizeNearestNeighbour(img, 32, 32))
}

// resizeNearestNeighbour scales src to w×h using nearest-neighbour sampling.
func resizeNearestNeighbour(src image.Image, w, h int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(b.Min.X+x*sw/w, b.Min.Y+y*sh/h))
		}
	}
	return dst
}
