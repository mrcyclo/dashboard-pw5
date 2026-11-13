package desk

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"claudedash/internal/applog"
	"claudedash/internal/background"
	"claudedash/internal/config"
	"claudedash/internal/grid"
	"claudedash/internal/media"
	"claudedash/internal/usage"
	"claudedash/internal/widget"
)

var now = time.Date(2026, 11, 11, 14, 30, 0, 0, time.UTC)

type fakeUsage struct {
	snapshot  usage.Snapshot
	refreshed int
}

func (f *fakeUsage) Latest() usage.Snapshot { return f.snapshot }

func (f *fakeUsage) Refresh(context.Context) usage.Snapshot {
	f.refreshed++
	return f.snapshot
}

type fakeMedia struct{ state media.State }

func (f *fakeMedia) Snapshot() media.State { return f.state }

type fakeClients struct {
	ip string
	at time.Time
	ok bool
}

func (f *fakeClients) LastClient() (string, time.Time, bool) { return f.ip, f.at, f.ok }

type fakeAutostart struct {
	on       bool
	enables  int
	disables int
	err      error
}

func (f *fakeAutostart) Enabled() (bool, error) { return f.on, nil }

func (f *fakeAutostart) Enable() error {
	f.enables++
	if f.err == nil {
		f.on = true
	}
	return f.err
}

func (f *fakeAutostart) Disable() error {
	f.disables++
	if f.err == nil {
		f.on = false
	}
	return f.err
}

type harness struct {
	app        *App
	dir        string
	configPath string
	usage      *fakeUsage
	media      *fakeMedia
	clients    *fakeClients
	autostart  *fakeAutostart
	background *background.Store
	log        *applog.Log

	tokenExpiry time.Time
	tokenErr    error
	rebinds     []int
	rebindErr   map[int]error
	intervals   []int
	pick        string
	pickErr     error
	opened      []string
	commands    []string
}

func newHarness(t *testing.T) *harness {
	h := &harness{
		dir:         t.TempDir(),
		usage:       &fakeUsage{snapshot: usage.Snapshot{FetchedAt: now.Unix() - 60, SevenDay: &usage.Window{UsedPercent: 29}}},
		media:       &fakeMedia{state: media.State{Active: true, Title: "So Am I"}},
		clients:     &fakeClients{},
		autostart:   &fakeAutostart{},
		background:  background.New("", 0.5, 0.5),
		log:         applog.New(nil, 50),
		tokenExpiry: now.Add(3 * time.Hour),
		rebindErr:   map[int]error{},
	}
	h.configPath = filepath.Join(h.dir, "config.json")
	h.app = New(Deps{
		ConfigPath: h.configPath,
		Config:     config.Default(),
		LogPath:    filepath.Join(h.dir, "claudedash.log"),
		Usage:      h.usage,
		Media:      h.media,
		Clients:    h.clients,
		Background: h.background,
		Widgets: widget.Default(func(cmd string) (media.State, error) {
			h.commands = append(h.commands, cmd)
			return media.State{}, nil
		}),
		Log:       h.log,
		Autostart: h.autostart,
		TokenExpiry: func() (time.Time, error) {
			return h.tokenExpiry, h.tokenErr
		},
		Rebind: func(port int) error {
			h.rebinds = append(h.rebinds, port)
			return h.rebindErr[port]
		},
		SetUsageInterval: func(seconds int) { h.intervals = append(h.intervals, seconds) },
		PickFile:         func() (string, error) { return h.pick, h.pickErr },
		OpenPath: func(path string) error {
			h.opened = append(h.opened, path)
			return nil
		},
		LanIP: func() string { return "10.0.0.225" },
		Now:   func() time.Time { return now },
	})
	return h
}

func (h *harness) saved(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load(h.configPath)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func picture(t *testing.T, dir, name string) string {
	t.Helper()
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 40, 30)))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGetStateAssemblesEverything(t *testing.T) {
	h := newHarness(t)
	h.autostart.on = true
	s := h.app.GetState()
	if s.Usage.SevenDay == nil || s.Usage.SevenDay.UsedPercent != 29 {
		t.Errorf("usage %+v", s.Usage)
	}
	if s.Media.Title != "So Am I" {
		t.Errorf("media %+v", s.Media)
	}
	if s.Address != "http://10.0.0.225:8377" || s.Now != now.Unix() {
		t.Errorf("address %q now %d", s.Address, s.Now)
	}
	want := Settings{Port: 8377, UsageIntervalSeconds: 300, Autostart: true, CloseToTray: true}
	if s.Settings != want {
		t.Errorf("settings %+v", s.Settings)
	}
	if s.Display != config.Default().Display {
		t.Errorf("display %+v", s.Display)
	}
	if s.Background.Set || s.Background.FocusX != 0.5 || s.Background.FocusY != 0.5 {
		t.Errorf("background %+v", s.Background)
	}
}

func TestTokenStates(t *testing.T) {
	h := newHarness(t)
	if got := h.app.GetState().Token; got.Status != TokenValid || got.ExpiresAt != h.tokenExpiry.Unix() {
		t.Errorf("valid: %+v", got)
	}
	h.tokenExpiry = now.Add(-time.Minute)
	if got := h.app.GetState().Token; got.Status != TokenExpired {
		t.Errorf("expired: %+v", got)
	}
	h.tokenErr = errors.New("unreadable")
	if got := h.app.GetState().Token; got.Status != TokenUnknown || got.ExpiresAt != 0 {
		t.Errorf("unknown: %+v", got)
	}
	h.tokenErr = nil
	h.usage.snapshot = usage.Snapshot{Stale: true, Error: usage.MessageLoginRequired}
	if got := h.app.GetState().Token; got.Status != TokenLoginRequired {
		t.Errorf("login required: %+v", got)
	}
}

func TestKindleStates(t *testing.T) {
	h := newHarness(t)
	if got := h.app.GetState().Kindle; got.Seen || got.Connected || got.IP != "" {
		t.Errorf("never seen: %+v", got)
	}
	h.clients.ip, h.clients.ok = "10.0.0.114", true
	h.clients.at = now.Add(-44 * time.Second)
	if got := h.app.GetState().Kindle; !got.Seen || !got.Connected || got.IP != "10.0.0.114" || got.LastSeen != h.clients.at.Unix() {
		t.Errorf("recent: %+v", got)
	}
	h.clients.at = now.Add(-45 * time.Second)
	if got := h.app.GetState().Kindle; !got.Seen || got.Connected {
		t.Errorf("silent for 45s should be disconnected: %+v", got)
	}
}

func TestWithoutMediaOrAutostart(t *testing.T) {
	h := newHarness(t)
	h.app.deps.Media, h.app.deps.Autostart = nil, nil
	s := h.app.GetState()
	if s.Media.Active || s.Settings.Autostart {
		t.Fatalf("got %+v", s)
	}
	if _, err := h.app.SaveSettings(Settings{Port: 8377, UsageIntervalSeconds: 300, Autostart: true, CloseToTray: true}); err != nil {
		t.Fatalf("saving without autostart support should still work: %v", err)
	}
}

func TestRefreshUsage(t *testing.T) {
	h := newHarness(t)
	h.app.RefreshUsage()
	if h.usage.refreshed != 1 {
		t.Fatalf("refreshed %d", h.usage.refreshed)
	}
}

func TestSaveDisplay(t *testing.T) {
	h := newHarness(t)
	before := h.app.Display().Hash()
	want := config.Display{PanelLighten: 0.85, FullRefreshMinutes: 30, MediaIntervalSeconds: 10}
	s, err := h.app.SaveDisplay(want)
	if err != nil || s.Display != want || h.app.Display() != want {
		t.Fatalf("state %+v err %v", s.Display, err)
	}
	if h.app.Display().Hash() == before {
		t.Fatal("the Kindle watches this hash; it must change")
	}
	if h.saved(t).Display != want {
		t.Fatal("not written to disk")
	}
	if entries := h.log.Entries(); len(entries) != 1 || entries[0].Category != applog.Kindle {
		t.Fatalf("log %+v", entries)
	}
}

func TestSaveDisplayRejectsInvalid(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []config.Display{
		{PanelLighten: 1.5, FullRefreshMinutes: 0, MediaIntervalSeconds: 5},
		{PanelLighten: 0.7, FullRefreshMinutes: 7, MediaIntervalSeconds: 5},
		{PanelLighten: 0.7, FullRefreshMinutes: 0, MediaIntervalSeconds: 1},
	} {
		s, err := h.app.SaveDisplay(bad)
		if err == nil || s.Display != config.Default().Display {
			t.Errorf("%+v: state %+v err %v", bad, s.Display, err)
		}
	}
	if _, err := os.Stat(h.configPath); !os.IsNotExist(err) {
		t.Fatal("nothing should have been written")
	}
	if len(h.log.Entries()) != 0 {
		t.Fatal("a rejected change is not logged as a change")
	}
}

func TestSaveSettings(t *testing.T) {
	h := newHarness(t)
	s, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 900, Autostart: true, CloseToTray: false})
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Port: 9000, UsageIntervalSeconds: 900, Autostart: true, CloseToTray: false}
	if s.Settings != want || s.Address != "http://10.0.0.225:9000" {
		t.Fatalf("got %+v %q", s.Settings, s.Address)
	}
	if len(h.rebinds) != 1 || h.rebinds[0] != 9000 {
		t.Fatalf("rebinds %v", h.rebinds)
	}
	if len(h.intervals) != 1 || h.intervals[0] != 900 {
		t.Fatalf("intervals %v", h.intervals)
	}
	if h.autostart.enables != 1 || h.app.CloseToTray() {
		t.Fatalf("autostart %+v closeToTray %v", h.autostart, h.app.CloseToTray())
	}
	saved := h.saved(t)
	if saved.Port != 9000 || saved.CacheSeconds != 900 || saved.CloseToTray {
		t.Fatalf("saved %+v", saved)
	}
}

func TestSaveSettingsWithoutChangesTouchesNothingExternal(t *testing.T) {
	h := newHarness(t)
	if _, err := h.app.SaveSettings(Settings{Port: 8377, UsageIntervalSeconds: 300, Autostart: false, CloseToTray: true}); err != nil {
		t.Fatal(err)
	}
	if len(h.rebinds) != 0 || len(h.intervals) != 0 || h.autostart.enables+h.autostart.disables != 0 {
		t.Fatalf("rebinds %v intervals %v autostart %+v", h.rebinds, h.intervals, h.autostart)
	}
}

func TestSaveSettingsPortInUseKeepsEverything(t *testing.T) {
	h := newHarness(t)
	h.rebindErr[9000] = errors.New("address already in use")
	s, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 900, Autostart: true, CloseToTray: false})
	if err == nil || !strings.Contains(err.Error(), "9000") {
		t.Fatalf("err %v", err)
	}
	want := Settings{Port: 8377, UsageIntervalSeconds: 300, Autostart: false, CloseToTray: true}
	if s.Settings != want {
		t.Fatalf("settings changed despite the failure: %+v", s.Settings)
	}
	if _, statErr := os.Stat(h.configPath); !os.IsNotExist(statErr) {
		t.Fatal("config must not be written when the port cannot be opened")
	}
	if len(h.intervals) != 0 || h.autostart.on {
		t.Fatal("no other setting may stay applied when the port change fails")
	}
}

func TestSaveSettingsRejectsInvalid(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []Settings{
		{Port: 80, UsageIntervalSeconds: 300, CloseToTray: true},
		{Port: 70000, UsageIntervalSeconds: 300, CloseToTray: true},
		{Port: 8377, UsageIntervalSeconds: 120, CloseToTray: true},
	} {
		if _, err := h.app.SaveSettings(bad); err == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
	if len(h.rebinds) != 0 {
		t.Fatalf("an invalid port must never be tried: %v", h.rebinds)
	}
}

func TestSaveSettingsAutostartFailureIsReported(t *testing.T) {
	h := newHarness(t)
	h.autostart.err = errors.New("registry locked")
	if _, err := h.app.SaveSettings(Settings{Port: 8377, UsageIntervalSeconds: 300, Autostart: true, CloseToTray: true}); err == nil {
		t.Fatal("want error")
	}
}

func TestChooseBackground(t *testing.T) {
	h := newHarness(t)
	h.background.SetFocus(0.1, 0.9)
	h.pick = picture(t, h.dir, "grünes bild.png")
	s, err := h.app.ChooseBackground()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Background.Set || s.Background.Name != "grünes bild.png" || s.Background.Hash == "" {
		t.Fatalf("got %+v", s.Background)
	}
	if s.Background.FocusX != 0.5 || s.Background.FocusY != 0.5 {
		t.Fatalf("a new picture should start centered: %+v", s.Background)
	}
	if saved := h.saved(t).Background; saved.Path != h.pick || saved.FocusX != 0.5 {
		t.Fatalf("saved %+v", saved)
	}
}

func TestChooseBackgroundCancelled(t *testing.T) {
	h := newHarness(t)
	s, err := h.app.ChooseBackground()
	if err != nil || s.Background.Set {
		t.Fatalf("got %+v err %v", s.Background, err)
	}
	if _, statErr := os.Stat(h.configPath); !os.IsNotExist(statErr) {
		t.Fatal("cancelling must not write anything")
	}
}

func TestChooseBackgroundRejectsNonPicture(t *testing.T) {
	h := newHarness(t)
	good := picture(t, h.dir, "good.png")
	h.pick = good
	h.app.ChooseBackground()

	notPicture := filepath.Join(h.dir, "notes.txt")
	os.WriteFile(notPicture, []byte("hello"), 0o600)
	h.pick = notPicture
	s, err := h.app.ChooseBackground()
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), h.dir) {
		t.Fatalf("the message is shown to the user and should not expose a full path: %v", err)
	}
	if s.Background.Name != "good.png" || h.saved(t).Background.Path != good {
		t.Fatalf("the previous picture must survive: %+v", s.Background)
	}
}

func TestSetBackgroundFocusAndClear(t *testing.T) {
	h := newHarness(t)
	h.pick = picture(t, h.dir, "a.png")
	first, _ := h.app.ChooseBackground()

	moved, err := h.app.SetBackgroundFocus(0.2, 1.7)
	if err != nil || moved.Background.FocusX != 0.2 || moved.Background.FocusY != 1 {
		t.Fatalf("got %+v err %v", moved.Background, err)
	}
	if moved.Background.Hash == first.Background.Hash {
		t.Fatal("moving the focus must change the hash")
	}
	if saved := h.saved(t).Background; saved.FocusX != 0.2 || saved.FocusY != 1 {
		t.Fatalf("saved %+v", saved)
	}

	cleared, err := h.app.ClearBackground()
	if err != nil || cleared.Background.Set || cleared.Background.Hash != "" {
		t.Fatalf("got %+v err %v", cleared.Background, err)
	}
	if saved := h.saved(t).Background; saved.Path != "" || saved.FocusX != 0.5 {
		t.Fatalf("saved %+v", saved)
	}
}

func TestLogsAndOpeners(t *testing.T) {
	h := newHarness(t)
	h.log.Add(applog.Usage, "one")
	h.log.Add(applog.Token, "two")
	if got := h.app.GetLogs(); len(got) != 2 || got[0].Message != "two" {
		t.Fatalf("got %+v", got)
	}
	h.app.OpenLogFile()
	h.app.OpenDataFolder()
	if len(h.opened) != 2 || h.opened[0] != filepath.Join(h.dir, "claudedash.log") || h.opened[1] != h.dir {
		t.Fatalf("opened %v", h.opened)
	}
}

func TestManualRefreshHasAFloor(t *testing.T) {
	h := newHarness(t)
	current := now
	h.app.deps.Now = func() time.Time { return current }
	h.app.RefreshUsage()
	current = current.Add(59 * time.Second)
	h.app.RefreshUsage()
	if h.usage.refreshed != 1 {
		t.Fatalf("a second click within a minute must not call Claude again: %d", h.usage.refreshed)
	}
	current = current.Add(2 * time.Second)
	h.app.RefreshUsage()
	if h.usage.refreshed != 2 {
		t.Fatalf("refreshed %d", h.usage.refreshed)
	}
}

func TestListeningIsReported(t *testing.T) {
	h := newHarness(t)
	if !h.app.GetState().Listening {
		t.Fatal("listening by default")
	}
	h.app.deps.Listening = func() bool { return false }
	if h.app.GetState().Listening {
		t.Fatal("a port that could not be opened must be visible to the window")
	}
}

func TestSaveSettingsRetriesAPortThatNeverOpened(t *testing.T) {
	h := newHarness(t)
	h.app.deps.Listening = func() bool { return false }
	if _, err := h.app.SaveSettings(Settings{Port: 8377, UsageIntervalSeconds: 300, CloseToTray: true}); err != nil {
		t.Fatal(err)
	}
	if len(h.rebinds) != 1 || h.rebinds[0] != 8377 {
		t.Fatalf("the unchanged port should be tried again: %v", h.rebinds)
	}
}

func TestSaveSettingsAutostartFailureChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.autostart.err = errors.New("registry locked")
	s, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 900, Autostart: true, CloseToTray: false})
	if err == nil {
		t.Fatal("want error")
	}
	if len(h.rebinds) != 0 || len(h.intervals) != 0 {
		t.Fatalf("the port and interval must not move when the save is reported as failed: %v %v", h.rebinds, h.intervals)
	}
	if s.Settings.Port != 8377 || !s.Settings.CloseToTray {
		t.Fatalf("got %+v", s.Settings)
	}
	if _, statErr := os.Stat(h.configPath); !os.IsNotExist(statErr) {
		t.Fatal("config must not be written")
	}
}

func TestSaveSettingsKeepsOtherPagesChanges(t *testing.T) {
	h := newHarness(t)
	display := config.Display{PanelLighten: 0.85, FullRefreshMinutes: 30, MediaIntervalSeconds: 10}
	h.app.SaveDisplay(display)
	h.pick = picture(t, h.dir, "a.png")
	h.app.ChooseBackground()
	if _, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 600, CloseToTray: true}); err != nil {
		t.Fatal(err)
	}
	saved := h.saved(t)
	if saved.Display != display || saved.Background.Path != h.pick || saved.Port != 9000 {
		t.Fatalf("saved %+v", saved)
	}
}

func TestSaveSettingsWriteFailureMovesThePortBack(t *testing.T) {
	h := newHarness(t)
	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	s, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 300, CloseToTray: true})
	if err == nil || s.Settings.Port != 8377 {
		t.Fatalf("settings %+v err %v", s.Settings, err)
	}
	if len(h.rebinds) != 2 || h.rebinds[0] != 9000 || h.rebinds[1] != 8377 {
		t.Fatalf("rebinds %v", h.rebinds)
	}
}

func TestSaveSettingsFollowsThePortWhenItCannotMoveBack(t *testing.T) {
	h := newHarness(t)
	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	h.rebindErr[8377] = errors.New("taken meanwhile")
	s, err := h.app.SaveSettings(Settings{Port: 9000, UsageIntervalSeconds: 300, CloseToTray: true})
	if err == nil {
		t.Fatal("want error")
	}
	if s.Settings.Port != 9000 || s.Address != "http://10.0.0.225:9000" {
		t.Fatalf("the server is on 9000, so the window must say so: %+v", s.Settings)
	}
}

func TestBackgroundChangeIsUndoneWhenItCannotBeSaved(t *testing.T) {
	h := newHarness(t)
	first := picture(t, h.dir, "first.png")
	h.pick = first
	h.app.ChooseBackground()
	h.app.SetBackgroundFocus(0.2, 0.8)

	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	h.pick = picture(t, h.dir, "second.png")
	s, err := h.app.ChooseBackground()
	if err == nil || strings.Contains(err.Error(), h.dir) {
		t.Fatalf("want a plain message without a path, got %v", err)
	}
	if s.Background.Name != "first.png" || s.Background.FocusX != 0.2 || s.Background.FocusY != 0.8 {
		t.Fatalf("the served picture must match what a restart would load: %+v", s.Background)
	}
	if s, err := h.app.SetBackgroundFocus(0.9, 0.9); err == nil || s.Background.FocusX != 0.2 {
		t.Fatalf("focus %+v err %v", s.Background, err)
	}
	if s, err := h.app.ClearBackground(); err == nil || !s.Background.Set {
		t.Fatalf("clear %+v err %v", s.Background, err)
	}
}

func TestSourceHashIgnoresFocus(t *testing.T) {
	h := newHarness(t)
	h.pick = picture(t, h.dir, "a.png")
	first, _ := h.app.ChooseBackground()
	moved, _ := h.app.SetBackgroundFocus(0.1, 0.1)
	if first.Background.SourceHash == "" || moved.Background.SourceHash != first.Background.SourceHash {
		t.Fatalf("source hash %q then %q", first.Background.SourceHash, moved.Background.SourceHash)
	}
	if moved.Background.Hash == first.Background.Hash {
		t.Fatal("the crop hash should still change")
	}
}

func TestSaveDisplayWriteFailureIsNotCalledInvalid(t *testing.T) {
	h := newHarness(t)
	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	_, err := h.app.SaveDisplay(config.Display{PanelLighten: 0.5, FullRefreshMinutes: 0, MediaIntervalSeconds: 5})
	if err == nil || strings.Contains(err.Error(), "not valid") {
		t.Fatalf("got %v", err)
	}
	if h.app.Display().PanelLighten != 0.7 {
		t.Fatal("an unsaved change must not take effect")
	}
}

func TestSetBackgroundView(t *testing.T) {
	h := newHarness(t)
	h.pick = picture(t, h.dir, "a.png")
	first, _ := h.app.ChooseBackground()
	if first.Background.Zoom != 1 {
		t.Fatalf("a new picture is not zoomed: %+v", first.Background)
	}
	s, err := h.app.SetBackgroundView(0.3, 0.7, 2.5)
	if err != nil || s.Background.Zoom != 2.5 || s.Background.FocusX != 0.3 || s.Background.FocusY != 0.7 {
		t.Fatalf("got %+v err %v", s.Background, err)
	}
	if s.Background.Hash == first.Background.Hash || s.Background.SourceHash != first.Background.SourceHash {
		t.Fatalf("crop hash must change and source hash must not: %+v", s.Background)
	}
	if saved := h.saved(t).Background; saved.Zoom != 2.5 || saved.FocusX != 0.3 {
		t.Fatalf("saved %+v", saved)
	}
	if s, _ := h.app.SetBackgroundView(0.5, 0.5, 99); s.Background.Zoom != 5 {
		t.Fatalf("zoom should be clamped: %+v", s.Background)
	}
}

func TestNewPictureResetsZoom(t *testing.T) {
	h := newHarness(t)
	h.pick = picture(t, h.dir, "a.png")
	h.app.ChooseBackground()
	h.app.SetBackgroundView(0.1, 0.1, 3)
	h.pick = picture(t, h.dir, "b.png")
	s, _ := h.app.ChooseBackground()
	if s.Background.Zoom != 1 || s.Background.FocusX != 0.5 {
		t.Fatalf("the old picture's view should not carry over: %+v", s.Background)
	}
}

func TestViewIsUndoneWhenItCannotBeSaved(t *testing.T) {
	h := newHarness(t)
	h.pick = picture(t, h.dir, "a.png")
	h.app.ChooseBackground()
	h.app.SetBackgroundView(0.2, 0.8, 2)
	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	s, err := h.app.SetBackgroundView(0.9, 0.9, 4)
	if err == nil || s.Background.Zoom != 2 || s.Background.FocusX != 0.2 {
		t.Fatalf("got %+v err %v", s.Background, err)
	}
}

func TestRefreshKindle(t *testing.T) {
	h := newHarness(t)
	if h.app.RefreshToken() != "" {
		t.Fatal("no refresh has been asked for yet")
	}
	h.app.RefreshKindle()
	first := h.app.RefreshToken()
	if first == "" {
		t.Fatal("asking for a refresh must set a token")
	}
	h.app.deps.Now = func() time.Time { return now.Add(time.Second) }
	h.app.RefreshKindle()
	if h.app.RefreshToken() == first {
		t.Fatal("each request needs its own token")
	}
}

func TestSaveDisplayLights(t *testing.T) {
	h := newHarness(t)
	d := config.Default().Display
	d.Frontlight, d.Warmth = 10, 24
	s, err := h.app.SaveDisplay(d)
	if err != nil || s.Display.Frontlight != 10 || s.Display.Warmth != 24 {
		t.Fatalf("got %+v err %v", s.Display, err)
	}
	d.Frontlight = 25
	if _, err := h.app.SaveDisplay(d); err == nil {
		t.Fatal("a light above the top level must be rejected")
	}
}

func placement(s State, kind string) grid.Placement {
	for _, p := range s.Widgets.Layout {
		if p.Kind == kind {
			return p
		}
	}
	return grid.Placement{}
}

func TestWidgetsInState(t *testing.T) {
	s := newHarness(t).app.GetState()
	if len(s.Widgets.Catalog) != 3 || s.Widgets.Catalog[1].Kind != "claude" || s.Widgets.Catalog[1].Label != "Claude usage" {
		t.Fatalf("catalog %+v", s.Widgets.Catalog)
	}
	if !reflect.DeepEqual(s.Widgets.Catalog[0].Sizes, []string{"3x1", "6x1"}) || s.Widgets.Catalog[2].Icon != "music" {
		t.Fatalf("catalog %+v", s.Widgets.Catalog)
	}
	if !reflect.DeepEqual(s.Widgets.Layout, config.DefaultLayout()) {
		t.Fatalf("layout %+v", s.Widgets.Layout)
	}
	if len(s.Widgets.Screen.Widgets) != 3 {
		t.Fatalf("screen %+v", s.Widgets.Screen)
	}
}

func TestHideRemembersAndShowReturns(t *testing.T) {
	h := newHarness(t)
	s, err := h.app.SetWidgetSize("media", "none")
	if err != nil || placement(s, "media") != (grid.Placement{Kind: "media", Size: "none", Col: 0, Row: 6}) {
		t.Fatalf("%+v %v", placement(s, "media"), err)
	}
	if len(s.Widgets.Screen.Widgets) != 2 {
		t.Fatal("a hidden widget leaves the screen")
	}
	s, err = h.app.SetWidgetSize("media", "6x1")
	if err != nil || placement(s, "media") != (grid.Placement{Kind: "media", Size: "6x1", Col: 0, Row: 6}) {
		t.Fatalf("%+v %v", placement(s, "media"), err)
	}
	if saved := h.saved(t).Layout; !reflect.DeepEqual(saved, s.Widgets.Layout) {
		t.Fatalf("saved %+v", saved)
	}
}

func TestResizeMovesToNearestFreeSpot(t *testing.T) {
	h := newHarness(t)
	s, err := h.app.SetWidgetSize("clock", "6x1")
	if err != nil || placement(s, "clock") != (grid.Placement{Kind: "clock", Size: "6x1", Col: 0, Row: 2}) {
		t.Fatalf("%+v %v", placement(s, "clock"), err)
	}
}

// bigKind needs every row below the corner, so any other widget there blocks it.
type bigKind struct{}

func (bigKind) Name() string                          { return "big" }
func (bigKind) Label() string                         { return "Big" }
func (bigKind) Icon() string                          { return "square" }
func (bigKind) Sizes() []string                       { return []string{"6x7"} }
func (bigKind) Build(string, widget.Data) widget.Node { return widget.Text("big", 20) }

func TestNoRoomLeavesEverythingAlone(t *testing.T) {
	h := newHarness(t)
	clock, _ := widget.Default(nil).Get("clock")
	cfg := config.Default()
	cfg.Layout = []grid.Placement{{Kind: "clock", Size: "3x1", Col: 0, Row: 3}}
	h.app = New(Deps{ConfigPath: h.configPath, Config: cfg, Usage: h.usage, Media: h.media, Clients: h.clients,
		Background: h.background, Log: h.log, Widgets: widget.NewCatalog(clock, bigKind{}),
		TokenExpiry: func() (time.Time, error) { return h.tokenExpiry, nil }, LanIP: func() string { return "" },
		Now: func() time.Time { return now }})
	before := h.app.GetState().Widgets.Layout

	_, err := h.app.SetWidgetSize("big", "6x7")
	if err == nil || err.Error() != "No room for this size. Move or hide another widget first" {
		t.Fatalf("err %v", err)
	}
	if !reflect.DeepEqual(h.app.GetState().Widgets.Layout, before) {
		t.Fatal("a failed resize changes nothing")
	}
	if _, err := os.Stat(h.configPath); err == nil {
		t.Fatal("a failed resize writes no config")
	}
}

func TestMoveWidget(t *testing.T) {
	h := newHarness(t)
	s, err := h.app.MoveWidget("media", 0, 3)
	if err != nil || placement(s, "media").Row != 3 {
		t.Fatalf("%+v %v", placement(s, "media"), err)
	}
	for _, spot := range [][2]int{{3, 0}, {0, 1}, {0, 7}} {
		if _, err := h.app.MoveWidget("media", spot[0], spot[1]); err == nil || err.Error() != "That spot is taken" {
			t.Errorf("%v: %v", spot, err)
		}
	}
	if _, err := h.app.MoveWidget("media", 0, 4); err != nil {
		t.Fatal(err)
	}
}

func TestMoveHiddenWidgetIsRefused(t *testing.T) {
	h := newHarness(t)
	h.app.SetWidgetSize("media", "none")
	if _, err := h.app.MoveWidget("media", 0, 3); err == nil {
		t.Fatal("a hidden widget cannot be moved")
	}
}

func TestUnknownWidgetOrSize(t *testing.T) {
	h := newHarness(t)
	for _, c := range [][2]string{{"weather", "3x1"}, {"clock", "6x2"}, {"clock", "big"}} {
		if _, err := h.app.SetWidgetSize(c[0], c[1]); err == nil || err.Error() != "Unknown widget or size" {
			t.Errorf("%v: %v", c, err)
		}
	}
}

func TestLayoutChangeRolledBackWhenSaveFails(t *testing.T) {
	h := newHarness(t)
	h.app.deps.ConfigPath = filepath.Join(h.dir, "missing-folder", "config.json")
	s, err := h.app.SetWidgetSize("media", "none")
	if err == nil || placement(s, "media").Size != "6x2" {
		t.Fatalf("%+v %v", placement(s, "media"), err)
	}
}

func TestKindleBatteryFromScreenPoll(t *testing.T) {
	h := newHarness(t)
	if h.app.GetState().Kindle.Battery != nil {
		t.Fatal("unknown until the Kindle reports it")
	}
	h.app.Screen(76, true)
	h.app.Screen(0, false)
	if b := h.app.GetState().Kindle.Battery; b == nil || *b != 76 {
		t.Fatalf("battery %v", b)
	}
}

func TestActRunsWidgetActions(t *testing.T) {
	h := newHarness(t)
	if err := h.app.Act("media.next"); err != nil || !reflect.DeepEqual(h.commands, []string{"next"}) {
		t.Fatalf("%v %v", err, h.commands)
	}
	if err := h.app.Act("media.explode"); !errors.Is(err, widget.ErrUnknownAction) {
		t.Fatal(err)
	}
}

func TestUnknownKindsAreNotListed(t *testing.T) {
	h := newHarness(t)
	cfg := config.Default()
	cfg.Layout = []grid.Placement{
		{Kind: "weather", Size: "3x1", Col: 0, Row: 3},
		{Kind: "clock", Size: "6x1", Col: 0, Row: 0 + 1},
	}
	h.app = New(Deps{ConfigPath: h.configPath, Config: cfg, Usage: h.usage, Media: h.media, Clients: h.clients,
		Background: h.background, Log: h.log, Widgets: widget.Default(nil),
		TokenExpiry: func() (time.Time, error) { return h.tokenExpiry, nil }, LanIP: func() string { return "" },
		Now: func() time.Time { return now }})
	got := h.app.GetState().Widgets.Layout
	want := []grid.Placement{
		{Kind: "clock", Size: "6x1", Col: 0, Row: 1},
		{Kind: "claude", Size: "none"},
		{Kind: "media", Size: "none"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func newHarnessWithLayout(t *testing.T, layout []grid.Placement) *harness {
	h := newHarness(t)
	cfg := config.Default()
	cfg.Layout = layout
	h.app = New(Deps{ConfigPath: h.configPath, Config: cfg, Usage: h.usage, Media: h.media, Clients: h.clients,
		Background: h.background, Log: h.log, Widgets: widget.Default(nil),
		TokenExpiry: func() (time.Time, error) { return h.tokenExpiry, nil }, LanIP: func() string { return "" },
		Now: func() time.Time { return now }})
	return h
}

func savedPlacement(t *testing.T, h *harness, kind string) (grid.Placement, bool) {
	t.Helper()
	for _, p := range h.saved(t).Layout {
		if p.Kind == kind {
			return p, true
		}
	}
	return grid.Placement{}, false
}

// A newer build's widget must survive this build saving the config.
func TestUnknownKindsSurviveASave(t *testing.T) {
	weather := grid.Placement{Kind: "weather", Size: "3x1", Col: 0, Row: 3}
	h := newHarnessWithLayout(t, append(config.DefaultLayout(), weather))
	if _, err := h.app.SetWidgetSize("media", "none"); err != nil {
		t.Fatal(err)
	}
	if got, ok := savedPlacement(t, h, "weather"); !ok || got != weather {
		t.Fatalf("weather %+v %v", got, ok)
	}
	for _, p := range h.app.GetState().Widgets.Layout {
		if p.Kind == "weather" {
			t.Fatal("the window only lists kinds it knows")
		}
	}
}

func TestUnknownKindsMakeWayWhenCovered(t *testing.T) {
	weather := grid.Placement{Kind: "weather", Size: "3x1", Col: 0, Row: 2}
	h := newHarnessWithLayout(t, append(config.DefaultLayout(), weather))
	if _, err := h.app.SetWidgetSize("clock", "6x1"); err != nil {
		t.Fatal(err)
	}
	got, ok := savedPlacement(t, h, "weather")
	if !ok || got.Size != grid.None || got.Col != 0 || got.Row != 2 {
		t.Fatalf("a covered unknown widget is kept hidden in its spot: %+v %v", got, ok)
	}
}

// A size an older build saved, and this build no longer offers, becomes the
// first size that fits in the same spot instead of hiding the widget.
func TestRetiredSizeBecomesAFittingOne(t *testing.T) {
	h := newHarnessWithLayout(t, []grid.Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "6x2", Col: 0, Row: 1},
		{Kind: "media", Size: "6x2", Col: 0, Row: 6},
	})
	if got := placement(h.app.GetState(), "claude"); got != (grid.Placement{Kind: "claude", Size: "6x1", Col: 0, Row: 1}) {
		t.Fatalf("got %+v", got)
	}
}

func TestRetiredSizeWithNoFitHides(t *testing.T) {
	h := newHarnessWithLayout(t, []grid.Placement{
		{Kind: "clock", Size: "3x1", Col: 0, Row: 0},
		{Kind: "claude", Size: "3x3", Col: 3, Row: 0},
		{Kind: "media", Size: "6x2", Col: 0, Row: 6},
	})
	if got := placement(h.app.GetState(), "claude"); got.Size != grid.None {
		t.Fatalf("no new size fits in the reserved corner: %+v", got)
	}
}
