package artwork

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func sample(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / width), G: uint8(y * 255 / height), B: 90, A: 255})
		}
	}
	return img
}

func asPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func asJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func process(t *testing.T, raw []byte) (image.Image, string) {
	t.Helper()
	out, hash, err := Process(raw)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
	return img, hash
}

func TestSizes(t *testing.T) {
	cases := []struct {
		name          string
		raw           []byte
		width, height int
	}{
		{"large square jpeg", asJPEG(t, sample(1000, 1000)), 400, 400},
		{"wide png", asPNG(t, sample(800, 400)), 400, 200},
		{"tall png", asPNG(t, sample(300, 900)), 133, 400},
		{"small stays small", asPNG(t, sample(100, 100)), 100, 100},
		{"huge", asJPEG(t, sample(3000, 3000)), 400, 400},
	}
	for _, c := range cases {
		img, _ := process(t, c.raw)
		if b := img.Bounds(); b.Dx() != c.width || b.Dy() != c.height {
			t.Errorf("%s: got %dx%d want %dx%d", c.name, b.Dx(), b.Dy(), c.width, c.height)
		}
	}
}

func TestOutputIsGrayscale(t *testing.T) {
	img, _ := process(t, asPNG(t, sample(200, 200)))
	if img.ColorModel() != color.GrayModel {
		t.Fatalf("color model %v", img.ColorModel())
	}
	left := color.GrayModel.Convert(img.At(5, 100)).(color.Gray).Y
	right := color.GrayModel.Convert(img.At(195, 100)).(color.Gray).Y
	if right <= left {
		t.Fatalf("gradient lost: left %d right %d", left, right)
	}
}

func TestHashIsStableAndContentBased(t *testing.T) {
	a := asPNG(t, sample(200, 200))
	b := asPNG(t, sample(200, 100))
	_, first := process(t, a)
	_, again := process(t, a)
	_, other := process(t, b)
	if len(first) != 12 {
		t.Fatalf("hash %q should be 12 hex characters", first)
	}
	if first != again || first == other {
		t.Fatalf("hashes %q %q %q", first, again, other)
	}
}

func TestBadInput(t *testing.T) {
	for _, raw := range [][]byte{nil, {}, []byte("not an image"), asPNG(t, sample(50, 50))[:40]} {
		if _, _, err := Process(raw); err == nil {
			t.Errorf("%d bytes: want error", len(raw))
		}
	}
}

func TestRejectsAbsurdDimensionsWithoutDecoding(t *testing.T) {
	huge := asPNG(t, image.NewGray(image.Rect(0, 0, 20000, 20000)))
	if _, _, err := Process(huge); err == nil {
		t.Fatal("a 400 megapixel image should be refused")
	}
}
