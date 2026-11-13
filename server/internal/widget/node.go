// Package widget describes what the Kindle draws: each widget kind builds a
// tree of simple components from the PC's data, and the Kindle and the
// window's preview both lay it out with the same rules.
package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Node is one component of a widget's tree, serialized as JSON for the Kindle.
// Sizes are pixels on the 1236 px wide reference screen.
type Node map[string]any

// PatchOp predicts what a tap changes, so the Kindle can show it before the PC answers.
type PatchOp map[string]any

func node(kind string, props Node) Node {
	props["kind"] = kind
	return props
}

func Col(gap int, children ...Node) Node {
	return node("col", Node{"gap": gap, "children": children})
}

func Row(gap int, children ...Node) Node {
	return node("row", Node{"gap": gap, "align": "center", "children": children})
}

func Text(text string, size int) Node {
	return node("text", Node{"text": text, "size": size})
}

func Space(size int) Node {
	return node("space", Node{"size": size})
}

func Rule() Node {
	return node("rule", Node{"thickness": 2})
}

func Bar(percent, height int) Node {
	return node("bar", Node{"percent": percent, "height": height})
}

func Icon(name string, size int) Node {
	return node("icon", Node{"name": name, "size": size})
}

func Image(src string, w, h int) Node {
	return node("image", Node{"src": src, "w": w, "h": h})
}

func Button(icon string, size int, action string) Node {
	return node("button", Node{"icon": icon, "size": size, "action": action})
}

// Clock is drawn by the Kindle from its own clock: format is "time" or "date".
func Clock(format string, size int) Node {
	return node("clock", Node{"format": format, "size": size})
}

// Countdown is drawn by the Kindle as "resets in ..." until the given unix time.
func Countdown(to int64, size int) Node {
	return node("countdown", Node{"to": to, "size": size})
}

func (n Node) ID(id string) Node {
	n["id"] = id
	return n
}

func (n Node) Grow(weight int) Node {
	n["grow"] = weight
	return n
}

func (n Node) Bold() Node {
	n["bold"] = true
	return n
}

func (n Node) Dim() Node {
	n["dim"] = true
	return n
}

func (n Node) Align(align string) Node {
	n["align"] = align
	return n
}

func (n Node) Disabled() Node {
	n["disabled"] = true
	return n
}

// Plain draws a button as its icon only, without the round frame.
func (n Node) Plain() Node {
	n["plain"] = true
	return n
}

func (n Node) Patch(ops ...PatchOp) Node {
	n["patch"] = ops
	return n
}

// At swaps in the props of set once the unix time passes, even with the PC off.
func (n Node) At(time int64, set map[string]any) Node {
	n["at"] = map[string]any{"time": time, "set": set}
	return n
}

func Set(id, prop string, value any) PatchOp {
	return PatchOp{"id": id, "op": "set", "prop": prop, "value": value}
}

// Toggle turns a into b, and anything else into a.
func Toggle(id, prop string, a, b any) PatchOp {
	return PatchOp{"id": id, "op": "toggle", "prop": prop, "values": []any{a, b}}
}

// Add adds by to a number (or a text holding one) and keeps it within min and max.
func Add(id, prop string, by, min, max int) PatchOp {
	return PatchOp{"id": id, "op": "add", "prop": prop, "by": by, "min": min, "max": max}
}

// Hash identifies a tree's content, so the Kindle redraws only widgets that changed.
func Hash(n Node) string {
	data, err := json.Marshal(n)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}
