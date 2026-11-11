package tokensource

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"claudedash/internal/credentials"
	"claudedash/internal/usage"
)

var now = time.Unix(1794400000, 0)

func credFile(t *testing.T, expiresAt time.Time, refreshToken string) string {
	t.Helper()
	body := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "old-access",
			"refreshToken": refreshToken,
			"expiresAt":    expiresAt.UnixMilli(),
			"scopes":       []string{"user:inference", "user:profile"},
		},
		"organizationUuid": "org-1",
	}
	data, _ := json.Marshal(body)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type fakeOAuth struct {
	srv      *httptest.Server
	requests atomic.Int32
	status   int
	response string
	lastBody atomic.Value
	lastType atomic.Value
}

func newFakeOAuth(t *testing.T, status int, response string) *fakeOAuth {
	f := &fakeOAuth{status: status, response: response}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		f.lastBody.Store(string(body))
		f.lastType.Store(r.Method + " " + r.Header.Get("Content-Type"))
		w.WriteHeader(f.status)
		w.Write([]byte(f.response))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func source(path string, f *fakeOAuth) *Source {
	return &Source{Path: path, Endpoint: f.srv.URL, Now: func() time.Time { return now }}
}

const goodResponse = `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":28800,"refresh_token_expires_in":2592000,"scope":"user:inference user:profile"}`

func TestValidTokenIsReturnedWithoutRefreshing(t *testing.T) {
	path := credFile(t, now.Add(10*time.Minute), "old-refresh")
	before, _ := os.ReadFile(path)
	f := newFakeOAuth(t, 200, goodResponse)
	got, err := source(path, f).Token(context.Background())
	if err != nil || got != "old-access" {
		t.Fatalf("got %q err %v", got, err)
	}
	after, _ := os.ReadFile(path)
	if f.requests.Load() != 0 || string(before) != string(after) {
		t.Fatal("a valid token must not trigger a refresh or a write")
	}
}

func TestExpiredTokenIsRefreshedAndSaved(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 200, goodResponse)
	got, err := source(path, f).Token(context.Background())
	if err != nil || got != "new-access" {
		t.Fatalf("got %q err %v", got, err)
	}

	var sent map[string]string
	if err := json.Unmarshal([]byte(f.lastBody.Load().(string)), &sent); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": "old-refresh",
		"client_id":     "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		"scope":         "user:inference user:profile",
	}
	for key, value := range want {
		if sent[key] != value {
			t.Errorf("request %s = %q, want %q", key, sent[key], value)
		}
	}
	if f.lastType.Load() != "POST application/json" {
		t.Errorf("request was %v", f.lastType.Load())
	}

	saved, err := credentials.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "new-access" || saved.RefreshToken != "new-refresh" {
		t.Fatal("new tokens were not saved")
	}
	if !saved.ExpiresAt.Equal(now.Add(28800 * time.Second)) {
		t.Fatalf("expiry %v", saved.ExpiresAt)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"organizationUuid":"org-1"`) {
		t.Fatal("unrelated fields were dropped")
	}
	wantRefreshExpiry := now.Add(2592000 * time.Second).UnixMilli()
	var file struct {
		OAuth struct {
			RefreshTokenExpiresAt int64 `json:"refreshTokenExpiresAt"`
		} `json:"claudeAiOauth"`
	}
	json.Unmarshal(raw, &file)
	if file.OAuth.RefreshTokenExpiresAt != wantRefreshExpiry {
		t.Fatalf("refresh expiry %d want %d", file.OAuth.RefreshTokenExpiresAt, wantRefreshExpiry)
	}
}

func TestTokenAboutToExpireIsRefreshed(t *testing.T) {
	path := credFile(t, now.Add(30*time.Second), "old-refresh")
	f := newFakeOAuth(t, 200, goodResponse)
	if got, _ := source(path, f).Token(context.Background()); got != "new-access" {
		t.Fatalf("got %q", got)
	}
}

func TestSecondCallUsesSavedTokenWithoutRefreshingAgain(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 200, goodResponse)
	s := source(path, f)
	s.Token(context.Background())
	if got, _ := s.Token(context.Background()); got != "new-access" || f.requests.Load() != 1 {
		t.Fatalf("got %q after %d refreshes", got, f.requests.Load())
	}
}

func TestResponseWithoutRefreshTokenKeepsOldOne(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 200, `{"access_token":"new-access","expires_in":28800}`)
	if _, err := source(path, f).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, _ := credentials.Load(path)
	if saved.AccessToken != "new-access" || saved.RefreshToken != "old-refresh" {
		t.Fatal("old refresh token should be kept")
	}
}

func assertFailsCleanly(t *testing.T, path string, s *Source, want error) {
	t.Helper()
	before, _ := os.ReadFile(path)
	got, err := s.Token(context.Background())
	if got != "" || !errors.Is(err, want) {
		t.Fatalf("got %q err %v, want %v", got, err, want)
	}
	for _, secret := range []string{"old-access", "old-refresh", "new-access", "new-refresh", "leak-me"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks %q: %v", secret, err)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("credentials file changed on a failed refresh")
	}
}

func TestRejectedRefreshMeansLoginRequired(t *testing.T) {
	for _, status := range []int{400, 401, 403} {
		path := credFile(t, now.Add(-time.Minute), "old-refresh")
		f := newFakeOAuth(t, status, `{"error":"invalid_grant","error_description":"leak-me"}`)
		assertFailsCleanly(t, path, source(path, f), usage.ErrLoginRequired)
	}
}

func TestServerErrorIsTemporary(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 503, "leak-me")
	assertFailsCleanly(t, path, source(path, f), usage.ErrRefreshFailed)
}

func TestUnreachableServerIsTemporary(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 200, goodResponse)
	s := source(path, f)
	f.srv.Close()
	assertFailsCleanly(t, path, s, usage.ErrRefreshFailed)
}

func TestUnusableResponseDoesNotTouchFile(t *testing.T) {
	bodies := []string{
		`<html>`,
		`{"refresh_token":"new-refresh","expires_in":28800}`,
		`{"access_token":"new-access","refresh_token":"new-refresh"}`,
		`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":0}`,
	}
	for _, body := range bodies {
		path := credFile(t, now.Add(-time.Minute), "old-refresh")
		f := newFakeOAuth(t, 200, body)
		assertFailsCleanly(t, path, source(path, f), usage.ErrRefreshFailed)
	}
}

func TestExpiredWithoutRefreshTokenMeansLoginRequired(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "")
	f := newFakeOAuth(t, 200, goodResponse)
	assertFailsCleanly(t, path, source(path, f), usage.ErrLoginRequired)
	if f.requests.Load() != 0 {
		t.Fatal("must not call the server without a refresh token")
	}
}

func TestMissingFile(t *testing.T) {
	f := newFakeOAuth(t, 200, goodResponse)
	s := source(filepath.Join(t.TempDir(), "nope.json"), f)
	if _, err := s.Token(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
}

// A refresh token is single use, so two callers must never both spend it.
func TestConcurrentCallersRefreshOnce(t *testing.T) {
	path := credFile(t, now.Add(-time.Minute), "old-refresh")
	f := newFakeOAuth(t, 200, goodResponse)
	s := source(path, f)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := s.Token(context.Background()); err != nil || got != "new-access" {
				t.Errorf("got %q err %v", got, err)
			}
		}()
	}
	wg.Wait()
	if f.requests.Load() != 1 {
		t.Fatalf("refreshed %d times", f.requests.Load())
	}
}

func TestStatusReadsExpiryWithoutRefreshing(t *testing.T) {
	path := credFile(t, now.Add(-time.Hour), "old-refresh")
	before, _ := os.ReadFile(path)
	f := newFakeOAuth(t, 200, goodResponse)
	expires, err := source(path, f).Status()
	if err != nil || !expires.Equal(now.Add(-time.Hour)) {
		t.Fatalf("got %v err %v", expires, err)
	}
	after, _ := os.ReadFile(path)
	if f.requests.Load() != 0 || string(before) != string(after) {
		t.Fatal("Status must only read")
	}
}

func TestStatusMissingFile(t *testing.T) {
	f := newFakeOAuth(t, 200, goodResponse)
	if _, err := source(filepath.Join(t.TempDir(), "nope.json"), f).Status(); err == nil {
		t.Fatal("want error")
	}
}
