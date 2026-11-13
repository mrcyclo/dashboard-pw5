package widget

import (
	"testing"

	"claudedash/internal/grid"
	"claudedash/internal/media"
)

func testLayout() []grid.Placement {
	return []grid.Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "6x1", Col: 0, Row: 1},
		{Kind: "media", Size: grid.None, Col: 0, Row: 6},
		{Kind: "weather", Size: "3x1", Col: 0, Row: 3},
	}
}

func TestBuildScreenSkipsHiddenAndUnknown(t *testing.T) {
	s := BuildScreen(Default(nil), testLayout(), Data{Now: testNow})
	if len(s.Widgets) != 2 || s.Widgets[0].Kind != "clock" || s.Widgets[1].Kind != "claude" {
		t.Fatalf("%+v", s.Widgets)
	}
	for _, w := range s.Widgets {
		if w.Hash != Hash(w.Tree) || w.Tree == nil {
			t.Errorf("%s: hash %q", w.Kind, w.Hash)
		}
	}
}

func TestBuildScreenRectsFromGrid(t *testing.T) {
	s := BuildScreen(Default(nil), testLayout(), Data{Now: testNow})
	if got := s.Widgets[1].Rect; got != (grid.Rect{X: 48, Y: 244, W: 1140, H: 180}) {
		t.Fatalf("claude rect %+v", got)
	}
}

func TestBuildScreenSkipsUnsupportedSize(t *testing.T) {
	layout := []grid.Placement{{Kind: "clock", Size: "6x2", Col: 0, Row: 1}}
	if s := BuildScreen(Default(nil), layout, Data{Now: testNow}); len(s.Widgets) != 0 {
		t.Fatalf("%+v", s.Widgets)
	}
}

func TestLayoutHashChangesOnMove(t *testing.T) {
	a := BuildScreen(Default(nil), testLayout(), Data{Now: testNow})
	moved := testLayout()
	moved[1].Row = 2
	b := BuildScreen(Default(nil), moved, Data{Now: testNow})
	if a.Layout == "" || a.Layout == b.Layout {
		t.Fatalf("%q %q", a.Layout, b.Layout)
	}
	later := BuildScreen(Default(nil), testLayout(), Data{Now: testNow.Add(3600e9)})
	if later.Layout != a.Layout {
		t.Fatal("the layout hash depends only on placements")
	}
}

func TestIconsListed(t *testing.T) {
	layout := []grid.Placement{{Kind: "media", Size: "6x2", Col: 0, Row: 6}}
	vol := 10
	s := BuildScreen(Default(nil), layout, Data{Now: testNow, Media: media.State{Active: true, Volume: &vol}})
	for _, name := range []string{"previous", "play", "pause", "next", "note", "volume-low", "volume-high"} {
		if s.Icons[name] != IconHash(name) || s.Icons[name] == "" {
			t.Errorf("icon %s: %q", name, s.Icons[name])
		}
	}
}
