package tokensource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"claudedash/internal/credentials"
	"claudedash/internal/usage"
)

const (
	DefaultEndpoint = "https://platform.claude.com/v1/oauth/token"

	// Claude Code's own OAuth client: the refresh token in its login file is bound to it.
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// Refreshing slightly early avoids handing out a token that dies mid-request.
	expiryMargin = time.Minute
)

type Source struct {
	Path     string
	Endpoint string
	Client   *http.Client
	Now      func() time.Time

	mu sync.Mutex
}

// Token returns a usable access token, refreshing and saving it when the one on
// disk has expired. Errors never contain token material or server responses.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-reading on every call picks up a token Claude Code refreshed by itself.
	token, err := credentials.Load(s.Path)
	if err != nil {
		return "", err
	}
	now := s.now()
	if !token.Expired(now.Add(expiryMargin)) {
		return token.AccessToken, nil
	}
	if token.RefreshToken == "" {
		return "", usage.ErrLoginRequired
	}

	fresh, err := s.refresh(ctx, token)
	if err != nil {
		return "", err
	}
	update := credentials.Update{
		AccessToken:  fresh.AccessToken,
		RefreshToken: fresh.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(fresh.ExpiresIn) * time.Second),
	}
	if update.RefreshToken == "" {
		update.RefreshToken = token.RefreshToken
	}
	if fresh.RefreshExpiresIn > 0 {
		update.RefreshExpiresAt = now.Add(time.Duration(fresh.RefreshExpiresIn) * time.Second)
	}
	// The old refresh token is already spent, so a failed save costs the login.
	if err := credentials.Save(s.Path, update); err != nil {
		return "", fmt.Errorf("%w: could not save refreshed token", usage.ErrRefreshFailed)
	}
	return fresh.AccessToken, nil
}

// Status reports when the stored access token expires, without refreshing it.
func (s *Source) Status() (expiresAt time.Time, err error) {
	token, err := credentials.Load(s.Path)
	if err != nil {
		return time.Time{}, err
	}
	return token.ExpiresAt, nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_token_expires_in"`
}

func (s *Source) refresh(ctx context.Context, token credentials.Token) (tokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": token.RefreshToken,
		"client_id":     clientID,
	}
	if len(token.Scopes) > 0 {
		payload["scope"] = strings.Join(token.Scopes, " ")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return tokenResponse{}, usage.ErrRefreshFailed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint(), bytes.NewReader(body))
	if err != nil {
		return tokenResponse{}, usage.ErrRefreshFailed
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claudedash")

	resp, err := s.client().Do(req)
	if err != nil {
		return tokenResponse{}, usage.ErrRefreshFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest ||
		resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden {
		return tokenResponse{}, fmt.Errorf("%w: refresh rejected with status %d", usage.ErrLoginRequired, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("%w: status %d", usage.ErrRefreshFailed, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, usage.ErrRefreshFailed
	}
	var fresh tokenResponse
	if json.Unmarshal(raw, &fresh) != nil || fresh.AccessToken == "" || fresh.ExpiresIn <= 0 {
		return tokenResponse{}, fmt.Errorf("%w: unusable response", usage.ErrRefreshFailed)
	}
	return fresh, nil
}

func (s *Source) endpoint() string {
	if s.Endpoint == "" {
		return DefaultEndpoint
	}
	return s.Endpoint
}

func (s *Source) client() *http.Client {
	if s.Client == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	return s.Client
}

func (s *Source) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}
