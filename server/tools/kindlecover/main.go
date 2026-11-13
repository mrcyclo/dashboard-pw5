// Command kindlecover draws the book cover the Kindle home screen shows for the
// app: a portrait page with the app icon and its name.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"claudedash/internal/tray"
)

// The Kindle home screen's cover slots are about 1.52 times taller than wide;
// a squatter picture is shrunk to fit and sits lower than the books beside it.
const (
	coverWidth  = 600
	coverHeight = 912
	iconSize    = 440
)

// background matches the icon's tile, so the icon's rounded corners vanish into the page.
var background = color.NRGBA{R: 0x23, G: 0x2f, B: 0x3e, A: 0xff}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: kindlecover <bold font.ttf> <out.png>")
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	img, err := cover(data)
	if err != nil {
		log.Fatal(err)
	}
	out, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		log.Fatal(err)
	}
}

func cover(boldTTF []byte) (*image.NRGBA, error) {
	img := image.NewNRGBA(image.Rect(0, 0, coverWidth, coverHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)

	icon, err := png.Decode(bytes.NewReader(tray.IconPNG(iconSize)))
	if err != nil {
		return nil, err
	}
	at := image.Pt((coverWidth-iconSize)/2, 126)
	draw.Draw(img, image.Rectangle{at, at.Add(image.Pt(iconSize, iconSize))}, icon, image.Point{}, draw.Over)

	bold, err := opentype.Parse(boldTTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: 64, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	for i, line := range []string{"Claude", "Dashboard"} {
		d := font.Drawer{Dst: img, Src: image.White, Face: face}
		x := (fixed.I(coverWidth) - d.MeasureString(line)) / 2
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(656 + i*84)}
		d.DrawString(line)
	}
	return img, nil
}
