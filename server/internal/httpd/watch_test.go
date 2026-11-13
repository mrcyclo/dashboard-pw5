package httpd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type watchHarness struct {
	now     time.Time
	lines   []string
	handler http.Handler
	served  int
}

func newWatchHarness() *watchHarness {
	h := &watchHarness{now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.served++ })
	h.handler = WatchClients(next, func() time.Time { return h.now }, func(format string, args ...any) {
		h.lines = append(h.lines, fmt.Sprintf(format, args...))
	})
	return h
}

func (h *watchHarness) request(remote, method, path string) {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remote
	h.handler.ServeHTTP(httptest.NewRecorder(), r)
}

func TestWatchLogsFirstContact(t *testing.T) {
	h := newWatchHarness()
	h.request("10.0.0.50:41000", http.MethodGet, "/screen.json")
	if len(h.lines) != 1 || !strings.Contains(h.lines[0], "10.0.0.50") || !strings.Contains(h.lines[0], "connected") {
		t.Fatalf("lines %q", h.lines)
	}
	if h.served != 1 {
		t.Fatal("the request must still be served")
	}
}

func TestWatchStaysQuietDuringNormalPolling(t *testing.T) {
	h := newWatchHarness()
	for i := 0; i < 20; i++ {
		h.request("10.0.0.50:41000", http.MethodGet, "/screen.json")
		h.now = h.now.Add(30 * time.Second)
	}
	if len(h.lines) != 1 {
		t.Fatalf("regular polling, including the 30 second backoff, must not be logged: %q", h.lines)
	}
}

func TestWatchLogsReturnAfterSilence(t *testing.T) {
	h := newWatchHarness()
	h.request("10.0.0.50:41000", http.MethodGet, "/screen.json")
	h.now = h.now.Add(10 * time.Second)
	h.request("10.0.0.50:41001", http.MethodPost, "/action/media.next")
	h.now = h.now.Add(4*time.Minute + 12*time.Second)
	h.request("10.0.0.50:41002", http.MethodGet, "/screen.json")
	if len(h.lines) != 2 {
		t.Fatalf("lines %q", h.lines)
	}
	line := h.lines[1]
	for _, want := range []string{"10.0.0.50", "4m12s", "12:00:10", "POST /action/media.next"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q should mention %q", line, want)
		}
	}
}

func TestWatchTracksClientsSeparately(t *testing.T) {
	h := newWatchHarness()
	h.request("10.0.0.50:1", http.MethodGet, "/screen.json")
	h.now = h.now.Add(5 * time.Minute)
	h.request("10.0.0.77:1", http.MethodGet, "/screen.json")
	if len(h.lines) != 2 || !strings.Contains(h.lines[1], "connected") {
		t.Fatalf("a second device is a first contact, not a return: %q", h.lines)
	}
}

func TestWatchIgnoresThisMachine(t *testing.T) {
	h := newWatchHarness()
	for _, remote := range []string{"127.0.0.1:5000", "[::1]:5000"} {
		h.request(remote, http.MethodGet, "/screen.json")
	}
	if len(h.lines) != 0 || h.served != 2 {
		t.Fatalf("lines %q served %d", h.lines, h.served)
	}
}

func TestWatchDoesNotLogQueryStrings(t *testing.T) {
	h := newWatchHarness()
	h.request("10.0.0.50:1", http.MethodGet, "/background.png?w=1236&h=1648")
	h.now = h.now.Add(10 * time.Minute)
	h.request("10.0.0.50:1", http.MethodGet, "/screen.json")
	if strings.Contains(h.lines[1], "?") || !strings.Contains(h.lines[1], "GET /background.png") {
		t.Fatalf("got %q", h.lines[1])
	}
}

func TestWatchForgetsManyStrangers(t *testing.T) {
	h := newWatchHarness()
	for i := 0; i < 500; i++ {
		h.request(fmt.Sprintf("10.0.%d.%d:1", i/250, i%250+1), http.MethodGet, "/")
	}
	if tracked := trackedClients(h.handler); tracked > maxTrackedClients {
		t.Fatalf("tracking %d clients", tracked)
	}
}

func TestLastClient(t *testing.T) {
	h := newWatchHarness()
	watch := h.handler.(*ClientWatch)
	if _, _, ok := watch.LastClient(); ok {
		t.Fatal("nobody has connected yet")
	}
	h.request("127.0.0.1:5000", http.MethodGet, "/screen.json")
	if _, _, ok := watch.LastClient(); ok {
		t.Fatal("this machine is not a client")
	}
	h.request("10.0.0.50:1", http.MethodGet, "/screen.json")
	h.now = h.now.Add(10 * time.Second)
	h.request("10.0.0.77:1", http.MethodGet, "/screen.json")
	ip, at, ok := watch.LastClient()
	if !ok || ip != "10.0.0.77" || !at.Equal(h.now) {
		t.Fatalf("got %q %v %v", ip, at, ok)
	}
	h.now = h.now.Add(5 * time.Second)
	h.request("10.0.0.50:2", http.MethodGet, "/screen.json")
	if ip, _, _ := watch.LastClient(); ip != "10.0.0.50" {
		t.Fatalf("the most recent caller should win, got %q", ip)
	}
}

func TestWatchSanitizesLoggedPath(t *testing.T) {
	h := newWatchHarness()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "10.0.0.50:1"
	r.URL.Path = "/a\nforged line\r" + strings.Repeat("z", 500)
	h.handler.ServeHTTP(httptest.NewRecorder(), r)
	h.now = h.now.Add(5 * time.Minute)
	h.request("10.0.0.50:1", http.MethodGet, "/screen.json")
	line := h.lines[1]
	if strings.ContainsAny(line, "\n\r") {
		t.Fatalf("control characters reached the log: %q", line)
	}
	if len(line) > 250 {
		t.Fatalf("an oversized path reached the log: %d bytes", len(line))
	}
}
