// Package render draws the dashboard into an 8-bit gray image of the whole
// Kindle screen, the way claudedash.koplugin/cd_view.lua drew it in KOReader.
package render

import (
	"image"
	"image/color"
	"math"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"claudedash/internal/kindle/screen"
	"claudedash/internal/tree"
)

// The Paperwhite 5 screen in portrait, the reference size of every widget tree.
const (
	Width  = 1236
	Height = 1648
)

const (
	padding    = 24
	cornerH    = 150
	exitW      = 110
	refreshW   = 90
	cornerText = 35
	cornerIcon = 45
)

var (
	black = color.Gray{Y: 0}
	white = color.Gray{Y: 255}
	dim   = color.Gray{Y: 85}
	faded = color.Gray{Y: 153}
)

// State is everything one frame shows.
type State struct {
	// Screen is nil before the PC was ever reached.
	Screen *screen.Screen
	// Shown holds each widget kind's tree with tap predictions applied.
	Shown      map[string]tree.Node
	Background *image.Gray
	// Frost is how strongly widget panels lighten the background, 0 to 1.
	Frost      float64
	Battery    int
	HasBattery bool
	Offline    bool
	WifiOff    bool
	// Switch is the name of another PC the user can move to, or "".
	Switch   string
	FirstRun bool
	// Hint replaces the first-run screen's second line, such as why a found PC cannot be reached.
	Hint string
	// Message replaces everything with one centered line, such as a fatal error.
	Message string
	Now     time.Time
}

// Assets gives downloaded icons and pictures, or nil while they load.
type Assets interface {
	Icon(name string, size int) *image.Gray
	Image(key string) image.Image
}

type Button struct {
	Rect     image.Rectangle
	Kind     string
	Action   string
	Patch    any
	Disabled bool
}

// Corner holds the tap zones and the repaint region of the top right controls.
type Corner struct {
	Exit, Refresh, Switch image.Rectangle
	Region                image.Rectangle
}

type painter struct {
	dst    *image.Gray
	fonts  Fonts
	assets Assets
}

// Draw paints a frame and returns repaint regions by name ("all", "corner" and
// each widget kind), the widget buttons, and the corner controls.
func Draw(dst *image.Gray, st State, f Fonts, a Assets) (map[string]image.Rectangle, []Button, Corner) {
	p := painter{dst: dst, fonts: f, assets: a}
	full := image.Rect(0, 0, Width, Height)
	regions := map[string]image.Rectangle{"all": full}
	p.fill(full, white)
	if st.Message != "" {
		p.centered(st.Message, Height/2, 45, true)
		return regions, nil, Corner{}
	}
	if st.Background != nil {
		xdraw.Draw(dst, full, st.Background, image.Point{}, xdraw.Src)
	}

	var buttons []Button
	if st.Screen == nil {
		p.centered("Looking for Claude Dashboard on your network", Height/2-40, 45, true)
		second := "Open Claude Dashboard on your PC"
		if st.Hint != "" {
			second = st.Hint
		}
		p.centered(second, Height/2+40, 35, false)
	} else {
		for _, w := range st.Screen.Widgets {
			regions[w.Kind] = w.Rect
			if st.Background != nil {
				p.lighten(w.Rect, st.Frost)
			}
			root := w.Tree
			if shown, ok := st.Shown[w.Kind]; ok {
				root = shown
			}
			for _, b := range tree.Layout(root, w.Rect.Inset(padding), f.Measure, st.Now) {
				p.box(b)
				action, _ := b.Node["action"].(string)
				if b.Node["kind"] == "button" && action != "" {
					buttons = append(buttons, Button{
						Rect:     image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H),
						Kind:     w.Kind,
						Action:   action,
						Patch:    b.Node["patch"],
						Disabled: b.Node["disabled"] == true,
					})
				}
			}
		}
	}
	corner := p.corner(st)
	regions["corner"] = corner.Region
	return regions, buttons, corner
}

func (p painter) fill(r image.Rectangle, c color.Gray) {
	r = r.Intersect(p.dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p.dst.SetGray(x, y, c)
		}
	}
}

func (p painter) lighten(r image.Rectangle, amount float64) {
	r = r.Intersect(p.dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			v := float64(p.dst.GrayAt(x, y).Y)
			p.dst.SetGray(x, y, color.Gray{Y: uint8(math.Round(v + (255-v)*amount))})
		}
	}
}

// text draws one line left aligned in r, vertically centered, cut to fit.
func (p painter) text(r image.Rectangle, s string, size int, bold bool, c color.Gray) {
	face := p.fonts.face(size, bold)
	if face == nil || s == "" {
		return
	}
	s = p.fonts.fit(s, size, bold, r.Dx())
	metrics := face.Metrics()
	ascent, descent := metrics.Ascent.Ceil(), metrics.Descent.Ceil()
	baseline := r.Min.Y + (r.Dy()-ascent-descent)/2 + ascent
	d := font.Drawer{Dst: p.dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(r.Min.X, baseline)}
	d.DrawString(s)
}

func (p painter) centered(s string, y, size int, bold bool) {
	width := p.fonts.Measure(s, size, bold)
	x := max((Width-width)/2, 16)
	p.text(image.Rect(x, y-size, min(x+width+2, Width-16), y+size), s, size, bold, black)
}

func number(v any, fallback float64) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return fallback
}

func (p painter) box(b tree.Box) {
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H)
	n := b.Node
	kind, _ := n["kind"].(string)
	switch {
	case b.Textual:
		c := black
		if n["dim"] == true {
			c = dim
		}
		p.text(r, b.Text, int(number(n["size"], 20)), n["bold"] == true, c)
	case kind == "bar":
		p.bar(r, number(n["percent"], 0))
	case kind == "rule":
		p.fill(r, dim)
	case kind == "icon":
		name, _ := n["name"].(string)
		p.icon(r, name, min(r.Dx(), r.Dy()), n["dim"] == true)
	case kind == "image":
		src, _ := n["src"].(string)
		p.image(r, "image:"+src)
	case kind == "button":
		p.button(r, n)
	}
}

func (p painter) bar(r image.Rectangle, percent float64) {
	p.fill(r, black)
	inner := r.Inset(2)
	p.fill(inner, white)
	filled := int(float64(inner.Dx()) * math.Min(math.Max(percent, 0), 100) / 100)
	p.fill(image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+filled, inner.Max.Y), black)
}

// icon draws a downloaded icon centered in r, darkening what is under it.
func (p painter) icon(r image.Rectangle, name string, side int, faint bool) {
	if p.assets == nil || side < 1 {
		return
	}
	img := p.assets.Icon(name, side)
	if img == nil {
		return
	}
	origin := image.Pt(r.Min.X+(r.Dx()-side)/2, r.Min.Y+(r.Dy()-side)/2)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			v := img.GrayAt(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).Y
			if faint {
				v = uint8(255 - float64(255-v)*0.4)
			}
			px, py := origin.X+x, origin.Y+y
			if image.Pt(px, py).In(p.dst.Bounds()) && v < p.dst.GrayAt(px, py).Y {
				p.dst.SetGray(px, py, color.Gray{Y: v})
			}
		}
	}
}

func (p painter) image(r image.Rectangle, key string) {
	var img image.Image
	if p.assets != nil {
		img = p.assets.Image(key)
	}
	if img == nil {
		p.fill(r, black)
		p.fill(r.Inset(2), color.Gray{Y: 204})
		return
	}
	xdraw.ApproxBiLinear.Scale(p.dst, r, img, img.Bounds(), xdraw.Src, nil)
}

// button is a white disc with a ring and the icon, or the icon alone when plain.
func (p painter) button(r image.Rectangle, n tree.Node) {
	side := min(r.Dx(), r.Dy())
	disabled := n["disabled"] == true
	icon, _ := n["icon"].(string)
	if n["plain"] == true {
		p.icon(r, icon, side*63/100, disabled)
		return
	}
	ring := black
	if disabled {
		ring = faded
	}
	cx, cy := float64(r.Min.X)+float64(r.Dx())/2, float64(r.Min.Y)+float64(r.Dy())/2
	radius := float64(side) / 2
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			switch {
			case d > radius:
			case d > radius-4:
				p.dst.SetGray(x, y, ring)
			default:
				p.dst.SetGray(x, y, white)
			}
		}
	}
	p.icon(r, icon, side*42/100, disabled)
}

func (p painter) corner(st State) Corner {
	exit := image.Rect(Width-exitW, 0, Width, cornerH)
	refresh := image.Rect(exit.Min.X-refreshW, 0, exit.Min.X, cornerH)
	p.icon(exit, "close", cornerIcon, false)
	p.icon(refresh, "refresh", cornerIcon, false)

	left := refresh.Min.X
	put := func(s string, size int, bold bool, gap int) {
		width := p.fonts.Measure(s, size, bold)
		r := image.Rect(left-gap-width, 0, left-gap, cornerH)
		p.text(r, s, size, bold, black)
		left = r.Min.X
	}
	if st.HasBattery {
		put(itoa(st.Battery)+"%", cornerText, false, 8)
	}
	switch {
	case st.WifiOff:
		put("Wi-Fi is off", cornerText, true, 24)
	case st.Offline:
		put("PC offline", cornerText, true, 24)
	}
	var switchRect image.Rectangle
	if st.Switch != "" {
		label := "Use " + st.Switch
		width := p.fonts.Measure(label, 30, true) + 32
		switchRect = image.Rect(left-24-width, 40, left-24, 110)
		p.fill(switchRect, black)
		p.fill(switchRect.Inset(3), white)
		p.text(switchRect.Inset(16), label, 30, true, black)
		left = switchRect.Min.X
	}
	return Corner{Exit: exit, Refresh: refresh, Switch: switchRect, Region: image.Rect(max(left-8, 0), 0, Width, cornerH)}
}

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return itoa(v/10) + string(rune('0'+v%10))
}
