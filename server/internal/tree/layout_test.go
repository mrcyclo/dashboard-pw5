package tree

import (
	"encoding/json"
	"image"
	"os"
	"testing"
	"time"
)

func fakeMeasure(text string, size int, _ bool) int {
	return len(text) * size / 2
}

func TestFixtures(t *testing.T) {
	data, err := os.ReadFile("../widget/testdata/tree_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name string         `json:"name"`
		Box  map[string]int `json:"box"`
		Tree any            `json:"tree"`
		Want []struct {
			ID         string `json:"id"`
			X, Y, W, H int
		} `json:"expect"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		box := image.Rect(0, 0, f.Box["w"], f.Box["h"])
		var got []Box
		for _, b := range Layout(f.Tree, box, fakeMeasure, time.Unix(0, 0)) {
			if !b.In(box) {
				t.Errorf("%s: box %+v leaves the widget", f.Name, b)
			}
			if _, ok := b.Node["id"]; ok {
				got = append(got, b)
			}
		}
		if len(got) != len(f.Want) {
			t.Errorf("%s: %d boxes, want %d", f.Name, len(got), len(f.Want))
			continue
		}
		for i, want := range f.Want {
			b := got[i]
			if b.Node["id"] != want.ID || b.X != want.X || b.Y != want.Y || b.W != want.W || b.H != want.H {
				t.Errorf("%s: %s want %d,%d %dx%d got %v %d,%d %dx%d", f.Name, want.ID, want.X, want.Y, want.W, want.H, b.Node["id"], b.X, b.Y, b.W, b.H)
			}
		}
	}
}

// A Monday, 09:05 local time.
var monday = time.Date(2026, 11, 16, 9, 5, 0, 0, time.Local)

func TestClockAndCountdown(t *testing.T) {
	root := map[string]any{"kind": "col", "children": []any{
		map[string]any{"kind": "clock", "format": "time", "size": 20.0},
		map[string]any{"kind": "clock", "format": "date", "size": 20.0},
		map[string]any{"kind": "countdown", "to": float64(monday.Unix() + 90), "size": 20.0},
		map[string]any{"kind": "countdown", "to": float64(monday.Unix() - 1), "size": 20.0},
	}}
	boxes := Layout(root, image.Rect(0, 0, 400, 200), fakeMeasure, monday)
	want := []string{"09:05", "Monday, 16/11/2026", "resets in 2m", ""}
	for i, w := range want {
		if boxes[i].Text != w || !boxes[i].Textual {
			t.Errorf("box %d: %q want %q", i, boxes[i].Text, w)
		}
	}
}

func TestAtSwitch(t *testing.T) {
	now := monday.Unix()
	node := map[string]any{"kind": "text", "text": "74% used", "size": 10.0, "at": map[string]any{"time": float64(now), "set": map[string]any{"text": "0% used"}}}
	box := image.Rect(0, 0, 100, 20)
	if got := Layout(node, box, fakeMeasure, time.Unix(now-1, 0))[0].Text; got != "74% used" {
		t.Errorf("before: %q", got)
	}
	if got := Layout(node, box, fakeMeasure, time.Unix(now, 0))[0].Text; got != "0% used" {
		t.Errorf("after: %q", got)
	}
	if node["text"] != "74% used" {
		t.Error("the tree itself must not change")
	}
	bar := Resolve(map[string]any{"kind": "bar", "percent": 74.0, "at": map[string]any{"time": float64(now), "set": map[string]any{"percent": 0.0}}}, time.Unix(now+5, 0))
	if bar["percent"] != 0.0 {
		t.Errorf("bar %v", bar["percent"])
	}
}

func TestNullsAreDefaults(t *testing.T) {
	root := map[string]any{"kind": "col", "gap": nil, "children": []any{
		map[string]any{"kind": "text", "text": nil, "size": nil, "at": nil},
		nil,
		map[string]any{"kind": "bar", "percent": 10.0, "height": 4.0},
	}}
	boxes := Layout(root, image.Rect(0, 0, 100, 100), fakeMeasure, monday)
	if len(boxes) != 2 || boxes[0].Text != "" {
		t.Fatalf("%+v", boxes)
	}
	if got := Layout(nil, image.Rect(0, 0, 10, 10), fakeMeasure, monday); len(got) != 0 {
		t.Fatal("a missing root draws nothing")
	}
	if got := Layout("text", image.Rect(0, 0, 10, 10), fakeMeasure, monday); len(got) != 0 {
		t.Fatal("a root that is not an object draws nothing")
	}
}

func TestOffsetByRootBox(t *testing.T) {
	boxes := Layout(map[string]any{"kind": "text", "id": "t", "text": "Hi", "size": 10.0}, image.Rect(5, 7, 55, 27), fakeMeasure, monday)
	if len(boxes) != 1 || boxes[0].X != 5 || boxes[0].Y != 7 || boxes[0].W != 50 || boxes[0].H != 20 || boxes[0].Text != "Hi" {
		t.Fatalf("%+v", boxes)
	}
}

func TestCountdownCases(t *testing.T) {
	now := monday
	cases := map[int64]string{
		-5:                 "",
		0:                  "",
		30:                 "resets in 1m",
		14 * 60:            "resets in 14m",
		3600:               "resets in 1h 0m",
		2*3600 + 14*60:     "resets in 2h 14m",
		86400 - 60:         "resets in 23h 59m",
		86400:              "resets Tue 09:05",
		3*86400 + 3600 + 5: "resets Thu 10:05",
	}
	for offset, want := range cases {
		if got := CountdownText(now.Unix()+offset, now); got != want {
			t.Errorf("+%d: %q want %q", offset, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	root := map[string]any{"kind": "row", "children": []any{map[string]any{"kind": "col", "children": []any{map[string]any{"kind": "button", "id": "pp", "size": 10.0}}}}}
	if Find(root, "pp")["size"] != 10.0 || Find(root, "nope") != nil {
		t.Fatal("find")
	}
}
