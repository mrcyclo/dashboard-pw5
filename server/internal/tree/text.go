package tree

import (
	"fmt"
	"time"
)

const minutesPerDay = 24 * 60

func ClockText(t time.Time) string {
	return t.Format("15:04")
}

// DateText is "Friday, 13/11/2026", as the Kindle has always written it.
func DateText(t time.Time) string {
	return t.Format("Monday, 02/01/2006")
}

// CountdownText is "resets in 2h 14m" until the unix time to, or "" once it has passed.
func CountdownText(to int64, now time.Time) string {
	if to <= now.Unix() {
		return ""
	}
	minutes := (to - now.Unix() + 59) / 60
	if minutes < 60 {
		return fmt.Sprintf("resets in %dm", minutes)
	}
	if minutes < minutesPerDay {
		return fmt.Sprintf("resets in %dh %dm", minutes/60, minutes%60)
	}
	return "resets " + time.Unix(to, 0).In(now.Location()).Format("Mon 15:04")
}
