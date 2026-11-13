// Package grid is the Kindle screen's widget grid: where cells are, which are
// reserved, and where a widget of a given size can go.
package grid

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// All measurements are at the 1236 x 1648 reference screen.
const (
	Cols = 6
	Rows = 8

	ScreenWidth  = 1236
	ScreenHeight = 1648

	margin = 40
	gap    = 16

	// None is the size of a hidden widget.
	None = "none"
)

var ErrNoRoom = errors.New("grid: no room")

type Size struct {
	Cols, Rows int
}

func ParseSize(s string) (Size, bool) {
	cols, rows, ok := strings.Cut(s, "x")
	if !ok {
		return Size{}, false
	}
	c, errC := strconv.Atoi(cols)
	r, errR := strconv.Atoi(rows)
	if errC != nil || errR != nil {
		return Size{}, false
	}
	if c < 1 || c > Cols || r < 1 || r > Rows {
		return Size{}, false
	}
	return Size{Cols: c, Rows: r}, true
}

func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.Cols, s.Rows)
}

// Placement is one widget kind's spot. A hidden widget keeps Col and Row so
// it can come back where it was.
type Placement struct {
	Kind string `json:"kind"`
	Size string `json:"size"`
	Col  int    `json:"col"`
	Row  int    `json:"row"`
}

func (p Placement) Visible() (Size, bool) {
	return ParseSize(p.Size)
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func cellWidth() float64 {
	return float64(ScreenWidth-2*margin) / Cols
}

func cellHeight() float64 {
	return float64(ScreenHeight-2*margin) / Rows
}

// CellRect is the widget's rectangle in pixels, inset by half the gap on each side.
func CellRect(col, row int, s Size) Rect {
	cw, ch := cellWidth(), cellHeight()
	return Rect{
		X: int(math.Round(margin + float64(col)*cw + gap/2)),
		Y: int(math.Round(margin + float64(row)*ch + gap/2)),
		W: int(math.Round(float64(s.Cols)*cw - gap)),
		H: int(math.Round(float64(s.Rows)*ch - gap)),
	}
}

// Reserved cells hold the battery, refresh and exit controls.
func Reserved(col, row int) bool {
	return row == 0 && col >= 3
}

// Fits reports whether kind could sit at (col, row) with size s, ignoring kind's own current spot.
func Fits(p []Placement, kind string, s Size, col, row int) bool {
	if col < 0 || row < 0 || col+s.Cols > Cols || row+s.Rows > Rows {
		return false
	}
	for c := col; c < col+s.Cols; c++ {
		for r := row; r < row+s.Rows; r++ {
			if Reserved(c, r) {
				return false
			}
		}
	}
	for _, other := range p {
		if other.Kind == kind {
			continue
		}
		size, visible := other.Visible()
		if !visible {
			continue
		}
		if overlaps(col, row, s, other.Col, other.Row, size) {
			return false
		}
	}
	return true
}

func overlaps(c1, r1 int, s1 Size, c2, r2 int, s2 Size) bool {
	return c1 < c2+s2.Cols && c2 < c1+s1.Cols && r1 < r2+s2.Rows && r2 < r1+s1.Rows
}

func find(p []Placement, kind string) (Placement, bool) {
	for _, item := range p {
		if item.Kind == kind {
			return item, true
		}
	}
	return Placement{}, false
}

// Show finds a spot for a hidden widget: where it was last, else the first free cell.
func Show(p []Placement, kind string, s Size) (col, row int, ok bool) {
	if current, found := find(p, kind); found && Fits(p, kind, s, current.Col, current.Row) {
		return current.Col, current.Row, true
	}
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			if Fits(p, kind, s, c, r) {
				return c, r, true
			}
		}
	}
	return 0, 0, false
}

// Resize keeps a visible widget's top-left corner if the new size fits there,
// else moves it to the nearest cell that fits.
func Resize(p []Placement, kind string, s Size) (col, row int, ok bool) {
	current, _ := find(p, kind)
	if Fits(p, kind, s, current.Col, current.Row) {
		return current.Col, current.Row, true
	}
	best := -1
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			if !Fits(p, kind, s, c, r) {
				continue
			}
			distance := abs(c-current.Col) + abs(r-current.Row)
			if best < 0 || distance < best {
				best, col, row = distance, c, r
			}
		}
	}
	return col, row, best >= 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
