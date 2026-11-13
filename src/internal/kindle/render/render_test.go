package render

import (
	"image"
	"os"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"

	"claudedash/internal/kindle/screen"
	"claudedash/internal/tree"
)

func testFonts(t *testing.T) Fonts {
	t.Helper()
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return Fonts{Regular: regular, Bold: bold}
}

type noAssets struct{}

func (noAssets) Icon(string, int) *image.Gray { return nil }
func (noAssets) Image(string) image.Image     { return nil }

func canvas() *image.Gray {
	return image.NewGray(image.Rect(0, 0, Width, Height))
}

func widget(kind string, r image.Rectangle, root tree.Node) screen.Widget {
	return screen.Widget{Kind: kind, Rect: r, Hash: "h", Tree: root}
}

func state(widgets ...screen.Widget) State {
	return State{Screen: &screen.Screen{Layout: "L", Widgets: widgets}, Now: time.Unix(1794400000, 0)}
}

var barRect = image.Rect(48, 244, 1188, 424)

func barTree() tree.Node {
	return tree.Node{"kind": "col", "children": []any{map[string]any{"kind": "bar", "percent": 50.0, "height": 40.0}}}
}

func TestBarFillsToItsPercent(t *testing.T) {
	img := canvas()
	Draw(img, state(widget("claude", barRect, barTree())), testFonts(t), noAssets{})
	// The bar spans the widget's inner width 72..1164 at y 268..308.
	y := 290
	if img.GrayAt(300, y).Y != 0 {
		t.Errorf("left half should be black, got %d", img.GrayAt(300, y).Y)
	}
	if img.GrayAt(1000, y).Y != 255 {
		t.Errorf("right half should be white, got %d", img.GrayAt(1000, y).Y)
	}
}

func TestDimTextIsNotBlack(t *testing.T) {
	img := canvas()
	root := tree.Node{"kind": "text", "text": "MMMMMMMM", "size": 60.0, "dim": true}
	Draw(img, state(widget("w", image.Rect(48, 48, 610, 228), root)), testFonts(t), noAssets{})
	darkest := uint8(255)
	for y := 48; y < 228; y++ {
		for x := 48; x < 610; x++ {
			darkest = min(darkest, img.GrayAt(x, y).Y)
		}
	}
	if darkest == 0 || darkest > 120 {
		t.Fatalf("dim text darkest pixel %d", darkest)
	}
}

func TestFrostLightensOnlyWidgets(t *testing.T) {
	img := canvas()
	st := state(widget("w", image.Rect(48, 48, 610, 228), tree.Node{"kind": "col"}))
	st.Background = image.NewGray(image.Rect(0, 0, Width, Height))
	st.Frost = 0.5
	Draw(img, st, testFonts(t), noAssets{})
	if got := blockMean(img, image.Rect(100, 100, 132, 132)); got < 120 || got > 135 {
		t.Errorf("inside the widget %v, want about 128", got)
	}
	if got := img.GrayAt(100, 800).Y; got != 0 {
		t.Errorf("outside the widget %d, want the picture's 0", got)
	}
}

func TestButtonsAndCorner(t *testing.T) {
	img := canvas()
	root := tree.Node{"kind": "row", "children": []any{
		map[string]any{"kind": "button", "id": "pp", "icon": "play", "size": 120.0, "action": "media.play-pause", "patch": []any{"p"}},
		map[string]any{"kind": "button", "icon": "next", "size": 120.0, "action": "media.next", "disabled": true},
		map[string]any{"kind": "icon", "name": "note", "size": 40.0},
	}}
	regions, buttons, corner := Draw(img, state(widget("media", image.Rect(48, 1224, 1188, 1600), root)), testFonts(t), noAssets{})
	if len(buttons) != 2 || buttons[0].Action != "media.play-pause" || buttons[0].Kind != "media" || buttons[1].Disabled != true {
		t.Fatalf("%+v", buttons)
	}
	if buttons[0].Rect.Dx() != 120 {
		t.Fatalf("button rect %v", buttons[0].Rect)
	}
	if regions["media"] != image.Rect(48, 1224, 1188, 1600) || regions["all"] != image.Rect(0, 0, Width, Height) {
		t.Fatalf("%v", regions)
	}
	if corner.Exit != image.Rect(1126, 0, 1236, 150) || corner.Refresh != image.Rect(1036, 0, 1126, 150) {
		t.Fatalf("corner %+v", corner)
	}
	if !corner.Switch.Empty() {
		t.Fatal("no switch button unless asked")
	}
	if regions["corner"] != corner.Region || !corner.Exit.In(corner.Region) {
		t.Fatalf("corner region %v", corner.Region)
	}
}

func TestSwitchButton(t *testing.T) {
	st := state()
	st.Switch = "desk-pc"
	st.Offline = true
	st.Battery, st.HasBattery = 76, true
	_, _, corner := Draw(canvas(), st, testFonts(t), noAssets{})
	if corner.Switch.Empty() || corner.Switch.Max.X > corner.Refresh.Min.X || !corner.Switch.In(corner.Region) {
		t.Fatalf("%+v", corner)
	}
}

func centerHasInk(img *image.Gray) bool {
	for y := Height/2 - 100; y < Height/2+100; y++ {
		for x := 100; x < Width-100; x++ {
			if img.GrayAt(x, y).Y < 128 {
				return true
			}
		}
	}
	return false
}

func TestFirstRunAndMessage(t *testing.T) {
	img := canvas()
	Draw(img, State{FirstRun: true, Now: time.Now()}, testFonts(t), noAssets{})
	if !centerHasInk(img) {
		t.Fatal("first run text near the center")
	}
	img = canvas()
	Draw(img, State{Message: "Touch screen not found", Now: time.Now()}, testFonts(t), noAssets{})
	if !centerHasInk(img) {
		t.Fatal("message near the center")
	}
}

func TestRasterizeSVG(t *testing.T) {
	data, err := os.ReadFile("../../widget/icons/play.svg")
	if err != nil {
		t.Fatal(err)
	}
	icon, err := RasterizeSVG(data, 50)
	if err != nil || icon.Bounds() != image.Rect(0, 0, 50, 50) {
		t.Fatalf("%v %v", err, icon)
	}
	if icon.GrayAt(25, 25).Y > 64 || icon.GrayAt(2, 2).Y < 250 {
		t.Fatalf("center %d corner %d", icon.GrayAt(25, 25).Y, icon.GrayAt(2, 2).Y)
	}
	if _, err := RasterizeSVG([]byte("not svg"), 50); err == nil {
		t.Fatal("garbage")
	}
}

func TestMeasureGrowsWithText(t *testing.T) {
	f := testFonts(t)
	if f.Measure("Hello", 30, false) <= f.Measure("Hi", 30, false) || f.Measure("Hi", 60, true) <= f.Measure("Hi", 30, true) {
		t.Fatal("measure")
	}
}

func blockMean(img *image.Gray, r image.Rectangle) float64 {
	sum := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sum += int(img.GrayAt(x, y).Y)
		}
	}
	return float64(sum) / float64(r.Dx()*r.Dy())
}

// The panel shows 16 grays; a smooth picture must be dithered to them, not cut into bands.
func TestBackgroundIsDitheredToThePanelsGrays(t *testing.T) {
	bg := image.NewGray(image.Rect(0, 0, Width, Height))
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			bg.Pix[y*bg.Stride+x] = uint8(x * 255 / (Width - 1))
		}
	}
	img := canvas()
	st := state()
	st.Background = bg
	Draw(img, st, testFonts(t), noAssets{})
	for x := 0; x < Width; x++ {
		if v := img.GrayAt(x, 800).Y; v%17 != 0 {
			t.Fatalf("pixel %d is %d, not one of the 16 grays", x, v)
		}
	}
	for _, x := range []int{100, 400, 640, 1000} {
		r := image.Rect(x, 800, x+32, 832)
		if got, want := blockMean(img, r), blockMean(bg, r); got < want-4 || got > want+4 {
			t.Errorf("around x=%d the shade is %.1f, want about %.1f", x, got, want)
		}
	}
}

type pictureAssets struct{ img image.Image }

func (pictureAssets) Icon(string, int) *image.Gray { return nil }
func (a pictureAssets) Image(string) image.Image   { return a.img }

func TestPictureIsDitheredToThePanelsGrays(t *testing.T) {
	art := image.NewGray(image.Rect(0, 0, 128, 128))
	for i := range art.Pix {
		art.Pix[i] = uint8(i % 128 * 2)
	}
	img := canvas()
	st := state(widget("media", image.Rect(48, 48, 610, 400), tree.Node{"kind": "image", "src": "/art", "w": 128.0, "h": 128.0}))
	Draw(img, st, testFonts(t), pictureAssets{art})
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if v := img.GrayAt(x, y).Y; v != 255 && v%17 != 0 {
				t.Fatalf("pixel (%d,%d) is %d, not one of the 16 grays", x, y, v)
			}
		}
	}
}

// Ordered dithering, as KOReader does, gives a flat shade an even pattern that
// lines up across regions drawn at different times.
func TestFlatShadeGetsARegularPattern(t *testing.T) {
	flat := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range flat.Pix {
		flat.Pix[i] = 128
	}
	dither(flat, image.Point{})
	for y := 0; y < 56; y++ {
		for x := 0; x < 56; x++ {
			if flat.GrayAt(x, y) != flat.GrayAt(x+8, y) || flat.GrayAt(x, y) != flat.GrayAt(x, y+8) {
				t.Fatalf("the pattern repeats every 8 pixels, broken at (%d,%d)", x, y)
			}
		}
	}
}
