package tray

import (
	"testing"
	"time"

	"claudedash/internal/usage"
)

func TestTooltipBoth(t *testing.T) {
	s := usage.Snapshot{FiveHour: &usage.Window{UsedPercent: 42}, SevenDay: &usage.Window{UsedPercent: 18}}
	if got := Tooltip(s); got != "Claude: 5h 42%, week 18%" {
		t.Fatalf("got %q", got)
	}
}

func TestTooltipOnlyFiveHour(t *testing.T) {
	if got := Tooltip(usage.Snapshot{FiveHour: &usage.Window{UsedPercent: 42}}); got != "Claude: 5h 42%" {
		t.Fatalf("got %q", got)
	}
}

func TestTooltipOnlySevenDay(t *testing.T) {
	if got := Tooltip(usage.Snapshot{SevenDay: &usage.Window{UsedPercent: 18}}); got != "Claude: week 18%" {
		t.Fatalf("got %q", got)
	}
}

func TestTooltipNoData(t *testing.T) {
	if got := Tooltip(usage.Snapshot{Stale: true, Error: "x"}); got != "Claude Dashboard" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusFresh(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	at := time.Date(2026, 11, 11, 14, 30, 0, 0, loc).Unix()
	if got := StatusLine(usage.Snapshot{FetchedAt: at}, loc); got != "Updated at 14:30" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusStale(t *testing.T) {
	s := usage.Snapshot{FetchedAt: 1794400000, Stale: true, Error: "Cannot reach Claude"}
	if got := StatusLine(s, time.UTC); got != "Error: Cannot reach Claude" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusNever(t *testing.T) {
	if got := StatusLine(usage.Snapshot{}, time.UTC); got != "No data yet" {
		t.Fatalf("got %q", got)
	}
}

func TestTooltipWithModelLimits(t *testing.T) {
	s := usage.Snapshot{
		SevenDay: &usage.Window{UsedPercent: 28},
		Models:   []usage.ModelWindow{{Name: "Fable", UsedPercent: 6}},
	}
	if got := Tooltip(s); got != "Claude: week 28%, Fable 6%" {
		t.Fatalf("got %q", got)
	}
}
