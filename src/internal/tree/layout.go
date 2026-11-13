// Package tree lays out a widget's component tree, as sent by the PC in
// /screen.json, for the Kindle app. The window's preview does the same in
// src/frontend/src/lib/tree.js, and both are checked against
// src/internal/widget/testdata/tree_fixtures.json.
//
// Rules:
//  1. col stacks its children; they get its width when align is "stretch"
//     (default) or are bars and rules, else their own width aligned by align.
//     Leftover height is shared by "grow" weight.
//  2. row places children left to right. A growing child starts at width 0 and
//     takes its share of the leftover width; others keep their own width.
//     Children of a row are aligned vertically by align ("center" default),
//     except cols and rules, which take the row's full height.
//  3. Unknown kinds are skipped. Every box is clipped to the root box, and a
//     text line too short to show whole is left out.
//  4. x and y are floored, then w and h.
package tree

import (
	"image"
	"math"
	"time"
)

// Node is a component as decoded from JSON: numbers are float64, and anything
// of the wrong type counts as missing.
type Node = map[string]any

// Box is one drawable component placed on the screen.
type Box struct {
	Node       Node
	X, Y, W, H int
	// Text is the string to draw when Textual is set (text, clock, countdown).
	Text    string
	Textual bool
}

// Measure gives a text's width in pixels.
type Measure func(text string, size int, bold bool) int

const lineHeight = 1.3

var known = map[string]bool{
	"col": true, "row": true, "space": true, "rule": true, "text": true, "clock": true,
	"countdown": true, "bar": true, "icon": true, "image": true, "button": true,
}

var textual = map[string]bool{"text": true, "clock": true, "countdown": true}

func num(v any, fallback float64) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	if i, ok := v.(int); ok {
		return float64(i)
	}
	return fallback
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func kindOf(n Node) string {
	return str(n["kind"])
}

func asNode(v any) (Node, bool) {
	n, ok := v.(map[string]any)
	if !ok || !known[str(n["kind"])] {
		return nil, false
	}
	return n, true
}

// Resolve returns the node with its "at" values applied once their time has passed.
func Resolve(n Node, now time.Time) Node {
	at, ok := n["at"].(map[string]any)
	if !ok {
		return n
	}
	set, ok := at["set"].(map[string]any)
	if !ok {
		return n
	}
	when := num(at["time"], math.NaN())
	if math.IsNaN(when) || float64(now.Unix()) < when {
		return n
	}
	out := Node{}
	for k, v := range n {
		out[k] = v
	}
	for k, v := range set {
		out[k] = v
	}
	return out
}

type context struct {
	boxes   []Box
	root    image.Rectangle
	measure Measure
	now     time.Time
}

func (c *context) text(n Node) string {
	switch kindOf(n) {
	case "clock":
		if str(n["format"]) == "date" {
			return DateText(c.now)
		}
		return ClockText(c.now)
	case "countdown":
		return CountdownText(int64(num(n["to"], 0)), c.now)
	}
	return str(n["text"])
}

func (c *context) children(n Node) []Node {
	list, _ := n["children"].([]any)
	var out []Node
	for _, item := range list {
		if child, ok := asNode(item); ok {
			out = append(out, Resolve(child, c.now))
		}
	}
	return out
}

func textHeight(size float64) float64 {
	return math.Ceil(size * lineHeight)
}

// size is a node's own width and height; axis is its parent's direction.
func (c *context) size(n Node, axis string) (float64, float64) {
	kind := kindOf(n)
	switch {
	case textual[kind]:
		font := num(n["size"], 20)
		return float64(c.measure(c.text(n), int(font), n["bold"] == true)), textHeight(font)
	case kind == "icon" || kind == "button":
		side := num(n["size"], 0)
		return side, side
	case kind == "image":
		return num(n["w"], 0), num(n["h"], 0)
	case kind == "bar":
		return 0, num(n["height"], 8)
	case kind == "rule" || kind == "space":
		amount := num(n["size"], 0)
		if kind == "rule" {
			amount = num(n["thickness"], 2)
		}
		if axis == "row" {
			return amount, 0
		}
		return 0, amount
	}

	kids := c.children(n)
	var w, h float64
	for _, child := range kids {
		cw, ch := c.size(child, kind)
		if kind == "col" {
			w, h = math.Max(w, cw), h+ch
			continue
		}
		w, h = w+cw, math.Max(h, ch)
	}
	gaps := math.Max(float64(len(kids)-1), 0) * num(n["gap"], 0)
	if kind == "col" {
		return w, h + gaps
	}
	return w + gaps, h
}

func grow(n Node) float64 {
	return math.Max(num(n["grow"], 0), 0)
}

func (c *context) placeCol(n Node, x, y, w, h float64) {
	kids := c.children(n)
	gap := num(n["gap"], 0)
	align := str(n["align"])
	if align == "" {
		align = "stretch"
	}
	widths, heights := make([]float64, len(kids)), make([]float64, len(kids))
	var used, weights float64
	for i, child := range kids {
		widths[i], heights[i] = c.size(child, "col")
		used += heights[i]
		weights += grow(child)
	}
	leftover := math.Max(h-used-math.Max(float64(len(kids)-1), 0)*gap, 0)

	cy := y
	for i, child := range kids {
		ch := heights[i]
		if weights > 0 {
			ch += leftover * grow(child) / weights
		}
		cw, cx := w, x
		stretches := align == "stretch" || kindOf(child) == "bar" || kindOf(child) == "rule"
		if !stretches {
			cw = math.Min(widths[i], w)
			if align == "center" {
				cx = x + (w-cw)/2
			} else if align == "end" {
				cx = x + w - cw
			}
		}
		c.place(child, cx, cy, cw, ch)
		cy += ch + gap
	}
}

func (c *context) placeRow(n Node, x, y, w, h float64) {
	kids := c.children(n)
	gap := num(n["gap"], 0)
	align := str(n["align"])
	if align == "" {
		align = "center"
	}
	widths, heights := make([]float64, len(kids)), make([]float64, len(kids))
	var used, weights float64
	for i, child := range kids {
		widths[i], heights[i] = c.size(child, "row")
		if grow(child) > 0 {
			widths[i] = 0
		}
		used += widths[i]
		weights += grow(child)
	}
	leftover := math.Max(w-used-math.Max(float64(len(kids)-1), 0)*gap, 0)

	cx := x
	for i, child := range kids {
		cw := widths[i]
		if weights > 0 {
			cw += leftover * grow(child) / weights
		}
		ch, cy := h, y
		if kindOf(child) != "col" && kindOf(child) != "rule" {
			ch = heights[i]
			if align == "center" {
				cy = y + (h-ch)/2
			} else if align == "end" {
				cy = y + h - ch
			}
		}
		c.place(child, cx, cy, cw, ch)
		cx += cw + gap
	}
}

func (c *context) emit(n Node, x, y, w, h float64) {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	w0, h0 := int(math.Floor(w)), int(math.Floor(h))
	left, top := max(x0, c.root.Min.X), max(y0, c.root.Min.Y)
	right, bottom := min(x0+w0, c.root.Max.X), min(y0+h0, c.root.Max.Y)
	if right <= left || bottom <= top {
		return
	}
	kind := kindOf(n)
	// A line of text cannot be cut: it would be drawn whole, past the widget.
	if textual[kind] && float64(bottom-top) < textHeight(num(n["size"], 20)) {
		return
	}
	b := Box{Node: n, X: left, Y: top, W: right - left, H: bottom - top}
	if textual[kind] {
		b.Text, b.Textual = c.text(n), true
	}
	c.boxes = append(c.boxes, b)
}

func (c *context) place(n Node, x, y, w, h float64) {
	switch kindOf(n) {
	case "col":
		c.placeCol(n, x, y, w, h)
	case "row":
		c.placeRow(n, x, y, w, h)
	case "space":
	default:
		c.emit(n, x, y, w, h)
	}
}

// Layout returns the drawable boxes of a tree laid out in box, in paint order.
func Layout(root any, box image.Rectangle, measure Measure, now time.Time) []Box {
	n, ok := asNode(root)
	if !ok {
		return nil
	}
	c := &context{root: box, measure: measure, now: now}
	c.place(Resolve(n, now), float64(box.Min.X), float64(box.Min.Y), float64(box.Dx()), float64(box.Dy()))
	return c.boxes
}

// Find returns the node with the given id anywhere in the tree, or nil.
func Find(root any, id string) Node {
	n, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	if str(n["id"]) == id && id != "" {
		return n
	}
	list, _ := n["children"].([]any)
	for _, child := range list {
		if found := Find(child, id); found != nil {
			return found
		}
	}
	return nil
}
