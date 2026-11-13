package render

import (
	"os"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Fonts are the two weights the dashboard draws with (Noto Sans on the device).
type Fonts struct {
	Regular, Bold *opentype.Font
}

func LoadFonts(regular, bold string) (Fonts, error) {
	r, err := parseFile(regular)
	if err != nil {
		return Fonts{}, err
	}
	b, err := parseFile(bold)
	if err != nil {
		return Fonts{}, err
	}
	return Fonts{Regular: r, Bold: b}, nil
}

func parseFile(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return opentype.Parse(data)
}

type faceKey struct {
	font *opentype.Font
	size int
}

var faces sync.Map

// face is the font at a pixel size: tree sizes are pixel heights of the em box.
func (f Fonts) face(size int, bold bool) font.Face {
	source := f.Regular
	if bold {
		source = f.Bold
	}
	key := faceKey{source, size}
	if cached, ok := faces.Load(key); ok {
		return cached.(font.Face)
	}
	face, err := opentype.NewFace(source, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	actual, _ := faces.LoadOrStore(key, face)
	return actual.(font.Face)
}

// Measure gives a text's width in pixels, as the layout engine expects.
func (f Fonts) Measure(text string, size int, bold bool) int {
	face := f.face(max(size, 1), bold)
	if face == nil {
		return 0
	}
	return font.MeasureString(face, text).Ceil()
}

const ellipsis = "…"

// fit cuts text and adds an ellipsis until it is no wider than width.
func (f Fonts) fit(text string, size int, bold bool, width int) string {
	if f.Measure(text, size, bold) <= width {
		return text
	}
	for len(text) > 0 {
		_, last := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-last]
		if f.Measure(text+ellipsis, size, bold) <= width {
			return text + ellipsis
		}
	}
	return ""
}
