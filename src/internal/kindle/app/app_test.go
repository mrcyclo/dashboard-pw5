package app

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"

	"claudedash/internal/discovery"
	"claudedash/internal/kindle/render"
	"claudedash/internal/kindle/screen"
	"claudedash/internal/tree"
)

type call struct {
	full  bool
	rect  image.Rectangle
	flash bool
}

type fakeDisplay struct {
	mu    sync.Mutex
	calls []call
}

func (d *fakeDisplay) Show(_ *image.Gray, r image.Rectangle, flash bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, call{rect: r, flash: flash})
	return nil
}

func (d *fakeDisplay) Full(_ *image.Gray, flash bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, call{full: true, flash: flash})
	return nil
}

func (d *fakeDisplay) snapshot() []call {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]call(nil), d.calls...)
}

type fakeRunner struct {
	mu       sync.Mutex
	commands []string
	// wifiOff makes the Wi-Fi radio read as off.
	wifiOff bool
}

func (r *fakeRunner) ran(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (r *fakeRunner) Run(cmd string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmd)
	if strings.Contains(cmd, "wirelessEnable") && strings.HasPrefix(cmd, "lipc-get-prop") {
		if r.wifiOff {
			return "0", nil
		}
		return "1", nil
	}
	if strings.Contains(cmd, "battLevel") {
		return "76", nil
	}
	return "", nil
}

const pcID = "aaaaaaaaaaaaaaaa"

// pc is a fake PC app serving one music widget whose play/pause and hash it controls.
type pc struct {
	mu      sync.Mutex
	icon    string
	hash    string
	clock   string
	stall   chan struct{}
	actions atomic.Int32
	server  *httptest.Server
}

func (p *pc) screen() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, _ := json.Marshal(map[string]any{
		"layout": "L1",
		"widgets": []any{
			map[string]any{"kind": "media", "hash": p.hash, "rect": map[string]any{"x": 48, "y": 1224, "w": 1140, "h": 376},
				"tree": map[string]any{"kind": "row", "children": []any{
					map[string]any{"kind": "button", "id": "pp", "icon": p.icon, "size": 120, "action": "media.play-pause",
						"patch": []any{map[string]any{"id": "pp", "op": "toggle", "prop": "icon", "values": []any{p.icon, "other"}}}},
				}}},
			map[string]any{"kind": "clock", "hash": "c" + p.clock, "rect": map[string]any{"x": 48, "y": 48, "w": 562, "h": 180},
				"tree": map[string]any{"kind": "clock", "format": "time", "size": 72}},
		},
		"icons": map[string]any{},
	})
	return data
}

func newPC(t *testing.T) *pc {
	p := &pc{icon: "pause", hash: "m1"}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/action/"):
			p.actions.Add(1)
			p.wait()
			w.Write(p.screen())
		case r.URL.Path == "/screen.json":
			p.wait()
			w.Write(p.screen())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

// stallUntil makes requests wait until the returned function is called.
func (p *pc) stallUntil() func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stall = make(chan struct{})
	return func() { close(p.stall) }
}

func (p *pc) wait() {
	p.mu.Lock()
	stall := p.stall
	p.mu.Unlock()
	if stall != nil {
		<-stall
	}
}

func (p *pc) seen() discovery.Seen {
	addr := p.server.Listener.Addr().(*net.TCPAddr)
	return discovery.Seen{Beacon: discovery.NewBeacon(pcID, "desk-pc", addr.Port), Addr: addr.IP, At: time.Now()}
}

func fonts(t *testing.T) render.Fonts {
	regular, _ := opentype.Parse(goregular.TTF)
	bold, _ := opentype.Parse(gobold.TTF)
	return render.Fonts{Regular: regular, Bold: bold}
}

type harness struct {
	cfg     Config
	display *fakeDisplay
	runner  *fakeRunner
	beacons chan discovery.Seen
	taps    chan image.Point
	states  chan render.State
	done    chan error
	cancel  context.CancelFunc
}

func start(t *testing.T, dir string, prepare func(*Config)) *harness {
	h := &harness{
		display: &fakeDisplay{},
		runner:  &fakeRunner{},
		beacons: make(chan discovery.Seen, 8),
		taps:    make(chan image.Point, 8),
		states:  make(chan render.State, 256),
		done:    make(chan error, 1),
	}
	h.cfg = Config{
		Dir:      dir,
		Fonts:    fonts(t),
		Display:  h.display,
		Runner:   h.runner,
		HTTP:     &http.Client{Timeout: 500 * time.Millisecond},
		Now:      time.Now,
		Beacons:  h.beacons,
		Taps:     h.taps,
		Power:    make(chan string),
		Interval: 30 * time.Millisecond,
		Observe: func(st render.State) {
			select {
			case h.states <- st:
			default:
			}
		},
		MessageFor: 50 * time.Millisecond,
	}
	if prepare != nil {
		prepare(&h.cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() { h.done <- Run(ctx, h.cfg) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(3 * time.Second):
		}
	})
	return h
}

// waitState waits for a drawn frame that satisfies ok.
func (h *harness) waitState(t *testing.T, what string, ok func(render.State) bool) render.State {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case st := <-h.states:
			if ok(st) {
				return st
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func icon(st render.State) string {
	if st.Screen == nil {
		return ""
	}
	root := st.Shown["media"]
	if root == nil {
		return ""
	}
	return tree.Find(root, "pp")["icon"].(string)
}

func online(st render.State) bool {
	return st.Screen != nil && !st.Offline
}

func TestFirstReplyDrawsTheWholeScreen(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.waitState(t, "first-run screen", func(st render.State) bool { return st.FirstRun })
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	time.Sleep(50 * time.Millisecond)
	calls := h.display.snapshot()
	if !calls[len(calls)-1].full {
		t.Fatalf("a new layout repaints everything: %+v", calls)
	}
}

func TestOneChangedWidgetRepaintsOnlyIt(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	time.Sleep(80 * time.Millisecond)
	before := len(h.display.snapshot())
	p.mu.Lock()
	p.hash = "m2"
	p.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		calls := h.display.snapshot()[before:]
		for _, c := range calls {
			if c.full {
				t.Fatalf("a changed widget must not repaint everything: %+v", calls)
			}
			if c.rect == image.Rect(48, 1224, 1188, 1600) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the music widget was not repainted")
}

func TestTapPredictsBeforeThePCAnswers(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", func(st render.State) bool { return online(st) && icon(st) == "pause" })
	defer p.stallUntil()()
	h.taps <- image.Pt(48+24+60, 1224+24+188-24)
	h.waitState(t, "the predicted icon", func(st render.State) bool { return icon(st) == "other" })
	deadline := time.Now().Add(time.Second)
	for p.actions.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.actions.Load() != 1 {
		t.Fatalf("actions %d", p.actions.Load())
	}
}

func TestAtMostTwoActionsWaitAtOnce(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	defer p.stallUntil()()
	for i := 0; i < 3; i++ {
		h.taps <- image.Pt(132, 1388)
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if got := p.actions.Load(); got != 2 {
		t.Fatalf("%d actions reached the PC", got)
	}
}

func TestPredictionExpiresWithThePCOff(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", func(st render.State) bool { return online(st) && icon(st) == "pause" })
	p.server.Close()
	h.taps <- image.Pt(132, 1388)
	h.waitState(t, "the prediction", func(st render.State) bool { return icon(st) == "other" })
	deadline := time.After(6 * time.Second)
	for {
		select {
		case st := <-h.states:
			if icon(st) == "pause" {
				return
			}
		case <-deadline:
			t.Fatal("the prediction never expired")
		}
	}
}

func TestTwoFailedPollsMarkThePCOffline(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	p.server.Close()
	h.waitState(t, "the offline mark", func(st render.State) bool { return st.Offline && st.Screen != nil })
}

func TestCachedScreenShowsBeforeAnyReply(t *testing.T) {
	dir := t.TempDir()
	p := newPC(t)
	h := start(t, dir, nil)
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	time.Sleep(100 * time.Millisecond)
	h.cancel()
	<-h.done

	again := start(t, dir, nil)
	st := again.waitState(t, "the saved screen", func(st render.State) bool { return st.Screen != nil })
	if !st.Offline || len(st.Screen.Widgets) != 2 {
		t.Fatalf("saved screen shown offline: %+v", st)
	}
}

func TestSwitchToAnotherPC(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"pc":"bbbbbbbbbbbbbbbb"}`), 0o644)
	p := newPC(t)
	h := start(t, dir, nil)
	h.beacons <- p.seen()
	st := h.waitState(t, "the switch button", func(st render.State) bool { return st.Switch == "desk-pc" })
	// Tap the middle of the switch button as drawn in that frame.
	h.taps <- h.switchPoint(st)
	h.waitState(t, "the other PC's screen", online)
	data, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if !strings.Contains(string(data), pcID) {
		t.Fatalf("state %s", data)
	}
}

func (h *harness) switchPoint(st render.State) image.Point {
	_, _, corner := render.Draw(image.NewGray(image.Rect(0, 0, render.Width, render.Height)), st, h.cfg.Fonts, nil)
	r := corner.Switch
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

func TestExitTap(t *testing.T) {
	h := start(t, t.TempDir(), nil)
	h.waitState(t, "first-run screen", func(st render.State) bool { return st.FirstRun })
	h.taps <- image.Pt(1200, 60)
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exit did not return")
	}
}

func TestNoTouchScreenShowsAMessageAndReturns(t *testing.T) {
	h := start(t, t.TempDir(), func(c *Config) {
		taps := make(chan image.Point)
		close(taps)
		c.Taps = taps
	})
	h.waitState(t, "the message", func(st render.State) bool { return st.Message == "Touch screen not found" })
	select {
	case err := <-h.done:
		if err == nil {
			t.Fatal("no touch screen is an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not return")
	}
}

func TestManualAddressSkipsDiscovery(t *testing.T) {
	p := newPC(t)
	h := start(t, t.TempDir(), func(c *Config) {
		c.Manual = p.server.Listener.Addr().String()
	})
	h.waitState(t, "the PC's screen", online)
}

func TestWifiOffIsTurnedOnBeforeAnyPCIsKnown(t *testing.T) {
	h := start(t, t.TempDir(), func(c *Config) {
		c.Runner.(*fakeRunner).wifiOff = true
	})
	h.waitState(t, "the Wi-Fi mark", func(st render.State) bool { return st.WifiOff && st.FirstRun })
	if !h.runner.ran("lipc-set-prop com.lab126.cmd wirelessEnable 1") {
		t.Fatal("Wi-Fi must be switched on")
	}
}

// A PC that broadcasts but refuses /screen.json, as when its firewall blocks TCP 8377.
func refusingPC(t *testing.T) discovery.Seen {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	addr := server.Listener.Addr().(*net.TCPAddr)
	return discovery.Seen{Beacon: discovery.NewBeacon(pcID, "desk-pc", addr.Port), Addr: addr.IP, At: time.Now()}
}

func TestFoundButUnreachablePCSaysSo(t *testing.T) {
	h := start(t, t.TempDir(), nil)
	h.beacons <- refusingPC(t)
	st := h.waitState(t, "the hint", func(st render.State) bool { return st.Hint != "" })
	if !strings.Contains(st.Hint, "desk-pc") || !strings.Contains(st.Hint, "firewall") {
		t.Fatalf("hint %q", st.Hint)
	}
}

func TestRepeatedBeaconsDoNotRepaint(t *testing.T) {
	h := start(t, t.TempDir(), nil)
	seen := refusingPC(t)
	h.beacons <- seen
	h.waitState(t, "the hint", func(st render.State) bool { return st.Hint != "" })
	time.Sleep(100 * time.Millisecond)
	before := len(h.display.snapshot())
	for i := 0; i < 5; i++ {
		seen.At = time.Now()
		h.beacons <- seen
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if after := len(h.display.snapshot()); after != before {
		t.Fatalf("%d repaints for beacons that changed nothing", after-before)
	}
}

func TestTapWhileAsleepWakesInsteadOfActing(t *testing.T) {
	power := make(chan string, 2)
	h := start(t, t.TempDir(), func(c *Config) { c.Power = power })
	h.waitState(t, "first-run screen", func(st render.State) bool { return st.FirstRun })
	power <- "goingToScreenSaver"
	time.Sleep(50 * time.Millisecond)
	h.taps <- image.Pt(1200, 60)
	h.waitState(t, "a wake repaint", func(st render.State) bool { return st.FirstRun })
	select {
	case <-h.done:
		t.Fatal("a tap while asleep must not exit")
	case <-time.After(100 * time.Millisecond):
	}
	if !h.runner.ran("lipc-set-prop com.lab126.powerd preventScreenSaver 1") {
		t.Fatal("waking blocks the screensaver again")
	}
}

type flakyDisplay struct {
	fakeDisplay
	failNext atomic.Bool
}

func (d *flakyDisplay) Show(img *image.Gray, r image.Rectangle, flash bool) error {
	d.fakeDisplay.Show(img, r, flash)
	if d.failNext.Swap(false) {
		return errors.New("fbink failed")
	}
	return nil
}

func TestFailedRegionIsShownAgainNextTime(t *testing.T) {
	p := newPC(t)
	flaky := &flakyDisplay{}
	h := start(t, t.TempDir(), func(c *Config) { c.Display = flaky })
	h.beacons <- p.seen()
	h.waitState(t, "the PC's screen", online)
	time.Sleep(80 * time.Millisecond)
	media := image.Rect(48, 1224, 1188, 1600)
	shows := func() int {
		count := 0
		for _, c := range flaky.snapshot() {
			if c.rect == media {
				count++
			}
		}
		return count
	}
	flaky.failNext.Store(true)
	p.mu.Lock()
	p.hash = "m2"
	p.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for shows() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// Only the clock changes next, but the music region that failed is shown with it.
	p.mu.Lock()
	p.clock = "2"
	p.mu.Unlock()
	for time.Now().Before(deadline) {
		if shows() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the failed region was not shown again")
}

func TestActionReplyFromAnOldPCIsIgnored(t *testing.T) {
	p := newPC(t)
	l := newLoop(Config{Dir: t.TempDir(), Fonts: fonts(t), Display: &fakeDisplay{}, Runner: &fakeRunner{}, Now: time.Now})
	l.apply(p.screen())
	before := l.scr
	l.base, l.actions = "http://new-pc:8377", 1
	p.mu.Lock()
	p.hash = "late"
	p.mu.Unlock()
	l.result(fetched{kind: "action", base: "http://old-pc:8377", key: "media.next", body: p.screen()})
	if l.scr != before || l.actions != 0 {
		t.Fatalf("a reply from the PC left behind must only free its slot: actions %d", l.actions)
	}
}

// lastShown is the last region drawn without a full repaint.
func lastShown(d *fakeDisplay) image.Rectangle {
	calls := d.snapshot()
	for i := len(calls) - 1; i >= 0; i-- {
		if !calls[i].full {
			return calls[i].rect
		}
	}
	return image.Rectangle{}
}

// E-ink keeps whatever is not repainted, so a corner that got narrower must
// still repaint where its longer text was.
func TestShorterBatteryTextClearsTheOldDigits(t *testing.T) {
	d := &fakeDisplay{}
	l := newLoop(Config{Dir: t.TempDir(), Fonts: fonts(t), Display: d, Runner: &fakeRunner{}, Now: time.Now})
	l.battery, l.hasBattery = 100, true
	l.draw(nil, true, false)
	wide := l.corner.Region
	l.battery = 99
	l.draw([]string{"corner"}, false, false)
	if got := lastShown(d); !wide.In(got) {
		t.Fatalf("repainted %v, but 100%% covered %v", got, wide)
	}
}

func TestReconnectingClearsTheOfflineMark(t *testing.T) {
	p := newPC(t)
	d := &fakeDisplay{}
	l := newLoop(Config{Dir: t.TempDir(), Fonts: fonts(t), Display: d, Runner: &fakeRunner{}, Now: time.Now})
	l.apply(p.screen())
	l.offline = true
	l.draw([]string{"corner"}, false, false)
	marked := l.corner.Region
	l.apply(p.screen())
	if l.offline {
		t.Fatal("a reply from the PC ends offline")
	}
	if got := lastShown(d); !marked.In(got) {
		t.Fatalf("repainted %v, but PC offline covered %v", got, marked)
	}
}

func TestAButtonPictureCountsAsUsed(t *testing.T) {
	tree := map[string]any{"kind": "col", "children": []any{
		map[string]any{"kind": "button", "src": "/apps/a1.png?v=h", "size": 72.0, "action": "apps.open.a1"},
	}}
	if !uses(tree, screen.Asset{Key: "image:/apps/a1.png?v=h", Path: "/apps/a1.png?v=h"}) {
		t.Fatal("a new app icon must redraw its widget once it arrives")
	}
}

// A tapped button flashes inverted at once, so the user sees the tap landed,
// and the widget is drawn normally again shortly after.
func TestTappedButtonShowsItWasPressed(t *testing.T) {
	p := newPC(t)
	d := &fakeDisplay{}
	l := newLoop(Config{Dir: t.TempDir(), Fonts: fonts(t), Display: d, Runner: &fakeRunner{}, Now: time.Now, HTTP: &http.Client{Timeout: time.Second}})
	l.apply(p.screen())
	l.draw(nil, true, false)
	l.base = p.server.URL
	b := l.buttons[0]
	before := l.img.GrayAt(b.Rect.Min.X+b.Rect.Dx()/2, b.Rect.Min.Y+b.Rect.Dy()/2).Y
	l.tap(b.Rect.Min.Add(image.Pt(b.Rect.Dx()/2, b.Rect.Dy()/2)), time.NewTimer(time.Hour))
	if got := lastShown(d); got != b.Rect {
		t.Fatalf("last shown %v, want the button %v", got, b.Rect)
	}
	if after := l.img.GrayAt(b.Rect.Min.X+b.Rect.Dx()/2, b.Rect.Min.Y+b.Rect.Dy()/2).Y; after != 255-before {
		t.Fatalf("pressed pixel %d, was %d", after, before)
	}
	select {
	case kind := <-l.released:
		if kind != b.Kind {
			t.Fatalf("released %q", kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the button must be drawn normally again")
	}
}
