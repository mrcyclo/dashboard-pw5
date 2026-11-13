package credentials

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var ErrMalformed = errors.New("credentials: malformed file")

type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
}

type Update struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	// Zero keeps the value already in the file.
	RefreshExpiresAt time.Time
}

func (t Token) Expired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

func Load(path string) (Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Token{}, err
	}
	var file struct {
		OAuth struct {
			AccessToken  string   `json:"accessToken"`
			RefreshToken string   `json:"refreshToken"`
			ExpiresAt    int64    `json:"expiresAt"`
			Scopes       []string `json:"scopes"`
		} `json:"claudeAiOauth"`
	}
	// The decode error is dropped on purpose: it can quote file content.
	if json.Unmarshal(data, &file) != nil {
		return Token{}, ErrMalformed
	}
	if file.OAuth.AccessToken == "" || file.OAuth.ExpiresAt <= 0 {
		return Token{}, ErrMalformed
	}
	return Token{
		AccessToken:  file.OAuth.AccessToken,
		RefreshToken: file.OAuth.RefreshToken,
		ExpiresAt:    time.UnixMilli(file.OAuth.ExpiresAt),
		Scopes:       file.OAuth.Scopes,
	}, nil
}

// Save rewrites only the token fields and keeps every other field byte for byte,
// because Claude Code owns this file and may add fields this program does not know.
func Save(path string, u Update) error {
	if u.AccessToken == "" || u.RefreshToken == "" {
		return errors.New("credentials: refusing to save an empty token")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file map[string]json.RawMessage
	if json.Unmarshal(data, &file) != nil {
		return ErrMalformed
	}
	var oauth map[string]json.RawMessage
	if json.Unmarshal(file[oauthKey], &oauth) != nil || oauth == nil {
		return ErrMalformed
	}

	oauth["accessToken"] = mustJSON(u.AccessToken)
	oauth["refreshToken"] = mustJSON(u.RefreshToken)
	oauth["expiresAt"] = mustJSON(u.ExpiresAt.UnixMilli())
	if !u.RefreshExpiresAt.IsZero() {
		oauth["refreshTokenExpiresAt"] = mustJSON(u.RefreshExpiresAt.UnixMilli())
	}
	file[oauthKey] = mustJSON(oauth)
	return writeAtomic(path, mustJSON(file))
}

const oauthKey = "claudeAiOauth"

func mustJSON(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return out
}

// A crash mid-write must never leave a truncated login file, so the new content
// goes to a sibling temp file that then replaces the original in one rename.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
