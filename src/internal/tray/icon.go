package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The application icon: a dark tile with a usage gauge, most of it filled in
// orange. It is drawn in code so every size is sharp and no binary asset has
// to be kept in step with the palette.

var (
	tileColor  = color.NRGBA{R: 0x23, G: 0x2f, B: 0x3e, A: 0xff}
	trackColor = color.NRGBA{R: 0x4a, G: 0x5d, B: 0x74, A: 0xff}
	fillColor  = color.NRGBA{R: 0xff, G: 0x99, B: 0x00, A: 0xff}
	dotColor   = color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff}

	// Sizes Windows picks from for the tray, taskbar, title bar and Explorer.
	iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}
)

const (
	maxIconSize = 1024
	// Each pixel is the average of this many samples per side.
	supersample = 4

	tileRadius = 0.22
	gaugeY     = 0.52
	// The gauge opens downward: it starts lower left, runs over the top, ends lower right.
	gaugeStartDegrees = 135.0
	gaugeSweepDegrees = 270.0
	gaugeFilled       = 0.68
)

// gaugeShape is the gauge's radius and stroke as fractions of the icon. Small
// icons get a heavier stroke and no center dot, or the gauge turns to mush.
type gaugeShape struct {
	radius    float64
	thickness float64
	dot       float64
}

func shapeFor(size int) gaugeShape {
	if size <= 24 {
		return gaugeShape{radius: 0.27, thickness: 0.17}
	}
	if size <= 48 {
		return gaugeShape{radius: 0.275, thickness: 0.13, dot: 0.065}
	}
	return gaugeShape{radius: 0.28, thickness: 0.105, dot: 0.06}
}

// IconPNG returns the icon as a square PNG, or nil for a size that makes no sense.
func IconPNG(size int) []byte {
	if size < 1 || size > maxIconSize {
		return nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, render(size)); err != nil {
		return nil
	}
	return out.Bytes()
}

// Icon returns a Windows .ico holding the icon at every size in iconSizes.
func Icon() []byte {
	payloads := make([][]byte, len(iconSizes))
	for i, size := range iconSizes {
		payloads[i] = IconPNG(size)
	}

	// ICO container: a 6-byte ICONDIR, one 16-byte ICONDIRENTRY per image, then the PNG payloads.
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, uint16(len(iconSizes))})
	offset := 6 + 16*len(iconSizes)
	for i, size := range iconSizes {
		// One byte per dimension, where 0 stands for 256.
		side := byte(size)
		ico.Write([]byte{side, side, 0, 0})
		binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&ico, binary.LittleEndian, []uint32{uint32(len(payloads[i])), uint32(offset)})
		offset += len(payloads[i])
	}
	for _, payload := range payloads {
		ico.Write(payload)
	}
	return ico.Bytes()
}

func render(size int) *image.NRGBA {
	shape := shapeFor(size)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	samples := float64(supersample * supersample)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					u := (float64(x) + (float64(sx)+0.5)/supersample) / float64(size)
					v := (float64(y) + (float64(sy)+0.5)/supersample) / float64(size)
					c, ok := colorAt(u, v, shape)
					if !ok {
						continue
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a++
				}
			}
			if a == 0 {
				continue
			}
			// Colors are averaged over the covered samples only, so edges do not darken.
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(r / a)),
				G: uint8(math.Round(g / a)),
				B: uint8(math.Round(b / a)),
				A: uint8(math.Round(255 * a / samples)),
			})
		}
	}
	return img
}

// colorAt gives the icon's color at a point of the unit square, or false outside the tile.
func colorAt(u, v float64, shape gaugeShape) (color.NRGBA, bool) {
	if !insideRoundedSquare(u, v, tileRadius) {
		return color.NRGBA{}, false
	}
	dx, dy := u-0.5, v-gaugeY
	distance := math.Hypot(dx, dy)
	if shape.dot > 0 && distance <= shape.dot {
		return dotColor, true
	}

	half := shape.thickness / 2
	// Degrees along the gauge from its start, clockwise as seen on screen.
	along := math.Mod(math.Atan2(dy, dx)*180/math.Pi-gaugeStartDegrees+720, 360)
	onRing := math.Abs(distance-shape.radius) <= half
	filledSweep := gaugeSweepDegrees * gaugeFilled

	if (onRing && along <= filledSweep) || nearGaugePoint(dx, dy, shape, 0) || nearGaugePoint(dx, dy, shape, filledSweep) {
		return fillColor, true
	}
	if (onRing && along <= gaugeSweepDegrees) || nearGaugePoint(dx, dy, shape, gaugeSweepDegrees) {
		return trackColor, true
	}
	return tileColor, true
}

// nearGaugePoint rounds the ends of the gauge: it is true within half a stroke
// of the point that lies the given number of degrees along it.
func nearGaugePoint(dx, dy float64, shape gaugeShape, along float64) bool {
	angle := (gaugeStartDegrees + along) * math.Pi / 180
	px, py := shape.radius*math.Cos(angle), shape.radius*math.Sin(angle)
	return math.Hypot(dx-px, dy-py) <= shape.thickness/2
}

func insideRoundedSquare(u, v, radius float64) bool {
	if u < 0 || u > 1 || v < 0 || v > 1 {
		return false
	}
	// Distance from the nearest corner circle's center, only where a corner applies.
	cx := math.Max(radius-u, u-(1-radius))
	cy := math.Max(radius-v, v-(1-radius))
	if cx <= 0 || cy <= 0 {
		return true
	}
	return math.Hypot(cx, cy) <= radius
}
