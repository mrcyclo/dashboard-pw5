package widget

import (
	"fmt"
	"time"

	"claudedash/internal/usage"
)

const (
	maxLimits = 4
	// The API reports limits that reset together a second or so apart.
	sameMomentSeconds = 120

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
	// Next to a five-hour limit, "All models" alone would not say which window it is.
	weekly := "All models"
	if s.FiveHour != nil {
		weekly = "Weekly"
	}
	if s.SevenDay != nil {
		out = append(out, newLimit(weekly, s.SevenDay.UsedPercent, s.SevenDay.ResetsAt, now))
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

func usedText(l limit, size int) Node {
	n := Text(fmt.Sprintf("%d%% used", l.percent), size).Bold()
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
	return Countdown(l.resetsAt, claudeCellSize), true
}

// sharedReset is the reset time every limit shares, or 0.
func sharedReset(list []limit) int64 {
	first := list[0].resetsAt
	for _, l := range list {
		diff := l.resetsAt - first
		if l.resetsAt == 0 || diff <= -sameMomentSeconds || diff >= sameMomentSeconds {
			return 0
		}
	}
	return first
}

// wideCell is a column of the 6x1 size: label, its own countdown when the
// limits do not reset together, and "N% used" on one line, the bar below.
func wideCell(l limit, ownCountdown bool, size int) Node {
	line := []Node{Text(l.label, size).Bold().Grow(1)}
	if c, ok := countdown(l); ok && ownCountdown {
		line = append(line, c)
	}
	line = append(line, usedText(l, size))
	return Col(4, Row(8, line...), limitBar(l)).Grow(1)
}

// tallRow is a line of the 3x2 size: label, countdown and "N% used", then the bar.
func tallRow(l limit) Node {
	line := []Node{Text(l.label, claudeCellSize).Bold().Grow(1)}
	if c, ok := countdown(l); ok {
		line = append(line, c)
	}
	line = append(line, usedText(l, claudeCellSize))
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
		return wide(d.Usage, list, stale)
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

// wide is the original look: a titled header with the shared reset time on the
// right, a rule, then the limits side by side.
func wide(s usage.Snapshot, list []limit, stale Node) Node {
	title := "Claude · weekly limits"
	if s.FiveHour != nil {
		title = "Claude · usage limits"
	}
	header := []Node{Text(title, claudeTitleSize).Bold().Grow(1)}
	if stale != nil {
		header = append(header, stale)
	}
	shared := sharedReset(list)
	if shared > 0 {
		header = append(header, Countdown(shared, claudeCellSize))
	}
	// Four limits in a row only fit with smaller text.
	size := claudeTitleSize
	if len(list) > 2 {
		size = claudeCellSize
	}
	var cells []Node
	for _, l := range list {
		cells = append(cells, wideCell(l, shared == 0, size))
	}
	return Col(6, Row(12, header...), Rule(), Row(16, cells...).Align("start"))
}
