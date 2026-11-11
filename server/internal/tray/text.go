package tray

import (
	"fmt"
	"strings"
	"time"

	"claudedash/internal/usage"
)

func Tooltip(s usage.Snapshot) string {
	var parts []string
	if s.FiveHour != nil {
		parts = append(parts, fmt.Sprintf("5h %d%%", s.FiveHour.UsedPercent))
	}
	if s.SevenDay != nil {
		parts = append(parts, fmt.Sprintf("week %d%%", s.SevenDay.UsedPercent))
	}
	for _, model := range s.Models {
		parts = append(parts, fmt.Sprintf("%s %d%%", model.Name, model.UsedPercent))
	}
	if len(parts) == 0 {
		return "Claude Dashboard"
	}
	return "Claude: " + strings.Join(parts, ", ")
}

func StatusLine(s usage.Snapshot, loc *time.Location) string {
	if s.Stale && s.Error != "" {
		return "Error: " + s.Error
	}
	if s.FetchedAt == 0 {
		return "No data yet"
	}
	return "Updated at " + time.Unix(s.FetchedAt, 0).In(loc).Format("15:04")
}
