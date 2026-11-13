package httpd

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"claudedash/internal/widget"
)

// ScreenSource builds what the Kindle draws and runs its buttons' actions.
type ScreenSource interface {
	// Screen builds the reply; battery is the Kindle's charge when ok.
	Screen(battery int, ok bool) widget.Screen
	Act(name string) error
	// AppIcon is the picture of an apps widget tile, and its hash.
	AppIcon(id string) (pngData []byte, hash string, ok bool)
}

// WithScreens adds the widget endpoints in front of base: /screen.json,
// /action/<name> and /icons/<name>.svg.
func WithScreens(base http.Handler, s ScreenSource, bg BackgroundSource, d DisplaySource) http.Handler {
	reply := func(w http.ResponseWriter, battery int, ok bool) {
		out := s.Screen(battery, ok)
		if bg != nil {
			out.Background = bg.Hash()
		}
		if d != nil {
			out.Settings = d.Display().Hash()
			out.Refresh = d.RefreshToken()
		}
		writeJSON(w, out)
	}

	mux := http.NewServeMux()
	mux.Handle("/", base)
	mux.HandleFunc("/screen.json", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		battery, err := strconv.Atoi(r.URL.Query().Get("battery"))
		ok := err == nil && battery >= 0 && battery <= 100
		reply(w, battery, ok)
	})
	mux.HandleFunc("/action/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/action/")
		if name == "" || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		if !allow(w, r, http.MethodPost) {
			return
		}
		if err := s.Act(name); err != nil {
			http.Error(w, "action failed", actionStatus(err))
			return
		}
		reply(w, 0, false)
	})
	mux.HandleFunc("/apps/", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/apps/")
		id, isPNG := strings.CutSuffix(name, ".png")
		if !isPNG || id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		data, hash, ok := s.AppIcon(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"`+hash+`"`)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	})
	mux.HandleFunc("/icons/", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r, http.MethodGet) {
			return
		}
		name, isSVG := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/icons/"), ".svg")
		data, ok := widget.IconSVG(name)
		if !isSVG || !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("ETag", `"`+widget.IconHash(name)+`"`)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	})
	return mux
}

func actionStatus(err error) int {
	if errors.Is(err, widget.ErrUnknownAction) {
		return http.StatusNotFound
	}
	return commandStatus(err)
}
