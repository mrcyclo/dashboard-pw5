package usage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"five_hour":{"utilization":42,"resets_at":"2026-11-11T09:00:00+00:00"},"seven_day":{"utilization":18}}`

type fakeAPI struct {
	srv      *httptest.Server
	requests atomic.Int32
	status   atomic.Int32
	lastAuth atomic.Value
	lastBeta atomic.Value
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{}
	f.status.Store(200)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.lastAuth.Store(r.Header.Get("Authorization"))
		f.lastBeta.Store(r.Header.Get("anthropic-beta"))
		status := int(f.status.Load())
		w.WriteHeader(status)
		if status == 200 {
			w.Write([]byte(okBody))
			return
		}
		w.Write([]byte(`{"type":"error","error":{"message":"bad token sk-secret"}}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func setup(t *testing.T, token func() (string, error)) (*fakeAPI, *clock, *Service) {
	f, c := newFakeAPI(t), &clock{now: time.Unix(1794400000, 0)}
	if token == nil {
		token = func() (string, error) { return "sk-secret", nil }
	}
	s := New(Options{Endpoint: f.srv.URL, TokenFunc: token, Now: c.Now})
	return f, c, s
}

func TestLatestBeforeAnyFetch(t *testing.T) {
	f, _, s := setup(t, nil)
	want := Snapshot{Stale: true, Error: "No data yet"}
	if got := s.Latest(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if f.requests.Load() != 0 {
		t.Fatal("Latest must never call the API")
	}
}

func TestLatestReturnsLastRefreshWithoutFetching(t *testing.T) {
	f, _, s := setup(t, nil)
	fresh := s.Refresh(context.Background())
	for i := 0; i < 5; i++ {
		if got := s.Latest(); !reflect.DeepEqual(got, fresh) {
			t.Fatalf("got %+v want %+v", got, fresh)
		}
	}
	if f.requests.Load() != 1 {
		t.Fatalf("requests %d", f.requests.Load())
	}
	if fresh.Stale || fresh.FetchedAt != 1794400000 || fresh.FiveHour == nil || fresh.FiveHour.UsedPercent != 42 {
		t.Fatalf("got %+v", fresh)
	}
}

func TestRefreshAlwaysFetches(t *testing.T) {
	f, c, s := setup(t, nil)
	s.Refresh(context.Background())
	c.Advance(121 * time.Second)
	got := s.Refresh(context.Background())
	if f.requests.Load() != 2 || got.FetchedAt != 1794400121 {
		t.Fatalf("requests %d snapshot %+v", f.requests.Load(), got)
	}
}

func TestLatestDoesNotWaitForSlowRefresh(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte(okBody))
	}))
	defer slow.Close()
	s := New(Options{Endpoint: slow.URL, TokenFunc: func() (string, error) { return "tok", nil }})
	done := make(chan struct{})
	go func() {
		s.Refresh(context.Background())
		close(done)
	}()
	<-started
	answered := make(chan Snapshot, 1)
	go func() { answered <- s.Latest() }()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		t.Fatal("Latest blocked behind an in-flight refresh")
	}
	close(release)
	<-done
}

func TestSendsAuthHeaders(t *testing.T) {
	f, _, s := setup(t, func() (string, error) { return "tok", nil })
	s.Refresh(context.Background())
	if f.lastAuth.Load() != "Bearer tok" || f.lastBeta.Load() != "oauth-2025-04-20" {
		t.Fatalf("auth %v beta %v", f.lastAuth.Load(), f.lastBeta.Load())
	}
}

func TestFailureKeepsLastSnapshotStale(t *testing.T) {
	f, c, s := setup(t, nil)
	s.Refresh(context.Background())
	f.status.Store(500)
	c.Advance(200 * time.Second)
	got := s.Refresh(context.Background())
	if !got.Stale || got.Error != "Claude returned error 500" || got.FetchedAt != 1794400000 {
		t.Fatalf("got %+v", got)
	}
	if got.FiveHour == nil || got.FiveHour.UsedPercent != 42 {
		t.Fatalf("old data lost: %+v", got)
	}
}

func TestFailureBeforeAnySuccess(t *testing.T) {
	f, _, s := setup(t, func() (string, error) { return "", errors.New("boom sk-secret") })
	got := s.Refresh(context.Background())
	want := Snapshot{Stale: true, Error: "Could not read the token"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if f.requests.Load() != 0 {
		t.Fatal("must not call the API without a token")
	}
}

func TestExpiredTokenMessage(t *testing.T) {
	_, _, s := setup(t, func() (string, error) { return "", ErrTokenExpired })
	if got := s.Refresh(context.Background()); got.Error != "Token expired, open Claude Code" {
		t.Fatalf("got %+v", got)
	}
}

func TestTokenErrorMessages(t *testing.T) {
	cases := map[error]string{
		fmt.Errorf("wrapped: %w", ErrLoginRequired): "Sign in to Claude again on this PC",
		fmt.Errorf("wrapped: %w", ErrRefreshFailed): "Could not renew the token",
	}
	for tokenErr, want := range cases {
		_, _, s := setup(t, func() (string, error) { return "", tokenErr })
		if got := s.Refresh(context.Background()); got.Error != want {
			t.Errorf("%v: got %q", tokenErr, got.Error)
		}
	}
}

func TestUnauthorizedMeansExpired(t *testing.T) {
	f, _, s := setup(t, nil)
	f.status.Store(401)
	if got := s.Refresh(context.Background()); got.Error != "Token expired, open Claude Code" {
		t.Fatalf("got %+v", got)
	}
}

func TestUnreachableMessage(t *testing.T) {
	f, _, s := setup(t, nil)
	f.srv.Close()
	if got := s.Refresh(context.Background()); got.Error != "Cannot reach Claude" {
		t.Fatalf("got %+v", got)
	}
}

func TestBadBodyMessage(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>"))
	}))
	defer bad.Close()
	s := New(Options{Endpoint: bad.URL, TokenFunc: func() (string, error) { return "tok", nil }})
	if got := s.Refresh(context.Background()); got.Error != "The response has an unexpected format" {
		t.Fatalf("got %+v", got)
	}
}

func TestRecoversAfterFailure(t *testing.T) {
	f, _, s := setup(t, nil)
	f.status.Store(500)
	s.Refresh(context.Background())
	f.status.Store(200)
	got := s.Refresh(context.Background())
	if got.Stale || got.Error != "" || got.FiveHour == nil {
		t.Fatalf("got %+v", got)
	}
}

func TestErrorDoesNotLeakToken(t *testing.T) {
	for _, status := range []int32{401, 403, 500} {
		f, _, s := setup(t, nil)
		f.status.Store(status)
		got := s.Refresh(context.Background())
		if !got.Stale || strings.Contains(got.Error, "sk-secret") {
			t.Fatalf("status %d: %+v", status, got)
		}
	}
}

func TestOnUpdateCalled(t *testing.T) {
	f, _, s := setup(t, nil)
	var seen []Snapshot
	s.OnUpdate(func(snap Snapshot) { seen = append(seen, snap) })
	s.Refresh(context.Background())
	f.status.Store(500)
	s.Refresh(context.Background())
	if len(seen) != 2 || seen[0].Stale || !seen[1].Stale {
		t.Fatalf("got %+v", seen)
	}
}

// The listener mutates unsynchronized state, as the tray does. Run with -race.
func TestListenersNeverRunConcurrently(t *testing.T) {
	_, _, s := setup(t, nil)
	calls := 0
	s.OnUpdate(func(Snapshot) { calls++ })
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Refresh(context.Background())
		}()
	}
	wg.Wait()
	if calls != 10 {
		t.Fatalf("calls %d", calls)
	}
}

func TestRateLimitBacksOff(t *testing.T) {
	f, c, s := setup(t, nil)
	s.Refresh(context.Background())
	f.status.Store(429)
	c.Advance(5 * time.Minute)
	got := s.Refresh(context.Background())
	if !got.Stale || got.Error != "Claude is rate limiting, will retry later" || got.FiveHour == nil {
		t.Fatalf("got %+v", got)
	}
	f.status.Store(200)
	before := f.requests.Load()
	c.Advance(14 * time.Minute)
	if again := s.Refresh(context.Background()); !again.Stale || f.requests.Load() != before {
		t.Fatalf("refresh during backoff must not call the API: %+v, %d requests", again, f.requests.Load()-before)
	}
	c.Advance(2 * time.Minute)
	if after := s.Refresh(context.Background()); after.Stale || f.requests.Load() != before+1 {
		t.Fatalf("refresh after backoff should succeed: %+v", after)
	}
}

func TestRateLimitHonoursRetryAfter(t *testing.T) {
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(429)
	}))
	defer limited.Close()
	c := &clock{now: time.Unix(1794400000, 0)}
	var requests atomic.Int32
	counting := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	s := New(Options{Endpoint: limited.URL, TokenFunc: func() (string, error) { return "tok", nil }, Now: c.Now, Client: counting})
	s.Refresh(context.Background())
	c.Advance(59 * time.Minute)
	s.Refresh(context.Background())
	if requests.Load() != 1 {
		t.Fatalf("Retry-After of an hour ignored: %d requests", requests.Load())
	}
	c.Advance(2 * time.Minute)
	s.Refresh(context.Background())
	if requests.Load() != 2 {
		t.Fatalf("should retry after the hour: %d requests", requests.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRememberedSnapshotIsServedAsStaleUntilRefreshed(t *testing.T) {
	f, c := newFakeAPI(t), &clock{now: time.Unix(1794400000, 0)}
	remembered := Snapshot{FetchedAt: 1794390000, SevenDay: &Window{UsedPercent: 28}}
	s := New(Options{
		Endpoint: f.srv.URL, Now: c.Now, Remembered: remembered,
		TokenFunc: func() (string, error) { return "tok", nil },
	})
	got := s.Latest()
	if !got.Stale || got.Error != "" || got.FetchedAt != 1794390000 || got.SevenDay == nil || got.SevenDay.UsedPercent != 28 {
		t.Fatalf("got %+v", got)
	}
	f.status.Store(429)
	if limited := s.Refresh(context.Background()); limited.SevenDay == nil || limited.Error == "" {
		t.Fatalf("a failed refresh must keep the remembered numbers: %+v", limited)
	}
}

func TestRememberedSnapshotWithoutDataIsIgnored(t *testing.T) {
	f, c := newFakeAPI(t), &clock{now: time.Unix(1794400000, 0)}
	s := New(Options{Endpoint: f.srv.URL, Now: c.Now, Remembered: Snapshot{Stale: true, Error: "old error"}})
	want := Snapshot{Stale: true, Error: "No data yet"}
	if got := s.Latest(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
