package render

import (
	"fmt"
	"image"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// The e-ink panel shows 16 grays, 0, 17, 34 ... 255.
const grayStep = 17

type backdropKey struct {
	background *image.Gray
	frost      float64
	panels     string
}

// The backdrop only changes with the picture, the frost or the layout, and
// dithering a full screen is slow on the Kindle, so the last one is kept.
var lastBackdrop struct {
	sync.Mutex
	key backdropKey
	img *image.Gray
}

// backdrop is the background with the widget panels lightened, dithered to the
// panel's grays so smooth shading does not break into bands.
func backdrop(background *image.Gray, frost float64, panels []image.Rectangle) *image.Gray {
	key := backdropKey{background, frost, fmt.Sprint(panels)}
	lastBackdrop.Lock()
	defer lastBackdrop.Unlock()
	if lastBackdrop.img != nil && lastBackdrop.key == key {
		return lastBackdrop.img
	}
	img := image.NewGray(image.Rect(0, 0, Width, Height))
	xdraw.Draw(img, img.Bounds(), background, image.Point{}, xdraw.Src)
	p := painter{dst: img}
	for _, r := range panels {
		p.lighten(r, frost)
	}
	dither(img)
	lastBackdrop.key, lastBackdrop.img = key, img
	return img
}

// dither rounds every pixel to the panel's grays with Floyd-Steinberg error diffusion.
func dither(img *image.Gray) {
	b := img.Bounds()
	w := b.Dx()
	// Errors in sixteenths, with a spare column on each side.
	cur := make([]int, w+2)
	next := make([]int, w+2)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < w; x++ {
			v := int(row[x]) + cur[x+1]/16
			q := min(max((v+grayStep/2)/grayStep, 0), 255/grayStep) * grayStep
			row[x] = uint8(q)
			e := v - q
			cur[x+2] += e * 7
			next[x] += e * 3
			next[x+1] += e * 5
			next[x+2] += e
		}
		cur, next = next, cur
		clear(next)
	}
}
