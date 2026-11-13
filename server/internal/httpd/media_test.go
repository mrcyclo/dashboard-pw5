package httpd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeMedia struct {
	art  []byte
	hash string
}

func (f *fakeMedia) Art() ([]byte, string, bool) { return f.art, f.hash, f.hash != "" }

func call(m MediaSource, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(m, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestArtWithoutMediaSupport(t *testing.T) {
	if got := call(nil, http.MethodGet, "/media/art.png").Code; got != 404 {
		t.Fatalf("code %d", got)
	}
}

// The KOReader plugin's endpoints are gone; widget buttons go through /action/.
func TestPluginEndpointsAreGone(t *testing.T) {
	for _, c := range [][2]string{{http.MethodGet, "/usage.json"}, {http.MethodGet, "/media.json"}, {http.MethodPost, "/media/next"}} {
		if rec := call(&fakeMedia{}, c[0], c[1]); rec.Code != 404 {
			t.Errorf("%s %s: code %d", c[0], c[1], rec.Code)
		}
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
