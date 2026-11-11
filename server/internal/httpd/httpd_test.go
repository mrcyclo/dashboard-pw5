package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudedash/internal/usage"
)

func serve(snap usage.Snapshot, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(func() usage.Snapshot { return snap }, nil, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestUsageJSON(t *testing.T) {
	snap := usage.Snapshot{FetchedAt: 1794400000, FiveHour: &usage.Window{UsedPercent: 42, ResetsAt: 1794408000}}
	rec := serve(snap, http.MethodGet, "/usage.json")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := `{"fetched_at":1794400000,"stale":false,"five_hour":{"used_percent":42,"resets_at":1794408000}}`
	var got, exp any
	json.Unmarshal(rec.Body.Bytes(), &got)
	json.Unmarshal([]byte(want), &exp)
	gotJSON, _ := json.Marshal(got)
	expJSON, _ := json.Marshal(exp)
	if string(gotJSON) != string(expJSON) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestStaleStill200(t *testing.T) {
	rec := serve(usage.Snapshot{Stale: true, Error: "Cannot reach Claude"}, http.MethodGet, "/usage.json")
	var got usage.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !got.Stale || got.Error != "Cannot reach Claude" {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOtherPaths404(t *testing.T) {
	for _, path := range []string{"/", "/usage.json/x", "/.credentials.json", "/usage"} {
		if rec := serve(usage.Snapshot{}, http.MethodGet, path); rec.Code != 404 {
			t.Errorf("%s: code %d", path, rec.Code)
		}
	}
}

func TestNonGet405(t *testing.T) {
	if rec := serve(usage.Snapshot{}, http.MethodPost, "/usage.json"); rec.Code != 405 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestNoCacheHeader(t *testing.T) {
	if rec := serve(usage.Snapshot{}, http.MethodGet, "/usage.json"); rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("got %q", rec.Header().Get("Cache-Control"))
	}
}
