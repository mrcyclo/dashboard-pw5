package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"claudedash/internal/grid"
)

var ErrUnknownAction = errors.New("widget: unknown action")

// PlacedWidget is one visible widget as the Kindle draws it.
type PlacedWidget struct {
	Kind string    `json:"kind"`
	Rect grid.Rect `json:"rect"`
	// Hash changes with the tree, so the Kindle redraws only widgets that changed.
	Hash string `json:"hash"`
	Tree Node   `json:"tree"`
}

// Screen is the reply to the Kindle's poll.
type Screen struct {
	// Layout changes when a widget is shown, hidden, moved or resized.
	Layout  string         `json:"layout"`
	Widgets []PlacedWidget `json:"widgets"`
	// Icons maps every icon the trees name to its hash, so the Kindle knows when to fetch it again.
	Icons      map[string]string `json:"icons"`
	Background string            `json:"background,omitempty"`
	Settings   string            `json:"settings,omitempty"`
	Refresh    string            `json:"refresh,omitempty"`
}

// BuildScreen draws the visible placements of known kinds at sizes they support.
func BuildScreen(c *Catalog, layout []grid.Placement, d Data) Screen {
	s := Screen{Widgets: []PlacedWidget{}, Icons: map[string]string{}}
	var shown []grid.Placement
	for _, p := range layout {
		size, visible := p.Visible()
		kind, known := c.Get(p.Kind)
		if !visible || !known || !c.Supports(p.Kind, p.Size) {
			continue
		}
		tree := kind.Build(p.Size, d)
		s.Widgets = append(s.Widgets, PlacedWidget{
			Kind: p.Kind,
			Rect: grid.CellRect(p.Col, p.Row, size),
			Hash: Hash(tree),
			Tree: tree,
		})
		shown = append(shown, p)
		collectIcons(tree, s.Icons)
	}
	s.Layout = layoutHash(shown)
	return s
}

func layoutHash(shown []grid.Placement) string {
	data, _ := json.Marshal(shown)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// collectIcons includes the other icon of a toggle patch, which the Kindle shows on a tap.
func collectIcons(n Node, icons map[string]string) {
	add := func(name any) {
		text, ok := name.(string)
		if !ok {
			return
		}
		if hash := IconHash(text); hash != "" {
			icons[text] = hash
		}
	}
	switch n["kind"] {
	case "icon":
		add(n["name"])
	case "button":
		add(n["icon"])
	}
	if ops, ok := n["patch"].([]PatchOp); ok {
		for _, op := range ops {
			if op["prop"] != "icon" {
				continue
			}
			add(op["value"])
			if values, ok := op["values"].([]any); ok {
				for _, v := range values {
					add(v)
				}
			}
		}
	}
	children, _ := n["children"].([]Node)
	for _, child := range children {
		collectIcons(child, icons)
	}
}
