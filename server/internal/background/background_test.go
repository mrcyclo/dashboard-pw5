package background

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Left half black, right half white, so a crop can be told from a squash.
func writePicture(t *testing.T, dir, name string, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			shade := uint8(0)
			if x >= width/2 {
				shade = 255
			}
			img.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	var buf bytes.Buffer
	var err error
	if filepath.Ext(name) == ".png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newStore(t *testing.T) (*Store, string) {
	return New("", 0.5, 0.5), t.TempDir()
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestNoBackgroundByDefault(t *testing.T) {
	s, _ := newStore(t)
	if s.Hash() != "" {
		t.Fatalf("hash %q", s.Hash())
	}
	if _, ok := s.Render(1236, 1648); ok {
		t.Fatal("nothing should render")
	}
}

func TestRenderCoversTheRequestedSize(t *testing.T) {
	s, dir := newStore(t)
	for _, name := range []string{"wide.jpg", "tall.png", "small.png"} {
		sizes := map[string][2]int{"wide.jpg": {2000, 1000}, "tall.png": {600, 2400}, "small.png": {200, 100}}
		if err := s.Set(writePicture(t, dir, name, sizes[name][0], sizes[name][1])); err != nil {
			t.Fatal(err)
		}
		data, ok := s.Render(1236, 1648)
		if !ok {
			t.Fatalf("%s: no render", name)
		}
		img := decode(t, data)
		if b := img.Bounds(); b.Dx() != 1236 || b.Dy() != 1648 {
			t.Fatalf("%s: got %dx%d", name, b.Dx(), b.Dy())
		}
		if img.ColorModel() != color.GrayModel {
			t.Fatalf("%s: not grayscale", name)
		}
	}
}

func TestRenderCropsTheCenterInsteadOfSquashing(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "wide.png", 2000, 1000))
	data, _ := s.Render(1000, 1000)
	img := decode(t, data)
	gray := func(x int) uint8 { return color.GrayModel.Convert(img.At(x, 500)).(color.Gray).Y }
	// The middle 1000 columns of the source are kept, so the split stays in the middle.
	if gray(250) > 40 || gray(750) < 215 {
		t.Fatalf("left %d right %d", gray(250), gray(750))
	}
	if gray(480) > 40 || gray(520) < 215 {
		t.Fatalf("split moved: %d %d, the picture was squashed rather than cropped", gray(480), gray(520))
	}
}

func TestHashChangesWithThePicture(t *testing.T) {
	s, dir := newStore(t)
	first := writePicture(t, dir, "a.png", 300, 400)
	s.Set(first)
	a := s.Hash()
	if len(a) != 12 {
		t.Fatalf("hash %q", a)
	}
	if again := s.Hash(); again != a {
		t.Fatal("hash must be stable")
	}
	s.Set(writePicture(t, dir, "b.png", 300, 500))
	b := s.Hash()
	if b == a {
		t.Fatal("a different picture needs a different hash")
	}
	// Same path, new content.
	s.Set(first)
	writePicture(t, dir, "a.png", 320, 400)
	os.Chtimes(first, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	if s.Hash() == a {
		t.Fatal("editing the file in place must change the hash")
	}
}

func TestClear(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	s.Clear()
	if s.Hash() != "" {
		t.Fatal("hash should be empty after clearing")
	}
	if _, ok := s.Render(100, 100); ok {
		t.Fatal("nothing should render after clearing")
	}
}

func TestNewRestoresAChoice(t *testing.T) {
	dir := t.TempDir()
	path := writePicture(t, dir, "grünes bild.png", 300, 400)
	s := New(path, 0.25, 0.75)
	if s.Hash() == "" || s.Path() != path {
		t.Fatalf("path %q hash %q", s.Path(), s.Hash())
	}
	if x, y := s.Focus(); x != 0.25 || y != 0.75 {
		t.Fatalf("focus %v %v", x, y)
	}
}

func TestNewWithMissingFileHasNoBackground(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "gone.png"), 0.5, 0.5)
	if s.Hash() != "" {
		t.Fatal("a missing file is not a background")
	}
}

// The picture is black on its left half and white on its right half, so the
// average brightness of a crop tells which part of the picture was kept.
func brightness(t *testing.T, data []byte) float64 {
	t.Helper()
	img := decode(t, data)
	b := img.Bounds()
	total := 0
	for x := b.Min.X; x < b.Max.X; x += 4 {
		total += int(color.GrayModel.Convert(img.At(x, b.Min.Y+b.Dy()/2)).(color.Gray).Y)
	}
	return float64(total) / float64(b.Dx()/4) / 255
}

func TestFocusPicksWhichPartIsKept(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "wide.png", 2000, 1000))
	render := func(x, y float64) float64 {
		s.SetFocus(x, y)
		data, ok := s.Render(1000, 1000)
		if !ok {
			t.Fatal("no render")
		}
		if b := decode(t, data).Bounds(); b.Dx() != 1000 || b.Dy() != 1000 {
			t.Fatalf("size %v", b)
		}
		return brightness(t, data)
	}
	if left := render(0, 0.5); left > 0.05 {
		t.Errorf("focus at the left edge should show only the dark half, got %.2f", left)
	}
	if right := render(1, 0.5); right < 0.95 {
		t.Errorf("focus at the right edge should show only the light half, got %.2f", right)
	}
	if mid := render(0.5, 0.5); mid < 0.4 || mid > 0.6 {
		t.Errorf("centered focus should straddle the split, got %.2f", mid)
	}
}

func TestFocusOutsideRangeIsClamped(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "wide.png", 2000, 1000))
	s.SetFocus(-3, 9)
	if x, y := s.Focus(); x != 0 || y != 1 {
		t.Fatalf("focus %v %v", x, y)
	}
	data, ok := s.Render(1000, 1000)
	if !ok || brightness(t, data) > 0.05 {
		t.Fatal("a clamped focus must still fill the frame from inside the picture")
	}
}

func TestHashChangesWithFocus(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	before := s.Hash()
	s.SetFocus(0.2, 0.5)
	moved := s.Hash()
	if moved == before {
		t.Fatal("moving the focus must change the hash, or the Kindle keeps the old crop")
	}
	s.SetFocus(0.2, 0.5)
	if s.Hash() != moved {
		t.Fatal("the same focus must give the same hash")
	}
}

func TestSetKeepsFocus(t *testing.T) {
	s, dir := newStore(t)
	s.SetFocus(0.1, 0.9)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	if x, y := s.Focus(); x != 0.1 || y != 0.9 {
		t.Fatalf("focus %v %v", x, y)
	}
	s.Clear()
	if x, y := s.Focus(); x != 0.5 || y != 0.5 {
		t.Fatalf("clearing should recentre the focus, got %v %v", x, y)
	}
}

func TestSetRejectsWhatCannotBeShown(t *testing.T) {
	s, dir := newStore(t)
	good := writePicture(t, dir, "good.png", 300, 400)
	s.Set(good)
	notPicture := filepath.Join(dir, "notes.txt")
	os.WriteFile(notPicture, []byte("hello"), 0o600)
	for _, bad := range []string{notPicture, filepath.Join(dir, "missing.png"), dir} {
		if err := s.Set(bad); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
	if s.Path() != good {
		t.Fatalf("a rejected choice must keep the previous picture, got %q", s.Path())
	}
}

func TestDeletedPictureMeansNoBackground(t *testing.T) {
	s, dir := newStore(t)
	path := writePicture(t, dir, "a.png", 300, 400)
	s.Set(path)
	os.Remove(path)
	if s.Hash() != "" {
		t.Fatal("a deleted picture should report no background")
	}
	if _, ok := s.Render(100, 100); ok {
		t.Fatal("a deleted picture should not render")
	}
}

func TestRenderRefusesAbsurdSizes(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	for _, size := range [][2]int{{0, 100}, {100, -1}, {100000, 100}, {100, 100000}} {
		if _, ok := s.Render(size[0], size[1]); ok {
			t.Errorf("%v: should be refused", size)
		}
	}
}

func TestRenderIsCached(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	first, _ := s.Render(120, 160)
	second, _ := s.Render(120, 160)
	if &first[0] != &second[0] {
		t.Fatal("an unchanged picture at the same size should not be rendered twice")
	}
}

func TestSourceIsTheWholePictureScaledDown(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "wide.png", 2000, 1000))
	s.SetFocus(0, 0)
	data, ok := s.Source(800)
	if !ok {
		t.Fatal("no source")
	}
	img := decode(t, data)
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 400 {
		t.Fatalf("got %v, want the full 2:1 picture at 800 wide", b)
	}
	if img.ColorModel() != color.GrayModel {
		t.Fatal("not grayscale")
	}
	// Uncropped: both the dark left half and the light right half are present, whatever the focus.
	if got := brightness(t, data); got < 0.4 || got > 0.6 {
		t.Fatalf("brightness %.2f, the focus must not crop the source", got)
	}
}

func TestSourceDoesNotEnlargeSmallPictures(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "small.png", 200, 100))
	data, _ := s.Source(800)
	if b := decode(t, data).Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Fatalf("got %v", b)
	}
}

func TestSourceWithoutPictureOrWithBadSize(t *testing.T) {
	s, dir := newStore(t)
	if _, ok := s.Source(800); ok {
		t.Fatal("nothing to show")
	}
	s.Set(writePicture(t, dir, "a.png", 200, 100))
	for _, size := range []int{0, -1, 50000} {
		if _, ok := s.Source(size); ok {
			t.Errorf("size %d should be refused", size)
		}
	}
	s.Clear()
	if _, ok := s.Source(800); ok {
		t.Fatal("cleared picture still served")
	}
}

func TestCoverRectZoom(t *testing.T) {
	source := image.Rect(0, 0, 2000, 1000)
	cases := []struct {
		focusX, focusY, zoom float64
		want                 image.Rectangle
	}{
		// Not zoomed: the largest square, slid by the focus.
		{0.5, 0.5, 1, image.Rect(500, 0, 1500, 1000)},
		{0, 0.5, 1, image.Rect(0, 0, 1000, 1000)},
		// Zoomed in twice: half the size, and now there is room to move vertically too.
		{0.5, 0.5, 2, image.Rect(750, 250, 1250, 750)},
		{0, 0, 2, image.Rect(0, 0, 500, 500)},
		{1, 1, 2, image.Rect(1500, 500, 2000, 1000)},
		{0.5, 0.5, 4, image.Rect(875, 375, 1125, 625)},
	}
	for _, c := range cases {
		if got := coverRect(source, 1000, 1000, c.focusX, c.focusY, c.zoom); got != c.want {
			t.Errorf("focus (%v, %v) zoom %v: got %v want %v", c.focusX, c.focusY, c.zoom, got, c.want)
		}
	}
}

func TestCoverRectNeverLeavesThePicture(t *testing.T) {
	source := image.Rect(10, 20, 310, 220)
	for _, zoom := range []float64{1, 1.3, 2.7, 5} {
		for _, focus := range []float64{0, 0.33, 1} {
			got := coverRect(source, 1236, 1648, focus, 1-focus, zoom)
			if !got.In(source) || got.Dx() < 1 || got.Dy() < 1 {
				t.Errorf("zoom %v focus %v: %v is outside %v", zoom, focus, got, source)
			}
		}
	}
}

func TestZoomShowsLessOfThePicture(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "wide.png", 2000, 1000))
	// Centered and not zoomed, the frame straddles the dark and light halves.
	s.SetView(0.4, 0.5, 1)
	plain, _ := s.Render(1000, 1000)
	if got := brightness(t, plain); got < 0.2 || got > 0.6 {
		t.Fatalf("unzoomed brightness %.2f", got)
	}
	// Zoomed in at the same focus, only the dark half is left in view.
	s.SetView(0.4, 0.5, 4)
	zoomed, ok := s.Render(1000, 1000)
	if !ok {
		t.Fatal("no render")
	}
	if b := decode(t, zoomed).Bounds(); b.Dx() != 1000 || b.Dy() != 1000 {
		t.Fatalf("size %v", b)
	}
	if got := brightness(t, zoomed); got > 0.05 {
		t.Fatalf("zoomed brightness %.2f, expected only the dark half", got)
	}
}

func TestZoomIsClampedAndChangesHash(t *testing.T) {
	s, dir := newStore(t)
	s.Set(writePicture(t, dir, "a.png", 300, 400))
	if _, _, zoom := s.View(); zoom != 1 {
		t.Fatalf("a new store is not zoomed, got %v", zoom)
	}
	before := s.Hash()
	s.SetView(0.5, 0.5, 2)
	if s.Hash() == before {
		t.Fatal("zooming must change the hash, or the Kindle keeps the old crop")
	}
	source := s.FileHash()
	s.SetView(0.5, 0.5, 9)
	if _, _, zoom := s.View(); zoom != 5 {
		t.Fatalf("zoom %v should be clamped to 5", zoom)
	}
	s.SetView(0.5, 0.5, 0.2)
	if _, _, zoom := s.View(); zoom != 1 {
		t.Fatalf("zoom %v should be clamped to 1", zoom)
	}
	if s.FileHash() != source {
		t.Fatal("the file hash must not depend on zoom")
	}
	s.SetView(0.5, 0.5, 3)
	s.Clear()
	if _, _, zoom := s.View(); zoom != 1 {
		t.Fatalf("clearing should reset zoom, got %v", zoom)
	}
}
