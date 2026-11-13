// Package screen turns a /screen.json reply into what has to be redrawn, and
// keeps the last screen on disk so the dashboard can show it with the PC off.
package screen

import (
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"claudedash/internal/tree"
)

type Widget struct {
	Kind string
	Rect image.Rectangle
	// Hash is "" when the PC sent none; such a widget is redrawn on every reply.
	Hash string
	Tree tree.Node
}

type Screen struct {
	Layout     string
	Widgets    []Widget
	Icons      map[string]string
	Background string
	Settings   string
	Refresh    string
}

// Changes says what to repaint: everything, or the listed widget kinds.
type Changes struct {
	Full    bool
	Widgets map[string]bool
}

var errNotAScreen = errors.New("screen: reply is not a screen")

func text(v any) string {
	s, _ := v.(string)
	return s
}

func rect(v any) (image.Rectangle, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return image.Rectangle{}, false
	}
	var values [4]int
	for i, key := range []string{"x", "y", "w", "h"} {
		f, ok := m[key].(float64)
		if !ok {
			return image.Rectangle{}, false
		}
		values[i] = int(f)
	}
	return image.Rect(values[0], values[1], values[0]+values[2], values[1]+values[3]), true
}

func widgets(list []any) []Widget {
	var out []Widget
	for _, item := range list {
		w, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind := text(w["kind"])
		r, okRect := rect(w["rect"])
		t, okTree := w["tree"].(map[string]any)
		if kind == "" || !okRect || !okTree {
			continue
		}
		out = append(out, Widget{Kind: kind, Rect: r, Hash: text(w["hash"]), Tree: t})
	}
	return out
}

func icons(v any) map[string]string {
	out := map[string]string{}
	m, _ := v.(map[string]any)
	for name, hash := range m {
		if h, ok := hash.(string); ok {
			out[name] = h
		}
	}
	return out
}

// Accept reads a reply. A reply that is not a screen keeps prev and returns an error.
func Accept(prev *Screen, reply []byte) (*Screen, Changes, error) {
	changes := Changes{Widgets: map[string]bool{}}
	var raw map[string]any
	if err := json.Unmarshal(reply, &raw); err != nil || raw == nil {
		return prev, changes, errNotAScreen
	}
	list, ok := raw["widgets"].([]any)
	if !ok {
		return prev, changes, errNotAScreen
	}
	current := &Screen{
		Layout:     text(raw["layout"]),
		Widgets:    widgets(list),
		Icons:      icons(raw["icons"]),
		Background: text(raw["background"]),
		Settings:   text(raw["settings"]),
		Refresh:    text(raw["refresh"]),
	}
	if prev == nil || prev.Layout != current.Layout || len(prev.Widgets) != len(current.Widgets) {
		changes.Full = true
		return current, changes, nil
	}
	old := map[string]string{}
	for _, w := range prev.Widgets {
		old[w.Kind] = w.Hash
	}
	for _, w := range current.Widgets {
		hash, seen := old[w.Kind]
		// The same layout with another set of widgets: one dropped out, its pixels must go.
		if !seen {
			changes.Full = true
			return current, changes, nil
		}
		if hash != w.Hash || w.Hash == "" {
			changes.Widgets[w.Kind] = true
		}
	}
	return current, changes, nil
}

// Save writes the screen atomically, so a power cut never leaves half a file.
func Save(path string, s *Screen) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".screen-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Load(path string) (*Screen, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Screen
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Asset is something the screen needs downloaded. Key changes with the content,
// so a new cover or icon is fetched and a failed one is retried by key.
type Asset struct {
	Key  string
	Path string
	// Icon is the icon's name, or "" for a picture.
	Icon string
}

func collectImages(node any, seen map[string]bool, out *[]Asset) {
	n, ok := node.(map[string]any)
	if !ok {
		return
	}
	if src := text(n["src"]); n["kind"] == "image" && src != "" && !seen[src] {
		seen[src] = true
		*out = append(*out, Asset{Key: "image:" + src, Path: src})
	}
	children, _ := n["children"].([]any)
	for _, child := range children {
		collectImages(child, seen, out)
	}
}

func Assets(s *Screen) []Asset {
	if s == nil {
		return nil
	}
	var out []Asset
	names := make([]string, 0, len(s.Icons))
	for name := range s.Icons {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, Asset{Key: "icon:" + name + ":" + s.Icons[name], Path: "/icons/" + name + ".svg", Icon: name})
	}
	seen := map[string]bool{}
	for _, w := range s.Widgets {
		collectImages(w.Tree, seen, &out)
	}
	return out
}

var iconFile = regexp.MustCompile(`^(.+)-([^-]+)\.svg$`)

// StaleIcons picks the cached icon files ("<name>-<hash>.svg") the icon list no
// longer names. With no list yet, nothing is stale.
func StaleIcons(files []string, icons map[string]string) []string {
	var stale []string
	if len(icons) == 0 {
		return stale
	}
	for _, f := range files {
		m := iconFile.FindStringSubmatch(f)
		if m != nil && icons[m[1]] != m[2] {
			stale = append(stale, f)
		}
	}
	return stale
}
