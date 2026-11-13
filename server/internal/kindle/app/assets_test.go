package app

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"

	"claudedash/internal/kindle/screen"
)

func pngBytes(w, h int) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h)))
	return buf.Bytes()
}

func TestPicturesLeavingTheScreenAreDropped(t *testing.T) {
	s := newAssetStore(t.TempDir())
	old := screen.Asset{Key: "image:/media/art.png?v=1", Path: "/media/art.png?v=1"}
	if !s.store(old, pngBytes(20, 20)) {
		t.Fatal("store")
	}
	s.fail("image:/media/art.png?v=0", time.Now())
	s.keepOnly([]screen.Asset{{Key: "image:/media/art.png?v=2", Path: "/media/art.png?v=2"}})
	if s.Image(old.Key) != nil || len(s.failed) != 0 {
		t.Fatalf("old pictures and failures must go: %d images, %d failures", len(s.images), len(s.failed))
	}
}

func TestLargePicturesAreShrunk(t *testing.T) {
	s := newAssetStore(t.TempDir())
	a := screen.Asset{Key: "image:/big", Path: "/big"}
	s.store(a, pngBytes(1200, 800))
	b := s.Image(a.Key).Bounds()
	if b.Dx() > maxPicture || b.Dy() > maxPicture || b.Dx() != maxPicture {
		t.Fatalf("stored %v", b)
	}
}
