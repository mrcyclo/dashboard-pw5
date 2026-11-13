package grid

import "testing"

func TestParseSize(t *testing.T) {
	s, ok := ParseSize("3x1")
	if !ok || s != (Size{Cols: 3, Rows: 1}) || s.String() != "3x1" {
		t.Fatalf("got %+v %v", s, ok)
	}
	for _, bad := range []string{"", "none", "0x1", "7x1", "1x9", "3x", "x1", "3*1", "3x1x2"} {
		if _, ok := ParseSize(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestCellRect(t *testing.T) {
	if got := CellRect(0, 0, Size{3, 1}); got != (Rect{48, 48, 562, 180}) {
		t.Errorf("clock rect %+v", got)
	}
	if got := CellRect(0, 6, Size{6, 2}); got != (Rect{48, 1224, 1140, 376}) {
		t.Errorf("media rect %+v", got)
	}
	if got := CellRect(0, 1, Size{6, 2}); got != (Rect{48, 244, 1140, 376}) {
		t.Errorf("claude rect %+v", got)
	}
}

func TestReserved(t *testing.T) {
	for col := 3; col <= 5; col++ {
		if !Reserved(col, 0) {
			t.Errorf("(%d,0) should be reserved", col)
		}
	}
	if Reserved(2, 0) || Reserved(3, 1) {
		t.Error("only row 0, columns 3 to 5 are reserved")
	}
}

func layout() []Placement {
	return []Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "6x2", Col: 0, Row: 1},
		{Kind: "media", Size: "6x2", Col: 0, Row: 6},
	}
}

func TestFits(t *testing.T) {
	p := layout()
	if Fits(p, "media", Size{6, 2}, 0, 2) {
		t.Error("overlaps claude")
	}
	if Fits(p, "media", Size{3, 1}, 3, 0) {
		t.Error("reserved corner")
	}
	if Fits(p, "media", Size{6, 2}, 0, 7) || Fits(p, "media", Size{3, 1}, 4, 3) {
		t.Error("outside the grid")
	}
	if !Fits(p, "media", Size{6, 2}, 0, 5) {
		t.Error("its own placement must be ignored")
	}
	if !Fits(p, "media", Size{6, 2}, 0, 3) {
		t.Error("free rows 3 and 4")
	}
}

func TestHiddenPlacementsDoNotBlock(t *testing.T) {
	p := layout()
	p[1].Size = None
	if !Fits(p, "media", Size{6, 2}, 0, 1) {
		t.Error("a hidden widget takes no room")
	}
}

func TestShowUsesRememberedThenFirstFree(t *testing.T) {
	p := layout()
	p[2].Size = None
	col, row, ok := Show(p, "media", Size{6, 2})
	if !ok || col != 0 || row != 6 {
		t.Fatalf("remembered spot: %d,%d %v", col, row, ok)
	}
	p = append(p, Placement{Kind: "other", Size: "6x2", Col: 0, Row: 6})
	col, row, ok = Show(p, "media", Size{3, 3})
	if !ok || col != 0 || row != 3 {
		t.Fatalf("first free: %d,%d %v", col, row, ok)
	}
}

func TestResizeKeepsCornerThenNearest(t *testing.T) {
	p := layout()
	col, row, ok := Resize(p, "clock", Size{6, 1})
	if !ok || col != 0 || row != 3 {
		t.Fatalf("6x1 clock cannot stay in row 0, nearest free is row 3: %d,%d %v", col, row, ok)
	}
	col, row, ok = Resize(p, "media", Size{6, 1})
	if !ok || col != 0 || row != 6 {
		t.Fatalf("smaller size keeps its corner: %d,%d %v", col, row, ok)
	}
}

func TestShowNoRoom(t *testing.T) {
	p := []Placement{
		{Kind: "a", Size: "3x1", Col: 0, Row: 0},
		{Kind: "b", Size: "6x2", Col: 0, Row: 1},
		{Kind: "c", Size: "6x2", Col: 0, Row: 3},
		{Kind: "d", Size: "6x2", Col: 0, Row: 5},
		{Kind: "e", Size: "6x1", Col: 0, Row: 7},
	}
	if _, _, ok := Show(p, "f", Size{3, 1}); ok {
		t.Fatal("grid is full")
	}
}
