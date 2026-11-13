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

func TestIconFollowsTheCurrentHash(t *testing.T) {
	s := newAssetStore(t.TempDir())
	full := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000" d="M0 0h24v24H0z"/></svg>`
	corner := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000" d="M0 0h2v2H0z"/></svg>`
	s.store(screen.Asset{Key: "icon:play:h1", Icon: "play"}, []byte(full))
	s.setCurrent(map[string]string{"play": "h1"})
	if img := s.Icon("play", 24); img == nil || img.GrayAt(12, 12).Y > 128 {
		t.Fatal("the first icon is filled")
	}
	s.store(screen.Asset{Key: "icon:play:h2", Icon: "play"}, []byte(corner))
	s.setCurrent(map[string]string{"play": "h2"})
	if img := s.Icon("play", 24); img == nil || img.GrayAt(12, 12).Y < 128 {
		t.Fatal("a new hash must draw the new icon")
	}
}
