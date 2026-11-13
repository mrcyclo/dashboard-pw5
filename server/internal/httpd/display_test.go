package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudedash/internal/config"
)

type fakeDisplay struct {
	display config.Display
	refresh string
}

func (f *fakeDisplay) Display() config.Display { return f.display }
func (f *fakeDisplay) RefreshToken() string    { return f.refresh }

func callDisplay(m MediaSource, d DisplaySource, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(m, nil, d)
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
