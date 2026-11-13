package app

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"claudedash/internal/kindle/render"
	"claudedash/internal/kindle/screen"
)

// Seconds to wait before asking again for an asset that failed, by number of failures.
var retryDelays = []time.Duration{60 * time.Second, 300 * time.Second, 900 * time.Second}

type failure struct {
	count   int
	retryAt time.Time
}

// assetStore holds downloaded icons (kept on disk) and pictures (in memory),
// and implements render.Assets.
type assetStore struct {
	dir     string
	svgs    map[string][]byte
	sized   map[string]*image.Gray
	images  map[string]image.Image
	loading map[string]bool
	failed  map[string]failure
}

func newAssetStore(dir string) *assetStore {
	return &assetStore{
		dir:     dir,
		svgs:    map[string][]byte{},
		sized:   map[string]*image.Gray{},
		images:  map[string]image.Image{},
		loading: map[string]bool{},
		failed:  map[string]failure{},
	}
}

func (s *assetStore) iconPath(name, hash string) string {
	return filepath.Join(s.dir, name+"-"+hash+".svg")
}

// loadIcons reads icons saved in an earlier session.
func (s *assetStore) loadIcons(icons map[string]string) {
	for name, hash := range icons {
		if data, err := os.ReadFile(s.iconPath(name, hash)); err == nil {
			s.svgs[name+":"+hash] = data
		}
	}
}

func (s *assetStore) have(a screen.Asset) bool {
	if a.Icon != "" {
		_, ok := s.svgs[strings.TrimPrefix(a.Key, "icon:")]
		return ok
	}
	_, ok := s.images[a.Key]
	return ok
}

// wants reports whether an asset should be fetched now.
func (s *assetStore) wants(a screen.Asset, now time.Time) bool {
	if s.have(a) || s.loading[a.Key] {
		return false
	}
	f, failed := s.failed[a.Key]
	return !failed || !now.Before(f.retryAt)
}

func (s *assetStore) fail(key string, now time.Time) {
	f := s.failed[key]
	f.count++
	f.retryAt = now.Add(retryDelays[min(f.count, len(retryDelays))-1])
	s.failed[key] = f
}

func (s *assetStore) store(a screen.Asset, data []byte) bool {
	if a.Icon != "" {
		if _, err := render.RasterizeSVG(data, 8); err != nil {
			return false
		}
		key := strings.TrimPrefix(a.Key, "icon:")
		s.svgs[key] = data
		_, hash, _ := strings.Cut(key, ":")
		os.WriteFile(s.iconPath(a.Icon, hash), data, 0o644)
		return true
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	s.images[a.Key] = shrink(img)
	delete(s.failed, a.Key)
	return true
}

// maxPicture is the longest side a picture is kept at; covers are drawn at 130 px or less.
const maxPicture = 400

// shrink keeps a decoded picture small: a full-size cover is several megabytes.
func shrink(img image.Image) image.Image {
	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	if longest <= maxPicture {
		return img
	}
	w, h := b.Dx()*maxPicture/longest, b.Dy()*maxPicture/longest
	out := image.NewGray(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.ApproxBiLinear.Scale(out, out.Bounds(), img, b, xdraw.Src, nil)
	return out
}

// keepOnly forgets pictures and failures the screen no longer lists, so a day
// of changing covers does not fill the Kindle's memory.
func (s *assetStore) keepOnly(assets []screen.Asset) {
	wanted := map[string]bool{}
	for _, a := range assets {
		wanted[a.Key] = true
	}
	for key := range s.images {
		if !wanted[key] {
			delete(s.images, key)
		}
	}
	for key := range s.failed {
		if !wanted[key] {
			delete(s.failed, key)
		}
	}
}

// removeStale deletes icon files the current icon list no longer names.
func (s *assetStore) removeStale(icons map[string]string) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	for _, f := range screen.StaleIcons(files, icons) {
		os.Remove(filepath.Join(s.dir, f))
	}
	for key := range s.svgs {
		name, hash, _ := strings.Cut(key, ":")
		if icons[name] != hash {
			delete(s.svgs, key)
		}
	}
	for key := range s.sized {
		name, _, _ := strings.Cut(key, "@")
		if _, ok := icons[name]; !ok {
			delete(s.sized, key)
		}
	}
}

// Icon draws a downloaded icon at a size, or one bundled for the corner.
func (s *assetStore) Icon(name string, size int) *image.Gray {
	key := name + "@" + strconv.Itoa(size)
	if img, ok := s.sized[key]; ok {
		return img
	}
	var data []byte
	for k, v := range s.svgs {
		if strings.HasPrefix(k, name+":") {
			data = v
		}
	}
	if data == nil {
		data = bundled[name]
	}
	if data == nil {
		return nil
	}
	img, err := render.RasterizeSVG(data, size)
	if err != nil {
		return nil
	}
	s.sized[key] = img
	return img
}

func (s *assetStore) Image(key string) image.Image {
	return s.images[key]
}

// The corner's icons ship with the app, so exit is drawn even with the PC never reached.
var bundled = map[string][]byte{
	"close":   []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"><path fill="#000" d="M5.6 3.5 12 9.9l6.4-6.4 2.1 2.1-6.4 6.4 6.4 6.4-2.1 2.1-6.4-6.4-6.4 6.4-2.1-2.1 6.4-6.4-6.4-6.4z"/></svg>`),
	"refresh": refreshSVG,
}

var refreshSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"><path fill="none" stroke="#000" stroke-width="2.4" stroke-linecap="round" d="M19.2 12a7.2 7.2 0 1 1-2.1-5.1"/><path fill="#000" d="M20.3 2.6v6.6h-6.6z"/></svg>`)
