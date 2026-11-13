package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"claudedash/internal/grid"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	c := Default()
	want := Config{
		Port: 8377, CacheSeconds: 300, CloseToTray: true,
		Display:    Display{PanelLighten: 0.7, FullRefreshMinutes: 0, MediaIntervalSeconds: 5, Frontlight: -1, Warmth: -1},
		Background: Background{FocusX: 0.5, FocusY: 0.5, Zoom: 1},
		Layout:     DefaultLayout(),
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "no-such.json"))
	if err != nil || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("got %+v err %v", c, err)
	}
}

func TestLoadOldFileKeepsNewDefaults(t *testing.T) {
	c, err := Load(write(t, `{"port":9000,"cache_seconds":600}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.Port, want.CacheSeconds = 9000, 600
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v", c)
	}
}

// Files written by hand before the settings page existed may hold an interval
// the page no longer offers; it is raised to the nearest allowed one.
func TestLoadSnapsLegacyInterval(t *testing.T) {
	cases := map[string]int{"120": 300, "300": 300, "400": 600, "5000": 1800, "0": 300, "-5": 300}
	for in, want := range cases {
		c, err := Load(write(t, `{"cache_seconds":`+in+`}`))
		if err != nil || c.CacheSeconds != want {
			t.Errorf("cache_seconds %s: got %d err %v, want %d", in, c.CacheSeconds, err, want)
		}
	}
}

func TestLoadInvalidFileReturnsDefaultsAndError(t *testing.T) {
	for _, body := range []string{`{`, `{"port":70000}`, `{"port":80}`, `{"display":{"panel_lighten":4}}`} {
		c, err := Load(write(t, body))
		if err == nil || !reflect.DeepEqual(c, Default()) {
			t.Errorf("%s: got %+v err %v", body, c, err)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []func(*Config){
		func(c *Config) { c.Port = 1023 },
		func(c *Config) { c.Port = 65536 },
		func(c *Config) { c.CacheSeconds = 120 },
		func(c *Config) { c.CacheSeconds = 301 },
		func(c *Config) { c.Display.PanelLighten = -0.1 },
		func(c *Config) { c.Display.PanelLighten = 1.1 },
		func(c *Config) { c.Display.FullRefreshMinutes = 20 },
		func(c *Config) { c.Display.MediaIntervalSeconds = 4 },
		func(c *Config) { c.Background.FocusX = -0.01 },
		func(c *Config) { c.Background.FocusY = 1.01 },
		func(c *Config) { c.Background.Zoom = 0.99 },
		func(c *Config) { c.Background.Zoom = 5.01 },
	}
	for i, mutate := range bad {
		c := Default()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: %+v should be rejected", i, c)
		}
	}
	good := Default()
	good.Port, good.CacheSeconds = 1024, 1800
	good.Display = Display{PanelLighten: 1, FullRefreshMinutes: 60, MediaIntervalSeconds: 30}
	good.Background = Background{Path: `C:\pictures\a.jpg`, FocusX: 0, FocusY: 1, Zoom: 5}
	if err := good.Validate(); err != nil {
		t.Fatalf("%+v should be valid: %v", good, err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Default()
	c.Port, c.CacheSeconds, c.CloseToTray = 9100, 900, false
	c.Display = Display{PanelLighten: 0.85, FullRefreshMinutes: 30, MediaIntervalSeconds: 10}
	c.Background = Background{Path: `C:\bilder\grün.png`, FocusX: 0.2, FocusY: 0.9, Zoom: 2.5}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("got %+v err %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestSaveRejectsInvalidAndKeepsFile(t *testing.T) {
	path := write(t, `{"port":9000}`)
	c := Default()
	c.Port = 80
	if err := Save(path, c); err == nil {
		t.Fatal("want error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"port":9000}` {
		t.Fatalf("file was changed: %s", data)
	}
}

func TestDisplayHash(t *testing.T) {
	a := Default().Display
	b := a
	if len(a.Hash()) != 12 || a.Hash() != b.Hash() {
		t.Fatalf("hash %q %q", a.Hash(), b.Hash())
	}
	b.PanelLighten = 0.71
	c := a
	c.MediaIntervalSeconds = 10
	d := a
	d.FullRefreshMinutes = 15
	seen := map[string]bool{a.Hash(): true}
	for _, other := range []Display{b, c, d} {
		if seen[other.Hash()] {
			t.Fatalf("%+v collides", other)
		}
		seen[other.Hash()] = true
	}
}

func TestMigrateBackground(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "background.txt")
	os.WriteFile(legacy, []byte("  C:\\pictures\\old.jpg \n"), 0o600)

	c := Default()
	if !MigrateBackground(&c, legacy) || c.Background.Path != `C:\pictures\old.jpg` {
		t.Fatalf("got %+v", c.Background)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("the legacy file must stay until the caller has saved the config")
	}

	// A choice already in the config wins, and a missing legacy file is not an error.
	c = Default()
	c.Background.Path = "new.png"
	os.WriteFile(legacy, []byte("old.jpg"), 0o600)
	if MigrateBackground(&c, legacy) || c.Background.Path != "new.png" {
		t.Fatalf("got %+v", c.Background)
	}
	if MigrateBackground(&c, filepath.Join(dir, "none.txt")) {
		t.Fatal("nothing to migrate")
	}
	if strings.Contains(c.Background.Path, "\n") {
		t.Fatal("path not trimmed")
	}
}

// Windows Notepad and PowerShell save UTF-8 with a byte order mark.
func TestLoadAcceptsByteOrderMark(t *testing.T) {
	c, err := Load(write(t, "\xef\xbb\xbf"+`{"port":9000}`))
	if err != nil || c.Port != 9000 {
		t.Fatalf("got %+v err %v", c, err)
	}
}

// A config saved before zoom existed has no zoom field; it means "not zoomed".
func TestLoadBackgroundWithoutZoom(t *testing.T) {
	c, err := Load(write(t, `{"background":{"path":"a.png","focus_x":0.2,"focus_y":0.8}}`))
	if err != nil || c.Background.Zoom != 1 || c.Background.FocusX != 0.2 {
		t.Fatalf("got %+v err %v", c.Background, err)
	}
	c, err = Load(write(t, `{"background":{"path":"a.png","focus_x":0.2,"focus_y":0.8,"zoom":0}}`))
	if err != nil || c.Background.Zoom != 1 {
		t.Fatalf("a zero zoom is the same as none: %+v err %v", c.Background, err)
	}
}

func TestLightRange(t *testing.T) {
	c := Default()
	if c.Display.Frontlight != LightUnset || c.Display.Warmth != LightUnset {
		t.Fatalf("lights start unset: %+v", c.Display)
	}
	for _, v := range []int{-1, 0, 12, 24} {
		c.Display.Frontlight, c.Display.Warmth = v, v
		if err := c.Validate(); err != nil {
			t.Errorf("%d should be valid: %v", v, err)
		}
	}
	for _, v := range []int{-2, 25} {
		c = Default()
		c.Display.Frontlight = v
		if c.Validate() == nil {
			t.Errorf("frontlight %d should be rejected", v)
		}
		c = Default()
		c.Display.Warmth = v
		if c.Validate() == nil {
			t.Errorf("warmth %d should be rejected", v)
		}
	}
	a, b, w := Default().Display, Default().Display, Default().Display
	b.Frontlight, w.Warmth = 20, 20
	if a.Hash() == b.Hash() || a.Hash() == w.Hash() || b.Hash() == w.Hash() {
		t.Fatal("the hash must follow frontlight and warmth")
	}
}

func TestLoadConvertsPercentLights(t *testing.T) {
	c, err := Load(write(t, `{"display":{"panel_lighten":0.7,"media_interval_seconds":5,"frontlight":100,"warmth":50}}`))
	if err != nil || c.Display.Frontlight != 24 || c.Display.Warmth != 12 {
		t.Fatalf("got %+v err %v", c.Display, err)
	}
	c, err = Load(write(t, `{"display":{"panel_lighten":0.7,"media_interval_seconds":5,"frontlight":-1,"warmth":24}}`))
	if err != nil || c.Display.Frontlight != -1 || c.Display.Warmth != 24 {
		t.Fatalf("levels are left alone: %+v err %v", c.Display, err)
	}
	if _, err = Load(write(t, `{"display":{"panel_lighten":0.7,"media_interval_seconds":5,"frontlight":500}}`)); err == nil {
		t.Fatal("a value beyond any percentage is still invalid")
	}
}

func TestDefaultLayout(t *testing.T) {
	want := []grid.Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "6x1", Col: 0, Row: 1},
		{Kind: "media", Size: "6x2", Col: 0, Row: 6},
	}
	if got := DefaultLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	a, b := DefaultLayout(), DefaultLayout()
	a[0].Col = 2
	if b[0].Col != 0 {
		t.Fatal("each call must return its own slice")
	}
}

func TestLoadMissingLayoutUsesDefault(t *testing.T) {
	c, err := Load(write(t, `{"port": 9000}`))
	if err != nil || !reflect.DeepEqual(c.Layout, DefaultLayout()) || c.Port != 9000 {
		t.Fatalf("got %+v err %v", c, err)
	}
}

func TestLoadInvalidLayoutResetsOnlyLayout(t *testing.T) {
	body := `{"port": 9000, "layout": [
		{"kind": "clock", "size": "6x2", "col": 0, "row": 1},
		{"kind": "claude", "size": "6x2", "col": 0, "row": 1}
	]}`
	c, err := Load(write(t, body))
	if err != nil || c.Port != 9000 || !reflect.DeepEqual(c.Layout, DefaultLayout()) {
		t.Fatalf("got %+v err %v", c, err)
	}
}

func TestValidLayoutRejects(t *testing.T) {
	cases := map[string][]grid.Placement{
		"overlap": {
			{Kind: "a", Size: "6x2", Col: 0, Row: 1},
			{Kind: "b", Size: "3x1", Col: 2, Row: 2},
		},
		"reserved":  {{Kind: "a", Size: "3x1", Col: 3, Row: 0}},
		"outside":   {{Kind: "a", Size: "6x2", Col: 0, Row: 7}},
		"duplicate": {{Kind: "a", Size: "3x1", Col: 0, Row: 1}, {Kind: "a", Size: "none"}},
		"bad size":  {{Kind: "a", Size: "big", Col: 0, Row: 1}},
		"no kind":   {{Kind: "", Size: "3x1", Col: 0, Row: 1}},
	}
	for name, layout := range cases {
		if ValidLayout(layout) {
			t.Errorf("%s should be invalid", name)
		}
		c := Default()
		c.Layout = layout
		if c.Validate() == nil {
			t.Errorf("%s: Validate should refuse it", name)
		}
	}
	if !ValidLayout(DefaultLayout()) || !ValidLayout(nil) {
		t.Error("the default and an empty layout are valid")
	}
}

func TestHiddenPlacementKeepsPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Default()
	c.Layout[2] = grid.Placement{Kind: "media", Size: grid.None, Col: 0, Row: 6}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Layout[2] != c.Layout[2] {
		t.Fatalf("got %+v err %v", got.Layout, err)
	}
}
