package httpd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"claudedash/internal/config"
	"claudedash/internal/media"
	"claudedash/internal/usage"
)

type MediaSource interface {
	Snapshot() media.State
	Art() (pngData []byte, hash string, ok bool)
	Do(cmd string) (media.State, error)
}

type BackgroundSource interface {
	// Hash is "" when no picture is set.
	Hash() string
	Render(width, height int) (pngData []byte, ok bool)
	// Source is the whole picture, uncropped, for the window's preview.
	Source(maxSide int) (pngData []byte, ok bool)
}

// Longest edge of the uncropped preview picture.
const previewSide = 1200

type DisplaySource interface {
	Display() config.Display
	// RefreshToken changes when the user asks for a full repaint; "" means never asked.
	RefreshToken() string
}

// The background and settings hashes ride along with the media state because
// that is the reply the Kindle already polls every few seconds.
type mediaReply struct {
	media.State
	Background string `json:"background,omitempty"`
	Settings   string `json:"settings,omitempty"`
	Refresh    string `json:"refresh,omitempty"`
}

// NewHandler serves usage, media, the backdrop picture and display settings to
// the LAN. Any nil source leaves the remaining endpoints working.
func NewHandler(latest func() usage.Snapshot, m MediaSource, bg BackgroundSource, d DisplaySource) http.Handler {
	reply := func(state media.State) mediaReply {
		out := mediaReply{State: state}
		if bg != nil {
			out.Background = bg.Hash()
		}
		if d != nil {
			out.Settings = d.Display().Hash()
			out.Refresh = d.RefreshToken()
		}
		return out
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/background-source.png", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		if bg == nil {
			http.NotFound(w, r)
			return
		}
		pngData, ok := bg.Source(previewSide)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(pngData)
	})
	mux.HandleFunc("/kindle.json", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		if d == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, d.Display())
	})
	mux.HandleFunc("/background.png", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		width, errW := strconv.Atoi(r.URL.Query().Get("w"))
		height, errH := strconv.Atoi(r.URL.Query().Get("h"))
		if errW != nil || errH != nil || width < 1 || height < 1 {
			http.Error(w, "w and h must be positive integers", http.StatusBadRequest)
			return
		}
		if bg == nil {
			http.NotFound(w, r)
			return
		}
		pngData, ok := bg.Render(width, height)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"`+bg.Hash()+`"`)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(pngData)
	})
	mux.HandleFunc("/usage.json", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, latest())
	})
	mux.HandleFunc("/media.json", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		if m == nil {
			writeJSON(w, reply(media.State{}))
			return
		}
		writeJSON(w, reply(m.Snapshot()))
	})
	if m == nil {
		return mux
	}
	mux.HandleFunc("/media/art.png", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		pngData, hash, ok := m.Art()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"`+hash+`"`)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(pngData)
	})
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		cmd := strings.TrimPrefix(r.URL.Path, "/media/")
		if cmd == "" || strings.Contains(cmd, "/") {
			http.NotFound(w, r)
			return
		}
		if !allow(w, r, http.MethodPost) {
			return
		}
		state, err := m.Do(cmd)
		if err != nil {
			http.Error(w, "command failed", commandStatus(err))
			return
		}
		writeJSON(w, reply(state))
	})
	return mux
}

func commandStatus(err error) int {
	if errors.Is(err, media.ErrNoSession) {
		return http.StatusConflict
	}
	if errors.Is(err, media.ErrUnknownCommand) {
		return http.StatusNotFound
	}
	if errors.Is(err, media.ErrNoMixer) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func allow(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
