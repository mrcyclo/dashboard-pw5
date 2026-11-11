package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadReadsTokenAndExpiry(t *testing.T) {
	tok, err := Load(write(t, `{"claudeAiOauth":{"accessToken":"tok","expiresAt":1794400000000}}`))
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok" || tok.ExpiresAt.Unix() != 1794400000 {
		t.Fatalf("got %q %d", tok.AccessToken, tok.ExpiresAt.Unix())
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadTruncatedJSON(t *testing.T) {
	if _, err := Load(write(t, `{"claudeAiOauth":{"acc`)); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadEmptyToken(t *testing.T) {
	if _, err := Load(write(t, `{"claudeAiOauth":{"accessToken":"","expiresAt":1794400000000}}`)); err == nil {
		t.Fatal("want error")
	}
}

func TestErrorNeverContainsToken(t *testing.T) {
	_, err := Load(write(t, `{"claudeAiOauth":{"accessToken":"sk-secret","expiresAt":"soon"}}`))
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestExpired(t *testing.T) {
	tok := Token{AccessToken: "t", ExpiresAt: time.Unix(1794400000, 0)}
	if !tok.Expired(tok.ExpiresAt.Add(time.Second)) {
		t.Fatal("want expired after expiry")
	}
	if tok.Expired(tok.ExpiresAt.Add(-time.Second)) {
		t.Fatal("want valid before expiry")
	}
}

const fullFile = `{"claudeAiOauth":{"accessToken":"old-access","refreshToken":"old-refresh","expiresAt":1794400000000,"refreshTokenExpiresAt":1796600000000,"scopes":["user:inference","user:profile"],"subscriptionType":"max","rateLimitTier":"tier_x"},"organizationUuid":"org-1","futureField":{"keep":9007199254740993}}`

func TestLoadReadsRefreshTokenAndScopes(t *testing.T) {
	tok, err := Load(write(t, fullFile))
	if err != nil {
		t.Fatal(err)
	}
	if tok.RefreshToken != "old-refresh" || strings.Join(tok.Scopes, " ") != "user:inference user:profile" {
		t.Fatalf("got %+v", tok.Scopes)
	}
}

func readBack(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSaveUpdatesTokensAndKeepsEverythingElse(t *testing.T) {
	path := write(t, fullFile)
	err := Save(path, Update{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresAt:        time.UnixMilli(1794428800000),
		RefreshExpiresAt: time.UnixMilli(1799999999000),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := readBack(t, path)
	oauth := got["claudeAiOauth"].(map[string]any)
	want := map[string]string{
		"accessToken": "new-access", "refreshToken": "new-refresh",
		"expiresAt": "1794428800000", "refreshTokenExpiresAt": "1799999999000",
		"subscriptionType": "max", "rateLimitTier": "tier_x",
	}
	for key, value := range want {
		if fmt.Sprint(oauth[key]) != value {
			t.Errorf("%s = %v, want %s", key, oauth[key], value)
		}
	}
	if fmt.Sprint(oauth["scopes"]) != "[user:inference user:profile]" {
		t.Errorf("scopes %v", oauth["scopes"])
	}
	if got["organizationUuid"] != "org-1" {
		t.Errorf("organizationUuid %v", got["organizationUuid"])
	}
	if fmt.Sprint(got["futureField"].(map[string]any)["keep"]) != "9007199254740993" {
		t.Errorf("large number mangled: %v", got["futureField"])
	}
}

func TestSaveKeepsRefreshExpiryWhenNotProvided(t *testing.T) {
	path := write(t, fullFile)
	if err := Save(path, Update{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.UnixMilli(1)}); err != nil {
		t.Fatal(err)
	}
	oauth := readBack(t, path)["claudeAiOauth"].(map[string]any)
	if fmt.Sprint(oauth["refreshTokenExpiresAt"]) != "1796600000000" {
		t.Fatalf("got %v", oauth["refreshTokenExpiresAt"])
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	path := write(t, fullFile)
	if err := Save(path, Update{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.UnixMilli(1)}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("extra files left: %v", entries)
	}
}

func TestSaveRejectsEmptyTokensAndLeavesFileAlone(t *testing.T) {
	path := write(t, fullFile)
	if err := Save(path, Update{AccessToken: "", RefreshToken: "r", ExpiresAt: time.UnixMilli(1)}); err == nil {
		t.Fatal("want error")
	}
	if err := Save(path, Update{AccessToken: "a", RefreshToken: "", ExpiresAt: time.UnixMilli(1)}); err == nil {
		t.Fatal("want error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != fullFile {
		t.Fatal("file was modified")
	}
}

func TestSaveOnMalformedFileLeavesItAlone(t *testing.T) {
	path := write(t, `{"claudeAiOauth":{"acc`)
	if err := Save(path, Update{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.UnixMilli(1)}); err == nil {
		t.Fatal("want error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"claudeAiOauth":{"acc` {
		t.Fatal("file was modified")
	}
}
