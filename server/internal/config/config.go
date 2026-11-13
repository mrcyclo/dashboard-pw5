package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"claudedash/internal/grid"
)

var (
	// Asking Claude for usage more often than every five minutes got the account rate limited.
	UsageIntervals   = []int{300, 600, 900, 1800}
	MediaIntervals   = []int{3, 5, 10, 30}
	FullRefreshTimes = []int{0, 15, 30, 60}
)

const (
	MinPort = 1024
	MaxPort = 65535

	MinZoom = 1.0
	MaxZoom = 5.0

	// LightUnset leaves the Kindle's own front light setting alone.
	LightUnset = -1
	// The Paperwhite 5 has levels 0 to 24 for both the light and its warmth.
	MaxLight = 24
	// Earlier versions stored these as percentages.
	legacyMaxLight = 100
)

// Display holds what the Kindle fetches from the PC to decide how to draw.
type Display struct {
	PanelLighten         float64 `json:"panel_lighten"`
	FullRefreshMinutes   int     `json:"full_refresh_minutes"`
	MediaIntervalSeconds int     `json:"media_interval_seconds"`
	// Levels of the Kindle's own scale, or LightUnset.
	Frontlight int `json:"frontlight"`
	Warmth     int `json:"warmth"`
}

type Background struct {
	Path string `json:"path,omitempty"`
	// Which part of the picture stays visible when it is cropped: 0 is the left
	// or top edge, 1 the right or bottom edge.
	FocusX float64 `json:"focus_x"`
	FocusY float64 `json:"focus_y"`
	// 1 shows as much of the picture as fits; 2 shows half of that in each direction.
	Zoom float64 `json:"zoom"`
}

type Config struct {
	Port         int        `json:"port"`
	CacheSeconds int        `json:"cache_seconds"`
	CloseToTray  bool       `json:"close_to_tray"`
	Display      Display    `json:"display"`
	Background   Background `json:"background"`
	// Where each widget kind sits on the Kindle; see grid.Placement.
	Layout []grid.Placement `json:"layout"`
	// Per widget kind, the options it offers (such as "align") and their chosen values.
	WidgetOptions map[string]map[string]string `json:"widget_options,omitempty"`
}

func Default() Config {
	return Config{
		Port:         8377,
		CacheSeconds: 300,
		// This is a background service for the Kindle, so closing the window should not stop it.
		CloseToTray: true,
		Display: Display{
			PanelLighten:         0.7,
			FullRefreshMinutes:   0,
			MediaIntervalSeconds: 5,
			Frontlight:           LightUnset,
			Warmth:               LightUnset,
		},
		Background: Background{FocusX: 0.5, FocusY: 0.5, Zoom: MinZoom},
		Layout:     DefaultLayout(),
	}
}

// DefaultLayout is close to the dashboard as it looked before widgets.
func DefaultLayout() []grid.Placement {
	return []grid.Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "6x1", Col: 0, Row: 1},
		{Kind: "media", Size: "6x2", Col: 0, Row: 6},
	}
}

// ValidLayout checks the shape only: kinds are checked against the widget catalog elsewhere.
func ValidLayout(p []grid.Placement) bool {
	seen := map[string]bool{}
	for _, item := range p {
		if item.Kind == "" || seen[item.Kind] {
			return false
		}
		seen[item.Kind] = true
		if item.Size == grid.None {
			continue
		}
		size, ok := grid.ParseSize(item.Size)
		if !ok || !grid.Fits(p, item.Kind, size, item.Col, item.Row) {
			return false
		}
	}
	return true
}

// Hash changes whenever a display setting changes; the Kindle watches it to
// know when to fetch the settings again.
func (d Display) Hash() string {
	text := fmt.Sprintf(
		"%.4f|%d|%d|%d|%d",
		d.PanelLighten,
		d.FullRefreshMinutes,
		d.MediaIntervalSeconds,
		d.Frontlight,
		d.Warmth,
	)
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

func (c Config) Validate() error {
	if c.Port < MinPort || c.Port > MaxPort {
		return fmt.Errorf("config: port %d must be between %d and %d", c.Port, MinPort, MaxPort)
	}
	if !slices.Contains(UsageIntervals, c.CacheSeconds) {
		return fmt.Errorf("config: cache_seconds %d is not one of %v", c.CacheSeconds, UsageIntervals)
	}
	if c.Display.PanelLighten < 0 || c.Display.PanelLighten > 1 {
		return fmt.Errorf("config: panel_lighten %v must be between 0 and 1", c.Display.PanelLighten)
	}
	if !slices.Contains(FullRefreshTimes, c.Display.FullRefreshMinutes) {
		return fmt.Errorf("config: full_refresh_minutes %d is not one of %v", c.Display.FullRefreshMinutes, FullRefreshTimes)
	}
	if !slices.Contains(MediaIntervals, c.Display.MediaIntervalSeconds) {
		return fmt.Errorf("config: media_interval_seconds %d is not one of %v", c.Display.MediaIntervalSeconds, MediaIntervals)
	}
	if !validLight(c.Display.Frontlight) || !validLight(c.Display.Warmth) {
		return fmt.Errorf("config: frontlight and warmth must be %d or between 0 and %d", LightUnset, MaxLight)
	}
	if !inUnit(c.Background.FocusX) || !inUnit(c.Background.FocusY) {
		return errors.New("config: background focus must be between 0 and 1")
	}
	if !ValidLayout(c.Layout) {
		return errors.New("config: widget layout is not valid")
	}
	if c.Background.Zoom < MinZoom || c.Background.Zoom > MaxZoom {
		return fmt.Errorf("config: background zoom %v must be between %v and %v", c.Background.Zoom, MinZoom, MaxZoom)
	}
	return nil
}

func validLight(v int) bool {
	return v >= LightUnset && v <= MaxLight
}

func inUnit(v float64) bool {
	return v >= 0 && v <= 1
}

// Load returns the defaults alongside any error so callers can keep running.
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Default(), err
	}
	// Windows editors often save UTF-8 with a byte order mark, which JSON does not allow.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &c); err != nil {
		return Default(), fmt.Errorf("config: %w", err)
	}
	c.CacheSeconds = snapUp(c.CacheSeconds, UsageIntervals)
	if c.Background.Zoom == 0 {
		c.Background.Zoom = MinZoom
	}
	// A broken layout costs only the layout, not the rest of the settings.
	if c.Layout == nil || !ValidLayout(c.Layout) {
		c.Layout = DefaultLayout()
	}
	c.Display.Frontlight = levelFromLegacy(c.Display.Frontlight)
	c.Display.Warmth = levelFromLegacy(c.Display.Warmth)
	if err := c.Validate(); err != nil {
		return Default(), err
	}
	return c, nil
}

// levelFromLegacy converts a value too large to be a level, which can only be
// an old percentage. Small percentages cannot be told apart and are kept as levels.
func levelFromLegacy(v int) int {
	if v <= MaxLight || v > legacyMaxLight {
		return v
	}
	return (v*MaxLight + legacyMaxLight/2) / legacyMaxLight
}

// snapUp raises a hand-written value to the nearest allowed one.
func snapUp(value int, allowed []int) int {
	for _, candidate := range allowed {
		if value <= candidate {
			return candidate
		}
	}
	return allowed[len(allowed)-1]
}

// Save refuses an invalid config and never leaves a half-written file behind.
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// MigrateBackground copies the background choice from the one-line file that
// older versions wrote into the config. It reports whether c changed. The
// caller removes the legacy file once the config has been saved, so a failed
// save does not lose the choice.
func MigrateBackground(c *Config, legacyPath string) bool {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return false
	}
	if c.Background.Path != "" {
		return false
	}
	path := strings.TrimSpace(string(data))
	if path == "" {
		return false
	}
	c.Background.Path = path
	return true
}
