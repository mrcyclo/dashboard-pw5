package render

import (
	"bytes"
	"errors"
	"image"
	"image/color"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

var errNotAnIcon = errors.New("render: SVG has nothing to draw")

// RasterizeSVG draws an icon on white at size x size pixels.
func RasterizeSVG(svg []byte, size int) (*image.Gray, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg), oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}
	if len(icon.SVGPaths) == 0 {
		return nil, errNotAnIcon
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	rgba := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(size, size, scanner), 1)

	out := image.NewGray(rgba.Bounds())
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := rgba.RGBAAt(x, y)
			// Composite over white: premultiplied color plus the white that shows through.
			lum := (299*uint32(c.R) + 587*uint32(c.G) + 114*uint32(c.B)) / 1000
			out.SetGray(x, y, color.Gray{Y: uint8(lum + 255 - uint32(c.A))})
		}
	}
	return out, nil
}
