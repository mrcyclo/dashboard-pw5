// Package background keeps the picture the user chose as the dashboard
// backdrop and renders it for a given screen.
package background

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"sync"

	"golang.org/x/image/draw"
)

const (
	maxSide = 4096
	// Phone photos reach about 50 megapixels; anything far beyond is not a photo.
	maxSourcePixels = 120_000_000

	centered = 0.5

	// Zoom 1 shows as much of the picture as fits the screen's shape.
	MinZoom = 1.0
	MaxZoom = 5.0

	maxPreviewSide = 2000
)

var ErrNotAPicture = errors.New("background: file is not a JPEG or PNG picture")

// Store holds the current choice in memory. Persisting it is the caller's job.
type Store struct {
	mu     sync.Mutex
	path   string
	focusX float64
	focusY float64
	zoom   float64

	cacheKey string
	cachePNG []byte

	sourceKey string
	sourcePNG []byte
}

func New(path string, focusX, focusY float64) *Store {
	return &Store{path: path, focusX: clampUnit(focusX), focusY: clampUnit(focusY), zoom: MinZoom}
}

func (s *Store) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *Store) Focus() (x, y float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focusX, s.focusY
}

// View is the focus plus how far the picture is zoomed in.
func (s *Store) View() (x, y, zoom float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focusX, s.focusY, s.zoom
}

func (s *Store) SetView(x, y, zoom float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focusX, s.focusY, s.zoom = clampUnit(x), clampUnit(y), clampZoom(zoom)
}

func (s *Store) Set(path string) error {
	if err := check(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	return nil
}

// SetFocus picks which part of the picture survives cropping: 0 keeps the left
// or top edge, 1 the right or bottom edge.
func (s *Store) SetFocus(x, y float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focusX, s.focusY = clampUnit(x), clampUnit(y)
}

// Restore puts back a choice exactly as it was, without checking the file again.
func (s *Store) Restore(path string, focusX, focusY, zoom float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	s.focusX, s.focusY, s.zoom = clampUnit(focusX), clampUnit(focusY), clampZoom(zoom)
}

// FileHash identifies the file alone, ignoring the focus. It is "" when there is no usable picture.
func (s *Store) FileHash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fileHash()
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path, s.cacheKey, s.cachePNG = "", "", nil
	s.sourceKey, s.sourcePNG = "", nil
	s.focusX, s.focusY, s.zoom = centered, centered, MinZoom
}

// Hash identifies the picture and its crop, and changes when the file is
// replaced or edited, the focus moves or the zoom changes. It is "" when there is no usable picture.
func (s *Store) Hash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hash()
}

// Render returns the picture as a grayscale PNG of exactly width x height,
// scaled to cover the area and cropped around the focus.
func (s *Store) Render(width, height int) ([]byte, bool) {
	if width < 1 || height < 1 || width > maxSide || height > maxSide {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := s.hash()
	if hash == "" {
		return nil, false
	}
	key := fmt.Sprintf("%s/%dx%d", hash, width, height)
	if key == s.cacheKey {
		return s.cachePNG, true
	}
	data, err := render(s.path, width, height, s.focusX, s.focusY, s.zoom)
	if err != nil {
		return nil, false
	}
	s.cacheKey, s.cachePNG = key, data
	return data, true
}

// Source returns the whole picture, uncropped, as a grayscale PNG no larger
// than maxSide on its longer edge. The window uses it to preview the crop.
func (s *Store) Source(maxSide int) ([]byte, bool) {
	if maxSide < 1 || maxSide > maxPreviewSide {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	fileHash := s.fileHash()
	if fileHash == "" {
		return nil, false
	}
	key := fmt.Sprintf("%s/source/%d", fileHash, maxSide)
	if key == s.sourceKey {
		return s.sourcePNG, true
	}
	data, err := renderSource(s.path, maxSide)
	if err != nil {
		return nil, false
	}
	s.sourceKey, s.sourcePNG = key, data
	return data, true
}

// fileHash identifies the file alone, ignoring the focus.
func (s *Store) fileHash() string {
	if s.path == "" {
		return ""
	}
	info, err := os.Stat(s.path)
	if err != nil || info.IsDir() {
		return ""
	}
	text := fmt.Sprintf("%s|%d|%d", s.path, info.Size(), info.ModTime().UnixNano())
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

func (s *Store) hash() string {
	if s.path == "" {
		return ""
	}
	info, err := os.Stat(s.path)
	if err != nil || info.IsDir() {
		return ""
	}
	text := fmt.Sprintf(
		"%s|%d|%d|%.4f|%.4f|%.4f",
		s.path,
		info.Size(),
		info.ModTime().UnixNano(),
		s.focusX,
		s.focusY,
		s.zoom,
	)
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

func clampUnit(v float64) float64 {
	return min(max(v, 0), 1)
}

func clampZoom(v float64) float64 {
	return min(max(v, MinZoom), MaxZoom)
}

func check(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return ErrNotAPicture
	}
	if config.Width < 1 || config.Height < 1 || config.Width*config.Height > maxSourcePixels {
		return ErrNotAPicture
	}
	return nil
}

func render(path string, width, height int, focusX, focusY, zoom float64) ([]byte, error) {
	if err := check(path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	dst := image.NewGray(image.Rect(0, 0, width, height))
	crop := coverRect(src.Bounds(), width, height, focusX, focusY, zoom)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderSource(path string, maxSide int) ([]byte, error) {
	if err := check(path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	width, height := src.Bounds().Dx(), src.Bounds().Dy()
	if width > maxSide || height > maxSide {
		if width >= height {
			width, height = maxSide, max(height*maxSide/width, 1)
		} else {
			width, height = max(width*maxSide/height, 1), maxSide
		}
	}
	dst := image.NewGray(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// coverRect picks the largest part of the source with the target's aspect
// ratio, so the picture fills the screen without being stretched, then shrinks
// it by the zoom. The focus slides that part along whichever axis has room to spare.
func coverRect(source image.Rectangle, width, height int, focusX, focusY, zoom float64) image.Rectangle {
	sw, sh := source.Dx(), source.Dy()
	cropW, cropH := sw, sw*height/width
	if cropH > sh {
		cropW, cropH = sh*width/height, sh
	}
	zoom = clampZoom(zoom)
	cropW, cropH = max(int(float64(cropW)/zoom), 1), max(int(float64(cropH)/zoom), 1)
	left := source.Min.X + int(float64(sw-cropW)*focusX)
	top := source.Min.Y + int(float64(sh-cropH)*focusY)
	return image.Rect(left, top, left+cropW, top+cropH)
}
