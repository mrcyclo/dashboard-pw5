package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const DefaultEndpoint = "https://api.anthropic.com/api/oauth/usage"

// MessageLoginRequired is the snapshot error shown when the stored login can no longer be refreshed.
const MessageLoginRequired = msgLogin

var (
	ErrTokenExpired  = errors.New("usage: access token expired")
	ErrLoginRequired = errors.New("usage: login required")
	ErrRefreshFailed = errors.New("usage: token refresh failed")
)

const (
	msgNoData      = "No data yet"
	msgExpired     = "Token expired, open Claude Code"
	msgLogin       = "Sign in to Claude again on this PC"
	msgRefresh     = "Could not renew the token"
	msgNoToken     = "Could not read the token"
	msgUnreachable = "Cannot reach Claude"
	msgBadBody     = "The response has an unexpected format"
	msgRateLimited = "Claude is rate limiting, will retry later"

	// How long to stay quiet after a 429 that carries no Retry-After header.
	defaultBackoff = 15 * time.Minute
)

type Options struct {
	Endpoint  string
	TokenFunc func() (string, error)
	Client    *http.Client
	Now       func() time.Time
	// Remembered is the last good snapshot from a previous run, shown as stale
	// until the first fetch, so a restart while rate limited is not a blank screen.
	Remembered Snapshot
}

type Service struct {
	opts Options

	// refreshMu serializes fetches and listener calls so updates arrive in order.
	refreshMu sync.Mutex

	mu        sync.Mutex
	last      Snapshot
	notBefore time.Time
	listeners []func(Snapshot)
}

func New(o Options) *Service {
	if o.Endpoint == "" {
		o.Endpoint = DefaultEndpoint
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 15 * time.Second}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	last := Snapshot{Stale: true, Error: msgNoData}
	if o.Remembered.FetchedAt != 0 && o.Remembered.HasData() {
		last = o.Remembered
		last.Stale, last.Error = true, ""
	}
	return &Service{opts: o, last: last}
}

func (s *Service) OnUpdate(fn func(Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Latest never touches the network, so LAN clients cannot stall on Claude.
func (s *Service) Latest() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *Service) Refresh(ctx context.Context) Snapshot {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	now := s.opts.Now()
	s.mu.Lock()
	// Asking again while rate limited only extends the limit.
	if now.Before(s.notBefore) {
		snap := s.last
		s.mu.Unlock()
		return snap
	}
	s.mu.Unlock()

	fresh, msg, retryAfter := s.fetch(ctx)

	s.mu.Lock()
	if msg == msgRateLimited {
		s.notBefore = now.Add(max(retryAfter, defaultBackoff))
	}
	if msg == "" {
		fresh.FetchedAt = now.Unix()
		s.last = fresh
	} else {
		s.last.Stale = true
		s.last.Error = msg
	}
	snap := s.last
	listeners := append([]func(Snapshot){}, s.listeners...)
	s.mu.Unlock()

	for _, fn := range listeners {
		fn(snap)
	}
	return snap
}

// fetch returns a user-facing message instead of an error so nothing from the
// token, the request or the response body can reach the UI or the log.
func (s *Service) fetch(ctx context.Context) (snap Snapshot, msg string, retryAfter time.Duration) {
	token, err := s.opts.TokenFunc()
	if errors.Is(err, ErrTokenExpired) {
		return Snapshot{}, msgExpired, 0
	}
	if errors.Is(err, ErrLoginRequired) {
		return Snapshot{}, msgLogin, 0
	}
	if errors.Is(err, ErrRefreshFailed) {
		return Snapshot{}, msgRefresh, 0
	}
	if err != nil || token == "" {
		return Snapshot{}, msgNoToken, 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opts.Endpoint, nil)
	if err != nil {
		return Snapshot{}, msgUnreachable, 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claudedash")
	resp, err := s.opts.Client.Do(req)
	if err != nil {
		return Snapshot{}, msgUnreachable, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return Snapshot{}, msgExpired, 0
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return Snapshot{}, msgRateLimited, time.Duration(seconds) * time.Second
	}
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Sprintf("Claude returned error %d", resp.StatusCode), 0
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Snapshot{}, msgUnreachable, 0
	}
	snap, err = Parse(body)
	if err != nil {
		return Snapshot{}, msgBadBody, 0
	}
	return snap, "", 0
}
