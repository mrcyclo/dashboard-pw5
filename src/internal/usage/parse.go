package usage

import (
	"encoding/json"
	"errors"
	"math"
	"time"
)

type Window struct {
	UsedPercent int   `json:"used_percent"`
	ResetsAt    int64 `json:"resets_at,omitempty"`
}

type ModelWindow struct {
	Name        string `json:"name"`
	UsedPercent int    `json:"used_percent"`
	ResetsAt    int64  `json:"resets_at,omitempty"`
}

type Snapshot struct {
	FetchedAt int64   `json:"fetched_at,omitempty"`
	Stale     bool    `json:"stale"`
	Error     string  `json:"error,omitempty"`
	FiveHour  *Window `json:"five_hour,omitempty"`
	SevenDay  *Window `json:"seven_day,omitempty"`
	// Weekly limits that apply to a single model, in the order the API lists them.
	Models []ModelWindow `json:"models,omitempty"`
}

func (s Snapshot) HasData() bool {
	return s.FiveHour != nil || s.SevenDay != nil || len(s.Models) > 0
}

var errNoWindows = errors.New("usage: response has no usage windows")

type apiWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

func Parse(body []byte) (Snapshot, error) {
	var raw struct {
		FiveHour *apiWindow `json:"five_hour"`
		SevenDay *apiWindow `json:"seven_day"`
		// Raw so that a change in this newer, richer list cannot break the windows above.
		Limits json.RawMessage `json:"limits"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{FiveHour: toWindow(raw.FiveHour), SevenDay: toWindow(raw.SevenDay)}
	if s.FiveHour == nil && s.SevenDay == nil {
		return Snapshot{}, errNoWindows
	}
	s.Models = modelWindows(raw.Limits)
	return s, nil
}

func modelWindows(raw json.RawMessage) []ModelWindow {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	var out []ModelWindow
	for _, entry := range entries {
		var limit struct {
			Kind     string   `json:"kind"`
			Percent  *float64 `json:"percent"`
			ResetsAt *string  `json:"resets_at"`
			Scope    *struct {
				Model *struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			} `json:"scope"`
		}
		if json.Unmarshal(entry, &limit) != nil {
			continue
		}
		if limit.Kind != "weekly_scoped" || limit.Scope == nil || limit.Scope.Model == nil {
			continue
		}
		window := toWindow(&apiWindow{Utilization: limit.Percent, ResetsAt: limit.ResetsAt})
		if window == nil || limit.Scope.Model.DisplayName == "" {
			continue
		}
		out = append(out, ModelWindow{
			Name:        limit.Scope.Model.DisplayName,
			UsedPercent: window.UsedPercent,
			ResetsAt:    window.ResetsAt,
		})
	}
	return out
}

func toWindow(w *apiWindow) *Window {
	if w == nil || w.Utilization == nil {
		return nil
	}
	percent := int(math.Round(*w.Utilization))
	out := &Window{UsedPercent: min(max(percent, 0), 100)}
	if w.ResetsAt == nil {
		return out
	}
	t, err := time.Parse(time.RFC3339Nano, *w.ResetsAt)
	if err != nil {
		return out
	}
	out.ResetsAt = t.Round(time.Second).Unix()
	return out
}
