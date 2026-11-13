package render

import (
	"fmt"
	"image"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// The e-ink panel shows 16 grays, 0, 17, 34 ... 255.
const grayStep = 17

// panel is a widget's box and how strongly it lightens the picture behind it.
type panel struct {
	rect    image.Rectangle
	lighten float64
}

type backdropKey struct {
	background *image.Gray
	panels     string
}

// The backdrop only changes with the picture or the panels, and
// dithering a full screen is slow on the Kindle, so the last one is kept.
var lastBackdrop struct {
	sync.Mutex
	key backdropKey
	img *image.Gray
}

// backdrop is the background with the widget panels lightened, dithered to the
// panel's grays so smooth shading does not break into bands.
func backdrop(background *image.Gray, panels []panel) *image.Gray {
	key := backdropKey{background, fmt.Sprint(panels)}
	lastBackdrop.Lock()
	defer lastBackdrop.Unlock()
	if lastBackdrop.img != nil && lastBackdrop.key == key {
		return lastBackdrop.img
	}
	img := image.NewGray(image.Rect(0, 0, Width, Height))
	xdraw.Draw(img, img.Bounds(), background, image.Point{}, xdraw.Src)
	p := painter{dst: img}
	for _, pn := range panels {
		p.lighten(pn.rect, pn.lighten)
	}
	dither(img, image.Point{})
	lastBackdrop.key, lastBackdrop.img = key, img
	return img
}

// threshold8x8 is the ordered dither map KOReader uses (ImageMagick's o8x8).
var threshold8x8 = [64]int{
	1, 49, 13, 61, 4, 52, 16, 64,
	33, 17, 45, 29, 36, 20, 48, 32,
	9, 57, 5, 53, 12, 60, 8, 56,
	41, 25, 37, 21, 44, 28, 40, 24,
	3, 51, 15, 63, 2, 50, 14, 62,
	35, 19, 47, 31, 34, 18, 46, 30,
	11, 59, 7, 55, 10, 58, 6, 54,
	43, 27, 39, 23, 42, 26, 38, 22,
}

// dither rounds every pixel to the panel's grays with an 8x8 ordered pattern.
// origin is where the image sits on screen, so the pattern lines up across images.
func dither(img *image.Gray, origin image.Point) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			// Spread 0..255 over 15 steps of 64 sub-levels each.
			t := (int(row[x])*(15*64+1) + 127) / 255
			level := t / 64
			if t%64 >= threshold8x8[(origin.X+x)&7+8*((origin.Y+y)&7)] {
				level++
			}
			row[x] = uint8(min(level*grayStep, 255))
		}
	}
}
