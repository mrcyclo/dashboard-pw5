package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudedash/internal/config"
	"claudedash/internal/media"
	"claudedash/internal/usage"
)

type fakeDisplay struct {
	display config.Display
	refresh string
}

func (f *fakeDisplay) Display() config.Display { return f.display }
func (f *fakeDisplay) RefreshToken() string    { return f.refresh }

func callDisplay(m MediaSource, d DisplaySource, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(func() usage.Snapshot { return usage.Snapshot{} }, m, nil, d)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestKindleJSON(t *testing.T) {
	d := &fakeDisplay{display: config.Display{PanelLighten: 0.85, FullRefreshMinutes: 30, MediaIntervalSeconds: 10}}
	rec := callDisplay(nil, d, http.MethodGet, "/kindle.json")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got map[string]float64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got["panel_lighten"] != 0.85 || got["full_refresh_minutes"] != 30 || got["media_interval_seconds"] != 10 {
		t.Fatalf("got %v", got)
	}
}

func TestKindleJSONWithoutDisplaySource(t *testing.T) {
	if rec := callDisplay(nil, nil, http.MethodGet, "/kindle.json"); rec.Code != 404 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestKindleJSONNeedsGet(t *testing.T) {
	if rec := callDisplay(nil, &fakeDisplay{}, http.MethodPost, "/kindle.json"); rec.Code != 405 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestMediaRepliesCarrySettingsHash(t *testing.T) {
	d := &fakeDisplay{display: config.Default().Display}
	f := &fakeMedia{state: media.State{Active: true}, afterDo: media.State{Active: true}}
	for _, call := range [][2]string{{http.MethodGet, "/media.json"}, {http.MethodPost, "/media/next"}} {
		rec := callDisplay(f, d, call[0], call[1])
		var got map[string]any
		json.Unmarshal(rec.Body.Bytes(), &got)
		if got["settings"] != d.display.Hash() {
			t.Fatalf("%s %s: got %v", call[0], call[1], got)
		}
	}
	d.display.PanelLighten = 0.9
	rec := callDisplay(f, d, http.MethodGet, "/media.json")
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["settings"] != d.display.Hash() {
		t.Fatal("the hash must follow the current settings")
	}
}

// An older plugin knows nothing about settings; its replies must stay as they were.
func TestMediaReplyWithoutDisplaySourceHasNoSettings(t *testing.T) {
	rec := callDisplay(&fakeMedia{state: media.State{Active: true}}, nil, http.MethodGet, "/media.json")
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if _, present := got["settings"]; present {
		t.Fatalf("got %v", got)
	}
}

func TestMediaReplyCarriesRefreshToken(t *testing.T) {
	d := &fakeDisplay{display: config.Default().Display}
	var got map[string]any
	json.Unmarshal(callDisplay(nil, d, http.MethodGet, "/media.json").Body.Bytes(), &got)
	if _, present := got["refresh"]; present {
		t.Fatalf("no token before anyone asked: %v", got)
	}
	d.refresh = "abc"
	json.Unmarshal(callDisplay(nil, d, http.MethodGet, "/media.json").Body.Bytes(), &got)
	if got["refresh"] != "abc" {
		t.Fatalf("got %v", got)
	}
}
