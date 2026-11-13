// Package app is the Kindle dashboard's event loop: it finds the PC, polls it,
// draws what changed, and answers taps, like claudedash.koplugin/main.lua did
// inside KOReader. All state lives in the loop goroutine; network calls run
// beside it and post their results back.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"claudedash/internal/discovery"
	"claudedash/internal/kindle/device"
	"claudedash/internal/kindle/patch"
	"claudedash/internal/kindle/render"
	"claudedash/internal/kindle/screen"
	"claudedash/internal/tree"
)

// Displayer puts parts of a frame on the e-ink screen.
type Displayer interface {
	Show(img *image.Gray, r image.Rectangle, flash bool) error
	Full(img *image.Gray, flash bool) error
}

type Config struct {
	// Dir is the app folder: cache, state.json and the optional config.json.
	Dir     string
	Fonts   render.Fonts
	Display Displayer
	Runner  device.Runner
	HTTP    *http.Client
	Now     func() time.Time
	Beacons <-chan discovery.Seen
	// Taps closes when the touch screen cannot be read.
	Taps  <-chan image.Point
	Power <-chan string
	// Manual is "host:port" from config.json, used instead of discovery.
	Manual string
	Log    func(format string, args ...any)

	// Interval overrides the PC's poll interval; tests make it short.
	Interval time.Duration
	// MessageFor is how long a fatal message stays before Run returns.
	MessageFor time.Duration
	// Observe sees every frame drawn; tests use it.
	Observe func(render.State)
}

var ErrNoTouch = errors.New("app: the touch screen cannot be read")

const (
	disconnectedAfter = 2
	backoffAfter      = 3
	backoffInterval   = 30 * time.Second
	maxPendingActions = 2
	defaultInterval   = 5 * time.Second
)

// Display settings as served by /kindle.json; see config.Display on the PC.
type display struct {
	PanelLighten         float64 `json:"panel_lighten"`
	FullRefreshMinutes   int     `json:"full_refresh_minutes"`
	MediaIntervalSeconds int     `json:"media_interval_seconds"`
	Frontlight           int     `json:"frontlight"`
	Warmth               int     `json:"warmth"`
}

type fetched struct {
	kind  string // "screen", "action", "settings", "background", "asset"
	epoch int
	key   string
	path  string
	body  []byte
	err   error
}

type loop struct {
	c       Config
	img     *image.Gray
	scr     *screen.Screen
	shown   map[string]tree.Node
	pending map[string]*patch.Pending

	chooser *discovery.Chooser
	base    string
	pcName  string
	pcPort  int
	// lastSwitch and hint are what the last frame showed, so beacons repaint only on a change.
	lastSwitch string
	hint       string
	polling    bool
	epoch      int
	actions    int

	failures int
	offline  bool
	wifiOff  bool
	sleeping bool

	settings     display
	settingsHash string
	lightsDone   [2]int

	background *image.Gray
	bgHash     string
	bgLoading  string

	assets  *assetStore
	refresh *string

	battery    int
	hasBattery bool

	regions map[string]image.Rectangle
	buttons []render.Button
	corner  render.Corner

	results chan fetched
	expired chan string
}

func (l *loop) logf(format string, args ...any) {
	if l.c.Log != nil {
		l.c.Log(format, args...)
	}
}

// Run shows the dashboard until exit is tapped (nil) or ctx ends.
func Run(ctx context.Context, c Config) error {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.MessageFor == 0 {
		c.MessageFor = 5 * time.Second
	}
	l := &loop{
		c:          c,
		img:        image.NewGray(image.Rect(0, 0, render.Width, render.Height)),
		shown:      map[string]tree.Node{},
		pending:    map[string]*patch.Pending{},
		chooser:    discovery.NewChooser(loadState(c.Dir)),
		settings:   display{PanelLighten: 0.7, MediaIntervalSeconds: 5, Frontlight: -1, Warmth: -1},
		lightsDone: [2]int{-1, -1},
		assets:     newAssetStore(filepath.Join(c.Dir, "cache", "icons")),
		results:    make(chan fetched, 16),
		expired:    make(chan string, 8),
	}
	os.MkdirAll(filepath.Join(c.Dir, "cache", "icons"), 0o755)
	l.loadCache()
	if c.Manual != "" {
		l.base = "http://" + c.Manual
	}
	l.readBattery()
	l.draw(nil, true, true)

	poll := time.NewTimer(0)
	defer poll.Stop()
	minute := time.NewTimer(untilNextMinute(c.Now()))
	defer minute.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case s, ok := <-c.Beacons:
			if !ok {
				c.Beacons = nil
				continue
			}
			l.hear(s, poll)
		case p, ok := <-c.Taps:
			if !ok {
				return l.noTouch(ctx)
			}
			if done := l.tap(p, poll); done {
				return nil
			}
		case event := <-c.Power:
			l.power(event, poll)
		case <-poll.C:
			l.startPoll()
			poll.Reset(l.interval())
		case r := <-l.results:
			l.result(r)
		case kind := <-l.expired:
			l.expire(kind)
		case <-minute.C:
			l.tick()
			minute.Reset(untilNextMinute(c.Now()))
		}
	}
}

func untilNextMinute(now time.Time) time.Duration {
	return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
}

func (l *loop) interval() time.Duration {
	every := time.Duration(l.settings.MediaIntervalSeconds) * time.Second
	if every <= 0 {
		every = defaultInterval
	}
	if l.c.Interval > 0 {
		every = l.c.Interval
	}
	if l.failures >= backoffAfter && l.c.Interval == 0 {
		every = max(every, backoffInterval)
	}
	return every
}

// draw renders a frame and shows it: everything, or the named regions.
func (l *loop) draw(names []string, full, flash bool) {
	if l.sleeping {
		return
	}
	st := render.State{
		Screen:     l.scr,
		Shown:      l.shown,
		Background: l.background,
		Frost:      l.settings.PanelLighten,
		Battery:    l.battery,
		HasBattery: l.hasBattery,
		Offline:    l.offline && l.scr != nil,
		WifiOff:    l.wifiOff,
		FirstRun:   l.scr == nil,
		Now:        l.c.Now(),
	}
	st.Switch = l.switchName()
	st.Hint = l.hint
	l.lastSwitch = st.Switch
	l.regions, l.buttons, l.corner = render.Draw(l.img, st, l.c.Fonts, l.assets)
	if l.c.Observe != nil {
		l.c.Observe(st)
	}
	if full {
		if err := l.c.Display.Full(l.img, flash); err != nil {
			l.logf("display: %v", err)
		}
		return
	}
	for _, name := range names {
		r, ok := l.regions[name]
		if !ok {
			continue
		}
		if err := l.c.Display.Show(l.img, r, false); err != nil {
			l.logf("display %s: %v", name, err)
		}
	}
}

func (l *loop) noTouch(ctx context.Context) error {
	l.logf("touch screen unavailable")
	st := render.State{Message: "Touch screen not found", Now: l.c.Now()}
	render.Draw(l.img, st, l.c.Fonts, l.assets)
	if l.c.Observe != nil {
		l.c.Observe(st)
	}
	l.c.Display.Full(l.img, true)
	select {
	case <-ctx.Done():
	case <-time.After(l.c.MessageFor):
	}
	return ErrNoTouch
}

// switchName is the other PC the corner offers while this one cannot be reached.
func (l *loop) switchName() string {
	if l.c.Manual != "" || (!l.offline && l.scr != nil) {
		return ""
	}
	others := l.chooser.Others(l.c.Now())
	if len(others) == 0 {
		return ""
	}
	return others[0].Name
}

// hear follows beacons: the chosen PC's address, and others to offer.
func (l *loop) hear(s discovery.Seen, poll *time.Timer) {
	l.chooser.Hear(s)
	if l.c.Manual != "" {
		return
	}
	if current, ok := l.chooser.Current(l.c.Now()); ok {
		l.pcName, l.pcPort = current.Name, current.Port
		base := fmt.Sprintf("http://%s", net.JoinHostPort(current.Addr.String(), strconv.Itoa(current.Port)))
		switch {
		case base != l.base:
			l.base = base
			saveState(l.c.Dir, current.ID)
			poll.Reset(0)
		case l.offline && !l.polling:
			// The PC is broadcasting again: ask now rather than wait out the backoff.
			poll.Reset(0)
		}
	}
	if l.switchName() != l.lastSwitch {
		l.draw([]string{"corner"}, l.scr == nil, false)
	}
}

func (l *loop) get(kind, path string, epoch int, key string) {
	base := l.base
	go func() {
		resp, err := l.c.HTTP.Get(base + path)
		l.results <- finish(kind, path, epoch, key, resp, err)
	}()
}

func finish(kind, path string, epoch int, key string, resp *http.Response, err error) fetched {
	r := fetched{kind: kind, epoch: epoch, key: key, path: path, err: err}
	if err != nil {
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.err = fmt.Errorf("http %d", resp.StatusCode)
		return r
	}
	r.body, r.err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return r
}

func (l *loop) startPoll() {
	if l.sleeping {
		return
	}
	// With no PC known yet the Kindle may simply be offline: no beacon can arrive then.
	if l.base == "" {
		l.checkWifi()
		return
	}
	if l.polling {
		return
	}
	l.polling = true
	l.readBattery()
	path := "/screen.json"
	if l.hasBattery {
		path += "?battery=" + strconv.Itoa(l.battery)
	}
	l.get("screen", path, l.epoch, "")
}

func (l *loop) result(r fetched) {
	switch r.kind {
	case "screen":
		l.polling = false
		if r.epoch != l.epoch {
			return
		}
		if r.err != nil {
			l.pollFailed(r.err)
			return
		}
		l.apply(r.body)
	case "action":
		l.actions--
		if r.err != nil {
			l.logf("action %s: %v", r.key, r.err)
			return
		}
		l.apply(r.body)
	case "settings":
		l.applySettings(r)
	case "background":
		l.applyBackground(r)
	case "asset":
		l.applyAsset(r)
	}
}

func (l *loop) pollFailed(err error) {
	l.failures++
	if l.failures == 1 {
		l.logf("poll failed: %v", err)
	}
	l.checkWifi()
	if l.scr == nil && l.base != "" && l.pcName != "" {
		hint := fmt.Sprintf("Found %s, but cannot connect. Allow TCP port %d in the PC's firewall.", l.pcName, l.pcPort)
		if hint != l.hint {
			l.hint = hint
			l.draw(nil, true, false)
		}
	}
	if !l.offline && l.failures >= disconnectedAfter {
		l.offline = true
		l.draw([]string{"corner"}, false, false)
	}
}

// checkWifi switches Wi-Fi back on when it is off, as the plugin did, and
// keeps the corner's "Wi-Fi is off" mark in step.
func (l *loop) checkWifi() {
	on := device.WifiOn(l.c.Runner)
	if !on {
		device.EnableWifi(l.c.Runner)
	}
	if on == !l.wifiOff {
		return
	}
	l.wifiOff = !on
	l.draw([]string{"corner"}, l.scr == nil, false)
}

func (l *loop) apply(body []byte) {
	current, changes, err := screen.Accept(l.scr, body)
	if err != nil {
		l.pollFailed(err)
		return
	}
	names := []string{}
	if l.offline || l.wifiOff {
		names = append(names, "corner")
	}
	l.failures, l.offline, l.wifiOff, l.hint = 0, false, false, ""
	l.scr = current

	now := l.c.Now()
	shown := map[string]tree.Node{}
	for _, w := range current.Widgets {
		hadPrediction := l.pending[w.Kind] != nil
		shown[w.Kind], l.pending[w.Kind] = patch.Reconcile(l.pending[w.Kind], w.Tree, now)
		if changes.Widgets[w.Kind] || hadPrediction {
			names = append(names, w.Kind)
		}
	}
	for kind := range l.pending {
		if _, ok := shown[kind]; !ok {
			delete(l.pending, kind)
		}
	}
	l.shown = shown
	if changes.Full || len(names) > 0 {
		screen.Save(filepath.Join(l.c.Dir, "cache", "screen.json"), current)
	}
	if changes.Full {
		l.assets.removeStale(current.Icons)
	}
	l.syncSettings()
	l.syncBackground()
	l.syncAssets()

	refresh := false
	if l.refresh == nil {
		token := current.Refresh
		l.refresh = &token
	} else if current.Refresh != *l.refresh {
		refresh = current.Refresh != ""
		*l.refresh = current.Refresh
	}
	switch {
	case refresh:
		l.draw(nil, true, true)
	case changes.Full:
		l.draw(nil, true, false)
	case len(names) > 0:
		l.draw(names, false, false)
	}
}

func (l *loop) syncSettings() {
	if l.scr.Settings == "" || l.scr.Settings == l.settingsHash {
		return
	}
	l.get("settings", "/kindle.json", 0, l.scr.Settings)
}

func (l *loop) applySettings(r fetched) {
	if r.err != nil {
		return
	}
	var d display
	if err := json.Unmarshal(r.body, &d); err != nil {
		return
	}
	l.settings, l.settingsHash = d, r.key
	os.WriteFile(filepath.Join(l.c.Dir, "cache", "kindle.json"), r.body, 0o644)
	if d.Frontlight >= 0 && d.Frontlight != l.lightsDone[0] {
		device.SetLight(l.c.Runner, d.Frontlight)
		l.lightsDone[0] = d.Frontlight
	}
	if d.Warmth >= 0 && d.Warmth != l.lightsDone[1] {
		device.SetWarmth(l.c.Runner, d.Warmth)
		l.lightsDone[1] = d.Warmth
	}
	l.draw(nil, true, false)
}

func (l *loop) syncBackground() {
	hash := l.scr.Background
	if hash == l.bgHash || hash == l.bgLoading {
		return
	}
	if hash == "" {
		l.background, l.bgHash = nil, ""
		os.Remove(filepath.Join(l.c.Dir, "cache", "background.png"))
		l.draw(nil, true, false)
		return
	}
	l.bgLoading = hash
	l.get("background", fmt.Sprintf("/background.png?w=%d&h=%d", render.Width, render.Height), 0, hash)
}

func (l *loop) applyBackground(r fetched) {
	if r.key != l.bgLoading {
		return
	}
	l.bgLoading = ""
	if r.err != nil {
		return
	}
	img, err := png.Decode(bytes.NewReader(r.body))
	if err != nil {
		return
	}
	l.background, l.bgHash = toGray(img), r.key
	os.WriteFile(filepath.Join(l.c.Dir, "cache", "background.png"), r.body, 0o644)
	os.WriteFile(filepath.Join(l.c.Dir, "cache", "background.hash"), []byte(r.key), 0o644)
	l.draw(nil, true, false)
}

func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	g := image.NewGray(img.Bounds())
	draw.Draw(g, g.Bounds(), img, img.Bounds().Min, draw.Src)
	return g
}

func (l *loop) syncAssets() {
	assets := screen.Assets(l.scr)
	l.assets.keepOnly(assets)
	for _, a := range assets {
		if !l.assets.wants(a, l.c.Now()) {
			continue
		}
		l.assets.loading[a.Key] = true
		l.get("asset", a.Path, 0, a.Key)
	}
}

func (l *loop) applyAsset(r fetched) {
	var asset screen.Asset
	for _, a := range screen.Assets(l.scr) {
		if a.Key == r.key {
			asset = a
		}
	}
	delete(l.assets.loading, r.key)
	if asset.Key == "" {
		return
	}
	if r.err != nil || !l.assets.store(asset, r.body) {
		l.assets.fail(r.key, l.c.Now())
		return
	}
	var names []string
	for _, w := range l.scr.Widgets {
		if uses(l.shown[w.Kind], asset) {
			names = append(names, w.Kind)
		}
	}
	l.draw(names, false, false)
}

// uses reports whether a tree draws the given icon or picture.
func uses(node any, a screen.Asset) bool {
	n, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if a.Icon != "" && (n["name"] == a.Icon || n["icon"] == a.Icon) {
		return true
	}
	if a.Icon == "" && n["kind"] == "image" && n["src"] == a.Path {
		return true
	}
	children, _ := n["children"].([]any)
	for _, child := range children {
		if uses(child, a) {
			return true
		}
	}
	return false
}

// tap handles a touch; it reports true when exit was tapped.
func (l *loop) tap(p image.Point, poll *time.Timer) bool {
	switch {
	case p.In(l.corner.Exit):
		return true
	case p.In(l.corner.Refresh):
		l.draw(nil, true, true)
		return false
	case p.In(l.corner.Switch):
		l.switchPC(poll)
		return false
	}
	for _, b := range l.buttons {
		if !p.In(b.Rect) {
			continue
		}
		if b.Disabled || l.actions >= maxPendingActions || l.base == "" {
			return false
		}
		l.epoch++
		if shown, ok := l.shown[b.Kind]; ok {
			l.shown[b.Kind], l.pending[b.Kind] = patch.Stack(l.pending[b.Kind], b.Patch, shown, l.c.Now())
			l.draw([]string{b.Kind}, false, false)
			kind := b.Kind
			time.AfterFunc(patch.Hold+100*time.Millisecond, func() { l.expired <- kind })
		}
		l.actions++
		base, action := l.base, b.Action
		go func() {
			resp, err := l.c.HTTP.Post(base+"/action/"+action, "application/json", nil)
			l.results <- finish("action", "", 0, action, resp, err)
		}()
		poll.Reset(l.interval())
		return false
	}
	return false
}

func (l *loop) switchPC(poll *time.Timer) {
	others := l.chooser.Others(l.c.Now())
	if len(others) == 0 {
		return
	}
	chosen := others[0]
	l.chooser.Choose(chosen.ID)
	saveState(l.c.Dir, chosen.ID)
	l.base = fmt.Sprintf("http://%s", net.JoinHostPort(chosen.Addr.String(), strconv.Itoa(chosen.Port)))
	l.epoch++
	poll.Reset(0)
}

func (l *loop) expire(kind string) {
	var last tree.Node
	for _, w := range l.scrWidgets() {
		if w.Kind == kind {
			last = w.Tree
		}
	}
	shown, pending := patch.Expire(l.pending[kind], last, l.c.Now())
	l.pending[kind] = pending
	if shown == nil {
		return
	}
	l.shown[kind] = shown
	l.draw([]string{kind}, false, false)
}

func (l *loop) scrWidgets() []screen.Widget {
	if l.scr == nil {
		return nil
	}
	return l.scr.Widgets
}

// power follows the Kindle going to sleep on purpose (power button) and waking.
func (l *loop) power(event string, poll *time.Timer) {
	switch event {
	case "goingToScreenSaver":
		l.logf("going to sleep")
		l.c.Runner.Run("lipc-set-prop com.lab126.powerd preventScreenSaver 0")
		l.sleeping = true
	case "outOfScreenSaver":
		l.logf("awake")
		l.c.Runner.Run("lipc-set-prop com.lab126.powerd preventScreenSaver 1")
		l.sleeping = false
		l.draw(nil, true, true)
		poll.Reset(0)
	}
}

// tick redraws what changes with the time, once a minute.
func (l *loop) tick() {
	l.readBattery()
	now := l.c.Now()
	if every := l.settings.FullRefreshMinutes; every > 0 && (now.Hour()*60+now.Minute())%every == 0 {
		l.draw(nil, true, true)
		return
	}
	names := []string{"corner"}
	for _, w := range l.scrWidgets() {
		root := l.shown[w.Kind]
		if root == nil {
			root = w.Tree
		}
		if ticks(root) {
			names = append(names, w.Kind)
		}
	}
	l.draw(names, false, false)
}

func ticks(node any) bool {
	n, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if n["kind"] == "clock" || n["kind"] == "countdown" || n["at"] != nil {
		return true
	}
	children, _ := n["children"].([]any)
	for _, child := range children {
		if ticks(child) {
			return true
		}
	}
	return false
}

func (l *loop) readBattery() {
	l.battery, l.hasBattery = device.Battery(l.c.Runner)
}

// loadCache brings back the last screen, background and settings for a start with the PC off.
func (l *loop) loadCache() {
	cache := filepath.Join(l.c.Dir, "cache")
	if s, err := screen.Load(filepath.Join(cache, "screen.json")); err == nil {
		l.scr, l.offline = s, true
		for _, w := range s.Widgets {
			l.shown[w.Kind] = w.Tree
		}
		l.assets.loadIcons(s.Icons)
	}
	if data, err := os.ReadFile(filepath.Join(cache, "kindle.json")); err == nil {
		json.Unmarshal(data, &l.settings)
	}
	hash, _ := os.ReadFile(filepath.Join(cache, "background.hash"))
	data, err := os.ReadFile(filepath.Join(cache, "background.png"))
	if err != nil || len(hash) == 0 {
		return
	}
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		l.background, l.bgHash = toGray(img), string(hash)
	}
}

type state struct {
	PC string `json:"pc"`
}

func loadState(dir string) string {
	var s state
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		json.Unmarshal(data, &s)
	}
	return s.PC
}

func saveState(dir, id string) {
	data, _ := json.Marshal(state{PC: id})
	os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644)
}
