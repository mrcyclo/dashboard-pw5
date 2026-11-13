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
	claudeNoteSize  = 25
	claudeBarHeight = 16
)

type claudeKind struct{}

func (claudeKind) Name() string    { return "claude" }
func (claudeKind) Label() string   { return "Claude usage" }
func (claudeKind) Icon() string    { return "chart-column" }
func (claudeKind) Sizes() []string { return []string{"6x2", "3x3"} }

type limit struct {
	label    string
	percent  int
	resetsAt int64
}

type limitSection struct {
	title  string
	limits []limit
}

func newLimit(label string, used int, resetsAt, now int64) limit {
	if resetsAt > 0 && resetsAt <= now {
		return limit{label: label}
	}
	return limit{label: label, percent: used, resetsAt: resetsAt}
}

// sections keeps at most maxLimits bars: five-hour first, then weekly, then per model.
func sections(s usage.Snapshot, now int64) []limitSection {
	var out []limitSection
	left := maxLimits
	if s.FiveHour != nil {
		out = append(out, limitSection{
			title:  "Claude · 5-hour limit",
			limits: []limit{newLimit("All models", s.FiveHour.UsedPercent, s.FiveHour.ResetsAt, now)},
		})
		left--
	}
	var weekly []limit
	if s.SevenDay != nil && left > 0 {
		weekly = append(weekly, newLimit("All models", s.SevenDay.UsedPercent, s.SevenDay.ResetsAt, now))
		left--
	}
	for _, m := range s.Models {
		if left == 0 {
			break
		}
		if m.Name == "" {
			continue
		}
		weekly = append(weekly, newLimit(m.Name+" only", m.UsedPercent, m.ResetsAt, now))
		left--
	}
	if len(weekly) > 0 {
		out = append(out, limitSection{title: "Claude · weekly limits", limits: weekly})
	}
	return out
}

// sharedReset is the reset time every limit of a section shares, or 0.
func sharedReset(limits []limit) int64 {
	first := limits[0].resetsAt
	for _, l := range limits {
		diff := l.resetsAt - first
		if l.resetsAt == 0 || diff <= -sameMomentSeconds || diff >= sameMomentSeconds {
			return 0
		}
	}
	return first
}

func usedText(l limit) Node {
	n := Text(fmt.Sprintf("%d%% used", l.percent), claudeTitleSize).Bold()
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

// limitCell is one limit: label, "N% used" and a bar. A countdown of its own
// shares the label's line in the wide size and gets a line below in the tall one.
func limitCell(l limit, ownCountdown, tall bool) Node {
	countdown := ownCountdown && l.resetsAt > 0
	line := []Node{Text(l.label, claudeTitleSize).Bold().Grow(1)}
	if countdown && !tall {
		line = append(line, Countdown(l.resetsAt, claudeNoteSize).Dim())
	}
	line = append(line, usedText(l))
	cell := []Node{Row(8, line...), limitBar(l)}
	if countdown && tall {
		cell = append(cell, Countdown(l.resetsAt, claudeNoteSize).Dim())
	}
	return Col(4, cell...)
}

func (claudeKind) Build(size string, d Data) Node {
	now := d.Now.Unix()
	list := sections(d.Usage, now)
	if len(list) == 0 {
		message := d.Usage.Error
		if message == "" {
			message = "No data yet"
		}
		return Col(6, Text("Claude", claudeTitleSize).Bold(), Text(message, claudeTitleSize))
	}

	tall := size == "3x3"
	// Old numbers are marked in the first header, so the mark costs no height.
	var stale Node
	if d.Usage.Stale && d.Usage.FetchedAt > 0 {
		at := time.Unix(d.Usage.FetchedAt, 0).In(d.Now.Location()).Format("15:04")
		stale = Text("Updated "+at+" (stale)", claudeNoteSize).Dim()
	}
	var blocks []Node
	for i, section := range list {
		shared := sharedReset(section.limits)
		var notes []Node
		if i == 0 && stale != nil {
			notes = append(notes, stale)
		}
		if shared > 0 {
			notes = append(notes, Countdown(shared, claudeNoteSize).Dim())
		}
		header := Row(12, append([]Node{Text(section.title, claudeTitleSize).Bold().Grow(1)}, notes...)...)
		if tall {
			header = Col(2, Text(section.title, claudeTitleSize).Bold())
			if len(notes) > 0 {
				header["children"] = append(header["children"].([]Node), Row(12, notes...).Align("start"))
			}
		}
		block := append([]Node{header, Rule()}, limitRows(section.limits, shared == 0, tall)...)
		blocks = append(blocks, Col(6, block...))
	}
	return Col(12, blocks...)
}

// limitRows puts two limits on a line in the wide size and one per line in the tall size.
func limitRows(limits []limit, ownCountdown, tall bool) []Node {
	var rows []Node
	for i := 0; i < len(limits); {
		if tall || i+1 == len(limits) {
			rows = append(rows, limitCell(limits[i], ownCountdown, tall))
			i++
			continue
		}
		rows = append(rows, Row(14,
			limitCell(limits[i], ownCountdown, tall).Grow(1),
			limitCell(limits[i+1], ownCountdown, tall).Grow(1),
		).Align("start"))
		i += 2
	}
	return rows
}
