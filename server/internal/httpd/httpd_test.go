package httpd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOtherPaths404(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	for _, path := range []string{"/", "/usage.json/x", "/.credentials.json", "/usage"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 404 {
			t.Errorf("%s: code %d", path, rec.Code)
		}
	}
}
