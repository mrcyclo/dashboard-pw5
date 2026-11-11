package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

type icoEntry struct {
	width, height int
	img           image.Image
}

func parseICO(t *testing.T, ico []byte) []icoEntry {
	t.Helper()
	if len(ico) < 6 || !bytes.Equal(ico[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("bad ICO header % x", ico[:min(len(ico), 4)])
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	var entries []icoEntry
	for i := 0; i < count; i++ {
		dir := ico[6+16*i : 22+16*i]
		size := binary.LittleEndian.Uint32(dir[8:12])
		offset := binary.LittleEndian.Uint32(dir[12:16])
		if int(offset+size) > len(ico) {
			t.Fatalf("entry %d runs past the end of the file", i)
		}
		img, err := png.Decode(bytes.NewReader(ico[offset : offset+size]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		// A directory byte of 0 means 256.
		width, height := int(dir[0]), int(dir[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		entries = append(entries, icoEntry{width: width, height: height, img: img})
	}
	return entries
}

func TestIconHoldsEverySizeWindowsAsksFor(t *testing.T) {
	entries := parseICO(t, Icon())
	want := []int{16, 20, 24, 32, 40, 48, 64, 256}
	if len(entries) != len(want) {
		t.Fatalf("got %d images", len(entries))
	}
	for i, entry := range entries {
		bounds := entry.img.Bounds()
		if entry.width != want[i] || entry.height != want[i] || bounds.Dx() != want[i] || bounds.Dy() != want[i] {
			t.Errorf("entry %d: directory %dx%d, picture %v, want %d", i, entry.width, entry.height, bounds, want[i])
		}
	}
}

func alpha(img image.Image, x, y int) uint8 {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A
}

func rgb(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func TestIconShape(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(IconPNG(256)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("size %v", b)
	}
	// Rounded corners are transparent, the tile itself is opaque.
	for _, corner := range [][2]int{{0, 0}, {255, 0}, {0, 255}, {255, 255}} {
		if alpha(img, corner[0], corner[1]) != 0 {
			t.Errorf("corner %v should be transparent", corner)
		}
	}
	if alpha(img, 128, 20) != 255 || alpha(img, 20, 128) != 255 {
		t.Fatal("the tile should reach its edges")
	}
	// The tile is dark navy, and the gauge on it has an orange part.
	tile := rgb(img, 128, 18)
	if tile.R > 70 || tile.B < tile.R {
		t.Errorf("tile color %v is not dark navy", tile)
	}
	orange := 0
	for y := 0; y < 256; y += 2 {
		for x := 0; x < 256; x += 2 {
			c := rgb(img, x, y)
			if c.R > 230 && c.G > 120 && c.G < 190 && c.B < 60 {
				orange++
			}
		}
	}
	if orange < 300 {
		t.Fatalf("expected a clearly visible orange gauge, found %d samples", orange)
	}
}

func TestSmallIconStillShowsTheGauge(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(IconPNG(16)))
	if err != nil {
		t.Fatal(err)
	}
	orangeish := 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := rgb(img, x, y)
			if c.R > 180 && c.G > 90 && c.B < 110 && c.R > c.B+80 {
				orangeish++
			}
		}
	}
	if orangeish < 12 {
		t.Fatalf("at tray size the gauge must still read as orange, found %d pixels", orangeish)
	}
}

func TestIconPNGRejectsSillySizes(t *testing.T) {
	for _, size := range []int{0, -4, 5000} {
		if IconPNG(size) != nil {
			t.Errorf("size %d should give nothing", size)
		}
	}
}
