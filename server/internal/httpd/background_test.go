package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudedash/internal/media"
	"claudedash/internal/usage"
)

type fakeBackground struct {
	hash     string
	rendered [][2]int
}

func (f *fakeBackground) Hash() string { return f.hash }

func (f *fakeBackground) Render(width, height int) ([]byte, bool) {
	f.rendered = append(f.rendered, [2]int{width, height})
	if f.hash == "" || width > 4096 || height > 4096 {
		return nil, false
	}
	return []byte("\x89PNG-background"), true
}

func (f *fakeBackground) Source(maxSide int) ([]byte, bool) {
	if f.hash == "" {
		return nil, false
	}
	return []byte("\x89PNG-source"), true
}

func TestBackgroundSource(t *testing.T) {
	rec := callWith(nil, &fakeBackground{hash: "abc123abc123"}, http.MethodGet, "/background-source.png")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "\x89PNG-source" {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("no web page should be able to read the picture; showing it in an <img> needs no CORS header")
	}
	if got := callWith(nil, &fakeBackground{}, http.MethodGet, "/background-source.png").Code; got != 404 {
		t.Fatalf("no picture: code %d", got)
	}
	if got := callWith(nil, nil, http.MethodGet, "/background-source.png").Code; got != 404 {
		t.Fatalf("no background support: code %d", got)
	}
	if got := callWith(nil, &fakeBackground{hash: "x"}, http.MethodPost, "/background-source.png").Code; got != 405 {
		t.Fatalf("post: code %d", got)
	}
}

func callWith(m MediaSource, bg BackgroundSource, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(func() usage.Snapshot { return usage.Snapshot{} }, m, bg, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestBackgroundPNG(t *testing.T) {
	bg := &fakeBackground{hash: "abc123abc123"}
	rec := callWith(nil, bg, http.MethodGet, "/background.png?w=1236&h=1648")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "\x89PNG-background" {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("ETag") != `"abc123abc123"` {
		t.Fatalf("etag %q", rec.Header().Get("ETag"))
	}
	if len(bg.rendered) != 1 || bg.rendered[0] != [2]int{1236, 1648} {
		t.Fatalf("rendered %v", bg.rendered)
	}
}

func TestBackgroundMissing(t *testing.T) {
	if rec := callWith(nil, &fakeBackground{}, http.MethodGet, "/background.png?w=600&h=800"); rec.Code != 404 {
		t.Fatalf("code %d", rec.Code)
	}
	if rec := callWith(nil, nil, http.MethodGet, "/background.png?w=600&h=800"); rec.Code != 404 {
		t.Fatalf("without background support: code %d", rec.Code)
	}
}

func TestBackgroundBadSize(t *testing.T) {
	bg := &fakeBackground{hash: "abc123abc123"}
	for _, query := range []string{"", "?w=600", "?w=abc&h=800", "?w=0&h=800", "?w=-5&h=800", "?w=600&h=1e9"} {
		if rec := callWith(nil, bg, http.MethodGet, "/background.png"+query); rec.Code != 400 {
			t.Errorf("%q: code %d", query, rec.Code)
		}
	}
	if len(bg.rendered) != 0 {
		t.Fatalf("bad sizes must not reach the renderer: %v", bg.rendered)
	}
	if rec := callWith(nil, bg, http.MethodGet, "/background.png?w=9000&h=9000"); rec.Code != 404 {
		t.Fatalf("a size the renderer refuses: code %d", rec.Code)
	}
}

func TestBackgroundNeedsGet(t *testing.T) {
	if rec := callWith(nil, &fakeBackground{hash: "x"}, http.MethodPost, "/background.png?w=1&h=1"); rec.Code != 405 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestMediaJSONCarriesBackgroundHash(t *testing.T) {
	f := &fakeMedia{state: media.State{Active: true, Title: "Song"}}
	rec := callWith(f, &fakeBackground{hash: "abc123abc123"}, http.MethodGet, "/media.json")
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["background"] != "abc123abc123" || got["title"] != "Song" || got["active"] != true {
		t.Fatalf("got %v", got)
	}
}

func TestMediaJSONWithoutBackgroundOmitsField(t *testing.T) {
	f := &fakeMedia{state: media.State{Active: true}}
	for _, bg := range []BackgroundSource{nil, &fakeBackground{}} {
		rec := callWith(f, bg, http.MethodGet, "/media.json")
		var got map[string]any
		json.Unmarshal(rec.Body.Bytes(), &got)
		if _, present := got["background"]; present {
			t.Fatalf("got %v", got)
		}
	}
}

func TestCommandReplyCarriesBackgroundHashToo(t *testing.T) {
	f := &fakeMedia{afterDo: media.State{Active: true}}
	rec := callWith(f, &fakeBackground{hash: "abc123abc123"}, http.MethodPost, "/media/next")
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["background"] != "abc123abc123" {
		t.Fatalf("a command reply without the hash would make the Kindle drop its background: %v", got)
	}
}
