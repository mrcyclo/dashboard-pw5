package httpd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claudedash/internal/config"
	"claudedash/internal/media"
	"claudedash/internal/widget"
)

type fakeScreens struct {
	battery    int
	hasBattery bool
	acted      []string
	actErr     error
}

func (f *fakeScreens) Screen(battery int, ok bool) widget.Screen {
	f.battery, f.hasBattery = battery, ok
	return widget.Screen{Layout: "L1", Widgets: []widget.PlacedWidget{{Kind: "clock", Hash: "h1"}}}
}

func (f *fakeScreens) Act(name string) error {
	f.acted = append(f.acted, name)
	return f.actErr
}

func screenServer(f *fakeScreens) http.Handler {
	base := NewHandler(nil, nil, &fakeDisplay{display: config.Default().Display, refresh: "r1"})
	return WithScreens(base, f, nil, &fakeDisplay{display: config.Default().Display, refresh: "r1"})
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestScreenJSONPassesBattery(t *testing.T) {
	f := &fakeScreens{}
	rec := do(screenServer(f), http.MethodGet, "/screen.json?battery=76")
	if rec.Code != 200 || !f.hasBattery || f.battery != 76 {
		t.Fatalf("code %d battery %d %v", rec.Code, f.battery, f.hasBattery)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["layout"] != "L1" || got["refresh"] != "r1" || got["settings"] != config.Default().Display.Hash() {
		t.Fatalf("%v", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("screen replies must not be cached")
	}
}

func TestScreenJSONIgnoresBadBattery(t *testing.T) {
	for _, q := range []string{"", "?battery=abc", "?battery=150", "?battery=-1"} {
		f := &fakeScreens{}
		if rec := do(screenServer(f), http.MethodGet, "/screen.json"+q); rec.Code != 200 || f.hasBattery {
			t.Errorf("%q: code %d battery %v", q, rec.Code, f.hasBattery)
		}
	}
}

func TestActionRunsAndReplies(t *testing.T) {
	f := &fakeScreens{}
	rec := do(screenServer(f), http.MethodPost, "/action/media.next")
	if rec.Code != 200 || len(f.acted) != 1 || f.acted[0] != "media.next" || !strings.Contains(rec.Body.String(), `"layout":"L1"`) {
		t.Fatalf("code %d acted %v body %s", rec.Code, f.acted, rec.Body)
	}
}

func TestActionErrors(t *testing.T) {
	cases := map[error]int{
		widget.ErrUnknownAction: http.StatusNotFound,
		media.ErrNoSession:      http.StatusConflict,
		errors.New("boom"):      http.StatusInternalServerError,
	}
	for err, code := range cases {
		f := &fakeScreens{actErr: err}
		if rec := do(screenServer(f), http.MethodPost, "/action/media.next"); rec.Code != code {
			t.Errorf("%v: code %d want %d", err, rec.Code, code)
		}
	}
}

func TestActionGetNotAllowed(t *testing.T) {
	f := &fakeScreens{}
	if rec := do(screenServer(f), http.MethodGet, "/action/media.next"); rec.Code != http.StatusMethodNotAllowed || len(f.acted) != 0 {
		t.Fatalf("code %d", rec.Code)
	}
	if rec := do(screenServer(f), http.MethodPost, "/action/"); rec.Code != http.StatusNotFound {
		t.Fatalf("empty action: code %d", rec.Code)
	}
}

func TestIconServed(t *testing.T) {
	rec := do(screenServer(&fakeScreens{}), http.MethodGet, "/icons/play.svg")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("ETag") != `"`+widget.IconHash("play")+`"` {
		t.Error("etag")
	}
}

func TestIconUnknown404(t *testing.T) {
	for _, path := range []string{"/icons/nope.svg", "/icons/play.png", "/icons/..%2Fcatalog.svg"} {
		if rec := do(screenServer(&fakeScreens{}), http.MethodGet, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: code %d", path, rec.Code)
		}
	}
}

func TestBaseEndpointsStillServe(t *testing.T) {
	h := screenServer(&fakeScreens{})
	for _, path := range []string{"/kindle.json"} {
		if rec := do(h, http.MethodGet, path); rec.Code != 200 {
			t.Errorf("%s: code %d", path, rec.Code)
		}
	}
}
