package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudedash/internal/media"
	"claudedash/internal/usage"
)

type fakeMedia struct {
	state   media.State
	art     []byte
	hash    string
	doErr   error
	did     []string
	afterDo media.State
}

func (f *fakeMedia) Snapshot() media.State { return f.state }

func (f *fakeMedia) Art() ([]byte, string, bool) { return f.art, f.hash, f.hash != "" }

func (f *fakeMedia) Do(cmd string) (media.State, error) {
	f.did = append(f.did, cmd)
	if f.doErr != nil {
		return media.State{}, f.doErr
	}
	return f.afterDo, nil
}

func call(m MediaSource, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(func() usage.Snapshot { return usage.Snapshot{} }, m, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestMediaJSON(t *testing.T) {
	f := &fakeMedia{state: media.State{Active: true, Title: "Song", Playing: true}}
	rec := call(f, http.MethodGet, "/media.json")
	var got media.State
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || got.Title != "Song" || !got.Playing {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("media state must not be cached")
	}
}

func TestMediaJSONWithoutMediaSupport(t *testing.T) {
	rec := call(nil, http.MethodGet, "/media.json")
	if rec.Code != 200 || rec.Body.String() != "{\"active\":false}\n" {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/media/art.png"} {
		if got := call(nil, http.MethodGet, path).Code; got != 404 {
			t.Errorf("%s: %d", path, got)
		}
	}
	if got := call(nil, http.MethodPost, "/media/next").Code; got != 404 {
		t.Fatalf("command without media support: %d", got)
	}
}

func TestArt(t *testing.T) {
	f := &fakeMedia{art: []byte("\x89PNG-data"), hash: "9f2c1ab04d7e"}
	rec := call(f, http.MethodGet, "/media/art.png")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "\x89PNG-data" {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("ETag") != `"9f2c1ab04d7e"` {
		t.Fatalf("etag %q", rec.Header().Get("ETag"))
	}
}

func TestArtMissing(t *testing.T) {
	if rec := call(&fakeMedia{}, http.MethodGet, "/media/art.png"); rec.Code != 404 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestCommandReturnsNewState(t *testing.T) {
	f := &fakeMedia{afterDo: media.State{Active: true, Title: "Next song"}}
	for _, cmd := range []string{"play-pause", "next", "previous", "volume-up", "volume-down"} {
		rec := call(f, http.MethodPost, "/media/"+cmd)
		var got media.State
		json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != 200 || got.Title != "Next song" {
			t.Fatalf("%s: code %d body %s", cmd, rec.Code, rec.Body.String())
		}
	}
	if len(f.did) != 5 || f.did[1] != "next" {
		t.Fatalf("did %v", f.did)
	}
}

func TestCommandErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{media.ErrNoSession, 409},
		{media.ErrUnknownCommand, 404},
		{media.ErrNoMixer, 503},
		{http.ErrAbortHandler, 500},
	}
	for _, c := range cases {
		if rec := call(&fakeMedia{doErr: c.err}, http.MethodPost, "/media/next"); rec.Code != c.want {
			t.Errorf("%v: code %d want %d", c.err, rec.Code, c.want)
		}
	}
}

func TestCommandNeedsPost(t *testing.T) {
	f := &fakeMedia{}
	if rec := call(f, http.MethodGet, "/media/next"); rec.Code != 405 {
		t.Fatalf("code %d", rec.Code)
	}
	if len(f.did) != 0 {
		t.Fatal("a GET must never run a command")
	}
}

func TestNestedMediaPathsAre404(t *testing.T) {
	f := &fakeMedia{}
	for _, path := range []string{"/media", "/media/", "/media/next/extra", "/media/../usage.json"} {
		if rec := call(f, http.MethodPost, path); rec.Code == 200 {
			t.Errorf("%s: unexpectedly OK", path)
		}
	}
	if len(f.did) != 0 {
		t.Fatalf("ran %v", f.did)
	}
}
