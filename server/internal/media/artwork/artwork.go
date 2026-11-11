package artwork

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

const (
	maxSide = 400
	// Cover art comes from whatever app is playing, so its size is not trusted.
	maxPixels = 50_000_000
)

var ErrTooLarge = errors.New("artwork: image too large")

// Process turns cover art into a grayscale PNG that fits a 400x400 box, plus a
// short content hash clients use to tell when the picture changed.
func Process(raw []byte) (pngData []byte, hash string, err error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > maxPixels {
		return nil, "", ErrTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}

	width, height := fit(src.Bounds().Dx(), src.Bounds().Dy())
	dst := image.NewGray(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(sum[:])[:12], nil
}

func fit(width, height int) (int, int) {
	if width <= maxSide && height <= maxSide {
		return width, height
	}
	if width >= height {
		return maxSide, max(height*maxSide/width, 1)
	}
	return max(width*maxSide/height, 1), maxSide
}
