package app

import (
	"context"
	"encoding/json"
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
			map[string]any{"kind": "clock", "hash": "c1", "rect": map[string]any{"x": 48, "y": 48, "w": 562, "h": 180},
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
