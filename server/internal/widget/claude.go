package widget

import (
	"fmt"
	"time"

	"claudedash/internal/usage"
)

const (
	maxLimits = 4

	claudeTitleSize = 29
	claudeCellSize  = 25
	claudeBarHeight = 16
)

type claudeKind struct{}

func (claudeKind) Name() string    { return "claude" }
func (claudeKind) Label() string   { return "Claude usage" }
func (claudeKind) Icon() string    { return "chart-column" }
func (claudeKind) Sizes() []string { return []string{"6x1", "3x2"} }

type limit struct {
	label    string
	percent  int
	resetsAt int64
}

func newLimit(label string, used int, resetsAt, now int64) limit {
	if resetsAt > 0 && resetsAt <= now {
		return limit{label: label}
	}
	return limit{label: label, percent: used, resetsAt: resetsAt}
}

// limits keeps at most maxLimits bars: five-hour first, then weekly, then per model.
func limits(s usage.Snapshot, now int64) []limit {
	var out []limit
	if s.FiveHour != nil {
		out = append(out, newLimit("5-hour", s.FiveHour.UsedPercent, s.FiveHour.ResetsAt, now))
	}
	if s.SevenDay != nil {
		out = append(out, newLimit("Weekly", s.SevenDay.UsedPercent, s.SevenDay.ResetsAt, now))
	}
	for _, m := range s.Models {
		if m.Name == "" {
			continue
		}
		out = append(out, newLimit(m.Name+" only", m.UsedPercent, m.ResetsAt, now))
	}
	if len(out) > maxLimits {
		out = out[:maxLimits]
	}
	return out
}

func usedText(l limit) Node {
	n := Text(fmt.Sprintf("%d%% used", l.percent), claudeCellSize).Bold()
	if l.resetsAt > 0 {
		n.At(l.resetsAt, map[string]any{"text": "0% used"})
	}
	return n
}

func limitBar(l limit) Node {
	n := Bar(l.percent, claudeBarHeight)
	if l.resetsAt > 0 {
		n.At(l.resetsAt, map[string]any{"percent": 0})
	}
	return n
}

func countdown(l limit) (Node, bool) {
	if l.resetsAt == 0 {
		return nil, false
	}
	return Countdown(l.resetsAt, claudeCellSize).Dim(), true
}

// wideCell is a column of the 6x1 size: label and "N% used", the bar, then
// its countdown, or the stale mark in the first column.
func wideCell(l limit, note Node) Node {
	cell := []Node{Row(8, Text(l.label, claudeCellSize).Bold().Grow(1), usedText(l)), limitBar(l)}
	if note == nil {
		note, _ = countdown(l)
	}
	if note != nil {
		cell = append(cell, note)
	}
	return Col(4, cell...).Grow(1)
}

// tallRow is a line of the 3x2 size: label, countdown and "N% used", then the bar.
func tallRow(l limit) Node {
	line := []Node{Text(l.label, claudeCellSize).Bold().Grow(1)}
	if c, ok := countdown(l); ok {
		line = append(line, c)
	}
	line = append(line, usedText(l))
	return Col(4, Row(10, line...), limitBar(l))
}

func (claudeKind) Build(size string, d Data) Node {
	list := limits(d.Usage, d.Now.Unix())
	if len(list) == 0 {
		message := d.Usage.Error
		if message == "" {
			message = "No data yet"
		}
		return Col(6, Text("Claude", claudeTitleSize).Bold(), Text(message, claudeTitleSize))
	}

	var stale Node
	if d.Usage.Stale && d.Usage.FetchedAt > 0 {
		at := time.Unix(d.Usage.FetchedAt, 0).In(d.Now.Location()).Format("15:04")
		stale = Text("Updated "+at+" (stale)", claudeCellSize).Dim()
	}

	if size == "6x1" {
		var cells []Node
		for i, l := range list {
			var note Node
			if i == 0 {
				note = stale
			}
			cells = append(cells, wideCell(l, note))
		}
		return Row(16, cells...).Align("start")
	}

	header := []Node{Text("Claude", claudeTitleSize).Bold().Grow(1)}
	if stale != nil {
		header = append(header, stale)
	}
	rows := []Node{Row(12, header...), Rule()}
	for _, l := range list {
		rows = append(rows, tallRow(l))
	}
	return Col(8, rows...)
}
