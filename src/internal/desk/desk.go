// Package desk is everything the desktop window can ask for or change. It
// knows nothing about Wails, so it can be tested without a window.
package desk

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"claudedash/internal/applog"
	"claudedash/internal/background"
	"claudedash/internal/config"
	"claudedash/internal/grid"
	"claudedash/internal/media"
	"claudedash/internal/usage"
	"claudedash/internal/widget"
)

// A Kindle that has not called for this long is shown as disconnected. It
// matches the silence threshold of the HTTP client watch.
const kindleTimeout = 45 * time.Second

const minManualRefresh = time.Minute

const (
	messageSaveFailed = "Could not save the settings"
	messageNotPicture = "This file is not a readable JPG or PNG picture"
)

const (
	TokenValid         = "valid"
	TokenExpired       = "expired"
	TokenLoginRequired = "login_required"
	TokenUnknown       = "unknown"
)

type UsageSource interface {
	Latest() usage.Snapshot
	Refresh(ctx context.Context) usage.Snapshot
}

type MediaSource interface {
	Snapshot() media.State
}

type ClientSource interface {
	LastClient() (ip string, at time.Time, ok bool)
}

type Autostart interface {
	Enabled() (bool, error)
	Enable() error
	Disable() error
}

type Deps struct {
	ConfigPath string
	Config     config.Config
	LogPath    string

	Usage      UsageSource
	Media      MediaSource
	Clients    ClientSource
	Background *background.Store
	Widgets    *widget.Catalog
	Log        *applog.Log
	Autostart  Autostart

	// TokenExpiry reads when the stored access token expires.
	TokenExpiry func() (time.Time, error)
	// Rebind moves the HTTP server to another port. On failure the old one keeps serving.
	Rebind func(port int) error
	// Listening reports whether the HTTP server is currently bound to its port.
	Listening func() bool
	// SetUsageInterval changes how often usage is fetched.
	SetUsageInterval func(seconds int)
	// PickFile shows the file dialog and returns "" when the user cancels.
	PickFile func() (string, error)
	OpenPath func(path string) error
	LanIP    func() string
	Now      func() time.Time
}

type TokenState struct {
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type KindleState struct {
	Seen      bool   `json:"seen"`
	Connected bool   `json:"connected"`
	IP        string `json:"ip,omitempty"`
	LastSeen  int64  `json:"last_seen,omitempty"`
	// Battery is the charge the Kindle reported with its last screen poll.
	Battery *int `json:"battery,omitempty"`
}

type WidgetKind struct {
	Kind    string          `json:"kind"`
	Label   string          `json:"label"`
	Icon    string          `json:"icon"`
	Sizes   []string        `json:"sizes"`
	Options []widget.Option `json:"options"`
}

type WidgetsState struct {
	Catalog []WidgetKind `json:"catalog"`
	// Layout has one placement per catalog kind, in catalog order; hidden ones have size "none".
	Layout []grid.Placement `json:"layout"`
	// Options has every kind's options with their current values, defaults filled in.
	Options map[string]map[string]string `json:"options"`
	// Lighten has every kind's panel lightening, 0 to 1, defaults filled in.
	Lighten map[string]float64 `json:"lighten"`
	// Screen is exactly what the Kindle is sent, for the window's preview.
	Screen widget.Screen `json:"screen"`
}

type BackgroundState struct {
	Set  bool   `json:"set"`
	Name string `json:"name,omitempty"`
	Hash string `json:"hash,omitempty"`
	// SourceHash identifies the file alone and does not change when the focus moves.
	SourceHash string  `json:"source_hash,omitempty"`
	FocusX     float64 `json:"focus_x"`
	FocusY     float64 `json:"focus_y"`
	Zoom       float64 `json:"zoom"`
}

type Settings struct {
	Port                 int  `json:"port"`
	UsageIntervalSeconds int  `json:"usage_interval_seconds"`
	Autostart            bool `json:"autostart"`
	CloseToTray          bool `json:"close_to_tray"`
}

type State struct {
	Usage      usage.Snapshot  `json:"usage"`
	Token      TokenState      `json:"token"`
	Media      media.State     `json:"media"`
	Kindle     KindleState     `json:"kindle"`
	Background BackgroundState `json:"background"`
	Display    config.Display  `json:"display"`
	Settings   Settings        `json:"settings"`
	Widgets    WidgetsState    `json:"widgets"`
	// Address is what goes into the Kindle's config: http://<LAN IP>:<port>.
	Address string `json:"address"`
	// Listening is false when the port could not be opened, so nothing can reach this PC.
	Listening bool  `json:"listening"`
	Now       int64 `json:"now"`
}

type App struct {
	deps Deps

	// saving serializes SaveSettings and layout changes.
	saving sync.Mutex

	mu          sync.Mutex
	config      config.Config
	lastRefresh time.Time
	// Changes each time the user asks the Kindle to repaint; "" until the first time.
	refreshToken string
	battery      *int
}

func New(d Deps) *App {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Widgets == nil {
		d.Widgets = widget.Default(nil)
	}
	cfg := d.Config
	cfg.Layout = append(fitCatalog(d.Widgets, cfg.Layout), unknownKinds(d.Widgets, cfg.Layout)...)
	return &App{deps: d, config: cfg}
}

// retiredSize replaces a size this build no longer offers with the first one
// that fits in the same spot, or hides the widget when none does.
func retiredSize(k widget.Kind, layout []grid.Placement, saved grid.Placement) string {
	for _, size := range k.Sizes() {
		parsed, ok := grid.ParseSize(size)
		if ok && grid.Fits(layout, saved.Kind, parsed, saved.Col, saved.Row) {
			return size
		}
	}
	return grid.None
}

// unknownKinds are placements of widgets a newer build added. They are kept and
// saved back untouched, so going back to that build finds them where they were.
func unknownKinds(c *widget.Catalog, layout []grid.Placement) []grid.Placement {
	var out []grid.Placement
	for _, p := range layout {
		if _, known := c.Get(p.Kind); !known {
			out = append(out, p)
		}
	}
	return out
}

func knownKinds(c *widget.Catalog, layout []grid.Placement) []grid.Placement {
	out := []grid.Placement{}
	for _, p := range layout {
		if _, known := c.Get(p.Kind); known {
			out = append(out, p)
		}
	}
	return out
}

// makeWay hides unknown widgets that a change now covers, keeping their spot.
func makeWay(known, unknown []grid.Placement) []grid.Placement {
	all := append([]grid.Placement(nil), known...)
	var out []grid.Placement
	for _, p := range unknown {
		if size, visible := p.Visible(); visible && !grid.Fits(all, p.Kind, size, p.Col, p.Row) {
			p.Size = grid.None
		}
		all = append(all, p)
		out = append(out, p)
	}
	return out
}

// fitCatalog keeps one placement per catalog kind, in catalog order; kinds the
// config never saw start hidden. Kinds this build does not know are handled by unknownKinds.
func fitCatalog(c *widget.Catalog, layout []grid.Placement) []grid.Placement {
	out := []grid.Placement{}
	for _, k := range c.All() {
		p := grid.Placement{Kind: k.Name(), Size: grid.None}
		for _, saved := range layout {
			if saved.Kind != k.Name() {
				continue
			}
			p = saved
			if saved.Size != grid.None && !c.Supports(saved.Kind, saved.Size) {
				p.Size = retiredSize(k, layout, saved)
			}
		}
		out = append(out, p)
	}
	return out
}

// Display implements httpd.DisplaySource.
func (a *App) Display() config.Display {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.Display
}

// RefreshToken implements httpd.DisplaySource.
func (a *App) RefreshToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshToken
}

// RefreshKindle asks the Kindle for a flashing repaint of its whole screen. It
// happens on the Kindle's next poll.
func (a *App) RefreshKindle() State {
	a.mu.Lock()
	a.refreshToken = strconv.FormatInt(a.deps.Now().UnixNano(), 36)
	a.mu.Unlock()
	a.deps.Log.Add(applog.Kindle, "asked the Kindle to refresh its screen")
	return a.GetState()
}

func (a *App) CloseToTray() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.CloseToTray
}

func (a *App) GetState() State {
	a.mu.Lock()
	cfg := a.config
	a.mu.Unlock()

	now := a.deps.Now()
	snapshot := a.deps.Usage.Latest()
	state := State{
		Usage:   snapshot,
		Token:   a.tokenState(snapshot, now),
		Kindle:  a.kindleState(now),
		Display: cfg.Display,
		Settings: Settings{
			Port:                 cfg.Port,
			UsageIntervalSeconds: cfg.CacheSeconds,
			CloseToTray:          cfg.CloseToTray,
		},
		Address:   fmt.Sprintf("http://%s:%d", a.deps.LanIP(), cfg.Port),
		Listening: a.deps.Listening == nil || a.deps.Listening(),
		Now:       now.Unix(),
	}
	if a.deps.Media != nil {
		state.Media = a.deps.Media.Snapshot()
	}
	state.Widgets = WidgetsState{
		Catalog: a.catalog(),
		Layout:  knownKinds(a.deps.Widgets, cfg.Layout),
		Options: a.options(cfg),
		Lighten: a.lighten(cfg),
		Screen: widget.BuildScreen(a.deps.Widgets, cfg.Layout, widget.Data{
			Now:     now,
			Usage:   snapshot,
			Media:   state.Media,
			Options: cfg.WidgetOptions,
			Lighten: a.lighten(cfg),
		}),
	}
	a.mu.Lock()
	if a.battery != nil {
		level := *a.battery
		state.Kindle.Battery = &level
	}
	a.mu.Unlock()
	if a.deps.Autostart != nil {
		state.Settings.Autostart, _ = a.deps.Autostart.Enabled()
	}

	focusX, focusY, zoom := a.deps.Background.View()
	state.Background = BackgroundState{FocusX: focusX, FocusY: focusY, Zoom: zoom}
	if hash := a.deps.Background.Hash(); hash != "" {
		state.Background.Set = true
		state.Background.Hash = hash
		state.Background.SourceHash = a.deps.Background.FileHash()
		state.Background.Name = filepath.Base(a.deps.Background.Path())
	}
	return state
}

func (a *App) tokenState(snapshot usage.Snapshot, now time.Time) TokenState {
	if snapshot.Stale && snapshot.Error == usage.MessageLoginRequired {
		return TokenState{Status: TokenLoginRequired}
	}
	expires, err := a.deps.TokenExpiry()
	if err != nil {
		return TokenState{Status: TokenUnknown}
	}
	if now.Before(expires) {
		return TokenState{Status: TokenValid, ExpiresAt: expires.Unix()}
	}
	return TokenState{Status: TokenExpired, ExpiresAt: expires.Unix()}
}

func (a *App) kindleState(now time.Time) KindleState {
	ip, at, ok := a.deps.Clients.LastClient()
	if !ok {
		return KindleState{}
	}
	return KindleState{Seen: true, Connected: now.Sub(at) < kindleTimeout, IP: ip, LastSeen: at.Unix()}
}

// RefreshUsage asks Claude again, unless it was asked moments ago: frequent
// requests are what got the account rate limited.
func (a *App) RefreshUsage() State {
	now := a.deps.Now()
	a.mu.Lock()
	tooSoon := !a.lastRefresh.IsZero() && now.Sub(a.lastRefresh) < minManualRefresh
	if !tooSoon {
		a.lastRefresh = now
	}
	a.mu.Unlock()

	if !tooSoon {
		a.deps.Usage.Refresh(context.Background())
	}
	return a.GetState()
}

func (a *App) ChooseBackground() (State, error) {
	path, err := a.deps.PickFile()
	if err != nil {
		return a.GetState(), err
	}
	if path == "" {
		return a.GetState(), nil
	}
	restore := a.rememberBackground()
	if err := a.deps.Background.Set(path); err != nil {
		a.deps.Log.Add(applog.System, "could not use the background picture %s", filepath.Base(path))
		return a.GetState(), errors.New(messageNotPicture)
	}
	// A new picture starts centered and whole; the old view belonged to the old picture.
	a.deps.Background.SetView(0.5, 0.5, background.MinZoom)
	if err := a.saveBackground(restore); err != nil {
		return a.GetState(), err
	}
	a.deps.Log.Add(applog.System, "background picture changed to %s", filepath.Base(path))
	return a.GetState(), nil
}

func (a *App) ClearBackground() (State, error) {
	restore := a.rememberBackground()
	a.deps.Background.Clear()
	if err := a.saveBackground(restore); err != nil {
		return a.GetState(), err
	}
	a.deps.Log.Add(applog.System, "background picture removed")
	return a.GetState(), nil
}

func (a *App) SetBackgroundFocus(x, y float64) (State, error) {
	_, _, zoom := a.deps.Background.View()
	return a.SetBackgroundView(x, y, zoom)
}

// SetBackgroundView picks the part of the picture the Kindle shows: where it
// is centered and how far it is zoomed in.
func (a *App) SetBackgroundView(x, y, zoom float64) (State, error) {
	restore := a.rememberBackground()
	a.deps.Background.SetView(x, y, zoom)
	err := a.saveBackground(restore)
	return a.GetState(), err
}

// rememberBackground returns a function that puts the current picture and view back.
func (a *App) rememberBackground() func() {
	path := a.deps.Background.Path()
	focusX, focusY, zoom := a.deps.Background.View()
	return func() {
		a.deps.Background.Restore(path, focusX, focusY, zoom)
	}
}

// saveBackground writes the current choice to the config. If that fails the
// choice is undone, so what is served never differs from what a restart would load.
func (a *App) saveBackground(restore func()) error {
	focusX, focusY, zoom := a.deps.Background.View()
	err := a.update(func(c *config.Config) {
		c.Background = config.Background{Path: a.deps.Background.Path(), FocusX: focusX, FocusY: focusY, Zoom: zoom}
	})
	if err == nil {
		return nil
	}
	restore()
	a.deps.Log.Add(applog.System, "could not save the background picture: %v", err)
	return errors.New(messageSaveFailed)
}

func (a *App) SaveDisplay(d config.Display) (State, error) {
	probe := config.Default()
	probe.Display = d
	if probe.Validate() != nil {
		return a.GetState(), errors.New("The display values are not valid")
	}
	if err := a.update(func(c *config.Config) { c.Display = d }); err != nil {
		a.deps.Log.Add(applog.System, "could not save the display settings: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	a.deps.Log.Add(
		applog.Kindle,
		"display settings changed: full refresh every %d min, media poll every %d s, front light %d, warmth %d",
		d.FullRefreshMinutes,
		d.MediaIntervalSeconds,
		d.Frontlight,
		d.Warmth,
	)
	return a.GetState(), nil
}

// SaveSettings applies the settings page. Steps that can fail come first and
// leave nothing changed; once the config is written the save is reported as done.
func (a *App) SaveSettings(s Settings) (State, error) {
	a.saving.Lock()
	defer a.saving.Unlock()

	a.mu.Lock()
	previous := a.config
	a.mu.Unlock()

	next := previous
	next.Port, next.CacheSeconds, next.CloseToTray = s.Port, s.UsageIntervalSeconds, s.CloseToTray
	if err := next.Validate(); err != nil {
		return a.GetState(), errors.New("The settings are not valid")
	}
	autostartChanged, err := a.applyAutostart(s.Autostart)
	if err != nil {
		a.deps.Log.Add(applog.System, "could not change start with Windows: %v", err)
		return a.GetState(), errors.New("Could not change start with Windows")
	}

	// A port that failed to open at startup is retried even when it is unchanged.
	listening := a.deps.Listening == nil || a.deps.Listening()
	rebound := next.Port != previous.Port || !listening
	if rebound {
		if err := a.deps.Rebind(next.Port); err != nil {
			a.deps.Log.Add(applog.System, "could not open port %d: %v", next.Port, err)
			a.undoAutostart(autostartChanged, !s.Autostart)
			return a.GetState(), fmt.Errorf("Could not open port %d, another program may be using it", next.Port)
		}
	}

	// Only this page's fields are written, so a display or background change
	// saved a moment ago from another page is not overwritten.
	err = a.update(func(c *config.Config) {
		c.Port, c.CacheSeconds, c.CloseToTray = next.Port, next.CacheSeconds, next.CloseToTray
	})
	if err != nil {
		a.deps.Log.Add(applog.System, "could not save the settings: %v", err)
		a.undoAutostart(autostartChanged, !s.Autostart)
		a.undoRebind(rebound && listening, previous.Port, next.Port)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	if next.CacheSeconds != previous.CacheSeconds {
		a.deps.SetUsageInterval(next.CacheSeconds)
	}
	a.deps.Log.Add(applog.System, "settings saved: port %d, usage polled every %d s", next.Port, next.CacheSeconds)
	return a.GetState(), nil
}

// undoRebind moves the server back after a save that could not be written. If
// even that fails, the server really is on the new port, so memory follows it
// rather than claim a port nothing is listening on.
func (a *App) undoRebind(needed bool, previousPort, currentPort int) {
	if !needed || previousPort == currentPort {
		return
	}
	if err := a.deps.Rebind(previousPort); err != nil {
		a.deps.Log.Add(applog.System, "could not go back to port %d, now on port %d: %v", previousPort, currentPort, err)
		a.mu.Lock()
		a.config.Port = currentPort
		a.mu.Unlock()
	}
}

// applyAutostart changes the sign-in entry only when it differs from what is
// wanted, and reports whether it changed anything.
func (a *App) applyAutostart(want bool) (changed bool, err error) {
	if a.deps.Autostart == nil {
		return false, nil
	}
	current, err := a.deps.Autostart.Enabled()
	if err != nil {
		return false, err
	}
	if current == want {
		return false, nil
	}
	return true, a.setAutostart(want)
}

func (a *App) setAutostart(on bool) error {
	if on {
		return a.deps.Autostart.Enable()
	}
	return a.deps.Autostart.Disable()
}

// undoAutostart puts the sign-in entry back when a later step of the save failed.
func (a *App) undoAutostart(changed, previous bool) {
	if !changed {
		return
	}
	if err := a.setAutostart(previous); err != nil {
		a.deps.Log.Add(applog.System, "could not restore start with Windows: %v", err)
	}
}

// update changes the config, and keeps the change only if it is valid and saved.
func (a *App) update(change func(*config.Config)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := a.config
	change(&next)
	if err := config.Save(a.deps.ConfigPath, next); err != nil {
		return err
	}
	a.config = next
	return nil
}

func (a *App) GetLogs() []applog.Entry {
	return a.deps.Log.Entries()
}

func (a *App) OpenLogFile() error {
	return a.deps.OpenPath(a.deps.LogPath)
}

func (a *App) OpenDataFolder() error {
	return a.deps.OpenPath(filepath.Dir(a.deps.ConfigPath))
}

func (a *App) catalog() []WidgetKind {
	var out []WidgetKind
	for _, k := range a.deps.Widgets.All() {
		out = append(out, WidgetKind{
			Kind:    k.Name(),
			Label:   k.Label(),
			Icon:    k.Icon(),
			Sizes:   k.Sizes(),
			Options: a.deps.Widgets.OptionsOf(k.Name()),
		})
	}
	return out
}

// Screen implements httpd.ScreenSource: it is the Kindle's poll.
func (a *App) Screen(battery int, ok bool) widget.Screen {
	a.mu.Lock()
	if ok {
		a.battery = &battery
	}
	layout := a.config.Layout
	options := a.config.WidgetOptions
	lighten := a.lighten(a.config)
	a.mu.Unlock()

	d := widget.Data{Now: a.deps.Now(), Usage: a.deps.Usage.Latest(), Options: options, Lighten: lighten}
	if a.deps.Media != nil {
		d.Media = a.deps.Media.Snapshot()
	}
	return widget.BuildScreen(a.deps.Widgets, layout, d)
}

// Act implements httpd.ScreenSource: it runs a widget button's action.
func (a *App) Act(name string) error {
	action, ok := a.deps.Widgets.Action(name)
	if !ok {
		return widget.ErrUnknownAction
	}
	return action()
}

var (
	errUnknownOption = errors.New("Unknown widget option")
	errUnknownWidget = errors.New("Unknown widget or size")
	errNoRoom        = errors.New("No room for this size. Move or hide another widget first")
	errSpotTaken     = errors.New("That spot is taken")
)

// SetWidgetSize shows, hides or resizes a widget. "none" hides it and keeps
// its spot for when it comes back.
func (a *App) SetWidgetSize(kind, size string) (State, error) {
	if size != grid.None && !a.deps.Widgets.Supports(kind, size) {
		return a.GetState(), errUnknownWidget
	}
	if _, known := a.deps.Widgets.Get(kind); !known {
		return a.GetState(), errUnknownWidget
	}

	err := a.updateLayout(kind, func(layout []grid.Placement, p *grid.Placement) error {
		if size == grid.None {
			p.Size = grid.None
			return nil
		}
		wanted, _ := grid.ParseSize(size)
		place := grid.Resize
		if p.Size == grid.None {
			place = grid.Show
		}
		col, row, ok := place(layout, kind, wanted)
		if !ok {
			return errNoRoom
		}
		p.Size, p.Col, p.Row = size, col, row
		return nil
	})
	return a.GetState(), err
}

// MoveWidget puts a visible widget's top-left corner on another cell.
func (a *App) MoveWidget(kind string, col, row int) (State, error) {
	err := a.updateLayout(kind, func(layout []grid.Placement, p *grid.Placement) error {
		size, visible := p.Visible()
		if !visible || !grid.Fits(layout, kind, size, col, row) {
			return errSpotTaken
		}
		p.Col, p.Row = col, row
		return nil
	})
	return a.GetState(), err
}

// updateLayout changes one kind's placement and saves; nothing changes on any error.
func (a *App) updateLayout(kind string, change func(layout []grid.Placement, p *grid.Placement) error) error {
	// Two quick changes must not each start from the same old layout.
	a.saving.Lock()
	defer a.saving.Unlock()

	a.mu.Lock()
	saved := append([]grid.Placement(nil), a.config.Layout...)
	a.mu.Unlock()
	layout := knownKinds(a.deps.Widgets, saved)

	index := -1
	for i, p := range layout {
		if p.Kind == kind {
			index = i
		}
	}
	if index < 0 {
		return errUnknownWidget
	}
	if err := change(layout, &layout[index]); err != nil {
		return err
	}
	all := append(layout, makeWay(layout, unknownKinds(a.deps.Widgets, saved))...)
	if err := a.update(func(c *config.Config) { c.Layout = all }); err != nil {
		a.deps.Log.Add(applog.System, "could not save the widget layout: %v", err)
		return errors.New(messageSaveFailed)
	}
	p := layout[index]
	a.deps.Log.Add(applog.Kindle, "widget layout changed: %s %s at %d,%d", p.Kind, p.Size, p.Col, p.Row)
	return nil
}

// options lists every kind's option values, with the default where none was chosen.
func (a *App) options(cfg config.Config) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, k := range a.deps.Widgets.All() {
		values := map[string]string{}
		for _, o := range a.deps.Widgets.OptionsOf(k.Name()) {
			values[o.Key] = o.Default
			if chosen, ok := cfg.WidgetOptions[k.Name()][o.Key]; ok && a.deps.Widgets.ValidOption(k.Name(), o.Key, chosen) {
				values[o.Key] = chosen
			}
		}
		out[k.Name()] = values
	}
	return out
}

// lighten lists every kind's panel lightening, with the default where none was chosen.
func (a *App) lighten(cfg config.Config) map[string]float64 {
	out := map[string]float64{}
	for _, k := range a.deps.Widgets.All() {
		out[k.Name()] = cfg.Lighten(k.Name())
	}
	return out
}

// SetWidgetLighten sets how strongly one widget's panel lightens the background, 0 to 1.
func (a *App) SetWidgetLighten(kind string, value float64) (State, error) {
	if _, known := a.deps.Widgets.Get(kind); !known {
		return a.GetState(), errUnknownWidget
	}
	if value < 0 || value > 1 {
		return a.GetState(), errors.New("Transparency must be between 0 and 100%")
	}
	err := a.update(func(c *config.Config) {
		lighten := map[string]float64{}
		for k, v := range c.WidgetLighten {
			lighten[k] = v
		}
		lighten[kind] = value
		c.WidgetLighten = lighten
	})
	if err != nil {
		a.deps.Log.Add(applog.System, "could not save the widget transparency: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	a.deps.Log.Add(applog.Kindle, "widget transparency changed: %s %.0f%%", kind, (1-value)*100)
	return a.GetState(), nil
}

// SetWidgetOption chooses one of a widget's options, such as its alignment.
func (a *App) SetWidgetOption(kind, key, value string) (State, error) {
	if !a.deps.Widgets.ValidOption(kind, key, value) {
		return a.GetState(), errUnknownOption
	}
	err := a.update(func(c *config.Config) {
		options := map[string]map[string]string{}
		for k, values := range c.WidgetOptions {
			options[k] = map[string]string{}
			for key, v := range values {
				options[k][key] = v
			}
		}
		if options[kind] == nil {
			options[kind] = map[string]string{}
		}
		options[kind][key] = value
		c.WidgetOptions = options
	})
	if err != nil {
		a.deps.Log.Add(applog.System, "could not save the widget option: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	a.deps.Log.Add(applog.Kindle, "widget option changed: %s %s = %s", kind, key, value)
	return a.GetState(), nil
}
