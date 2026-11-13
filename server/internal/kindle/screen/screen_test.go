package screen

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func widget(kind string, hash any, extra map[string]any) map[string]any {
	w := map[string]any{
		"kind": kind,
		"hash": hash,
		"rect": map[string]any{"x": 48, "y": 48, "w": 562, "h": 180},
		"tree": map[string]any{"kind": "col", "children": []any{map[string]any{"kind": "text", "text": kind, "size": 20}}},
	}
	for k, v := range extra {
		w[k] = v
	}
	return w
}

func reply(t *testing.T, layout string, widgets []any, icons map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"layout": layout, "widgets": widgets, "icons": icons, "background": "bg1", "settings": "s1", "refresh": "r1"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAccept(t *testing.T) {
	first, changes, err := Accept(nil, reply(t, "L1", []any{widget("clock", "a", nil), widget("claude", "b", nil)}, nil))
	if err != nil || !changes.Full || len(first.Widgets) != 2 {
		t.Fatalf("first reply: %+v %v", changes, err)
	}
	if first.Background != "bg1" || first.Settings != "s1" || first.Refresh != "r1" {
		t.Fatalf("hashes %+v", first)
	}
	if first.Widgets[0].Rect != image.Rect(48, 48, 610, 228) {
		t.Fatalf("rect %v", first.Widgets[0].Rect)
	}

	_, changes, _ = Accept(first, reply(t, "L1", []any{widget("clock", "a", nil), widget("claude", "b", nil)}, nil))
	if changes.Full || len(changes.Widgets) != 0 {
		t.Fatalf("same hashes: %+v", changes)
	}
	_, changes, _ = Accept(first, reply(t, "L1", []any{widget("clock", "a", nil), widget("claude", "c", nil)}, nil))
	if changes.Full || !changes.Widgets["claude"] || changes.Widgets["clock"] {
		t.Fatalf("one changed: %+v", changes)
	}
	moved, changes, _ := Accept(first, reply(t, "L2", []any{widget("clock", "a", nil)}, nil))
	if !changes.Full || len(moved.Widgets) != 1 {
		t.Fatalf("layout change: %+v", changes)
	}
	_, changes, _ = Accept(first, reply(t, "L1", []any{widget("clock", "a", nil), widget("claude", "b", map[string]any{"tree": nil})}, nil))
	if !changes.Full {
		t.Fatal("a widget dropping out repaints everything")
	}
}

func TestBrokenEntriesAreDropped(t *testing.T) {
	s, _, err := Accept(nil, reply(t, "L1", []any{
		widget("clock", "a", nil),
		widget("claude", "b", map[string]any{"tree": nil}),
		widget("media", "c", map[string]any{"rect": map[string]any{"x": 1, "y": 2, "w": "wide", "h": 4}}),
		widget("", "d", nil),
		nil,
		widget("other", nil, nil),
	}, nil))
	if err != nil || len(s.Widgets) != 2 || s.Widgets[0].Kind != "clock" || s.Widgets[1].Hash != "" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestBadReplyKeepsTheScreen(t *testing.T) {
	prev, _, _ := Accept(nil, reply(t, "L1", []any{widget("clock", "a", nil)}, nil))
	for _, bad := range [][]byte{[]byte("garbage"), []byte(`null`), []byte(`{"layout":"L1","widgets":"many"}`)} {
		s, changes, err := Accept(prev, bad)
		if err == nil || s != prev || changes.Full {
			t.Errorf("%s: %v %+v", bad, err, changes)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "screen.json")
	s, _, _ := Accept(nil, reply(t, "L1", []any{widget("clock", "a", nil)}, map[string]any{"play": "p1"}))
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !reflect.DeepEqual(loaded, s) {
		t.Fatalf("%+v %v", loaded, err)
	}
	os.WriteFile(path, []byte("not a screen"), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("garbage file")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file")
	}
}

func TestAssets(t *testing.T) {
	s, _, _ := Accept(nil, reply(t, "L1", []any{
		widget("media", "m", map[string]any{"tree": map[string]any{"kind": "row", "children": []any{
			map[string]any{"kind": "image", "src": "/media/art.png?v=1", "w": 120, "h": 120},
			map[string]any{"kind": "button", "icon": "play", "size": 120},
		}}}),
	}, map[string]any{"play": "p1", "pause": "p2", "bad": 5}))
	got := map[string]string{}
	for _, a := range Assets(s) {
		got[a.Key] = a.Path
	}
	want := map[string]string{
		"icon:pause:p2":            "/icons/pause.svg",
		"icon:play:p1":             "/icons/play.svg",
		"image:/media/art.png?v=1": "/media/art.png?v=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestStaleIcons(t *testing.T) {
	files := []string{"play-p1.svg", "play-old.svg", "pause-p2.svg", "gone-x.svg", "notes.txt"}
	stale := StaleIcons(files, map[string]string{"play": "p1", "pause": "p2"})
	sort.Strings(stale)
	if !reflect.DeepEqual(stale, []string{"gone-x.svg", "play-old.svg"}) {
		t.Fatalf("%v", stale)
	}
	if len(StaleIcons(files, nil)) != 0 {
		t.Fatal("nothing is removed before the PC listed its icons")
	}
}
