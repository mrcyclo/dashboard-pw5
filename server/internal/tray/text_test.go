package tray

import (
	"testing"

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

func TestTooltipWithModelLimits(t *testing.T) {
	s := usage.Snapshot{
		SevenDay: &usage.Window{UsedPercent: 28},
		Models:   []usage.ModelWindow{{Name: "Fable", UsedPercent: 6}},
	}
	if got := Tooltip(s); got != "Claude: week 28%, Fable 6%" {
		t.Fatalf("got %q", got)
	}
}
