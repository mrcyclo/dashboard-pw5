package main

import (
	"context"
	"os"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"claudedash/internal/desk"
	"claudedash/internal/tray"
	"claudedash/internal/usage"
)

// Development aid: set to "#kindle", "#settings" or "#logs" to open the window on that page.
const startPageEnv = "CLAUDEDASH_START_PAGE"

// shell is the tray icon and the window around the desk service.
type shell struct {
	wails *application.App
	app   *desk.App

	showing showGuard
	// nil while the app lives only in the tray.
	window atomic.Pointer[application.WebviewWindow]
}

func (s *shell) runTray(service *usage.Service) {
	menu := s.wails.NewMenu()
	menu.Add("Open Claude Dashboard").OnClick(func(*application.Context) { s.showWindow() })
	menu.Add("Refresh usage").OnClick(func(*application.Context) {
		go service.Refresh(context.Background())
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { s.wails.Quit() })

	icon := s.wails.SystemTray.New().
		SetIcon(tray.Icon()).
		SetMenu(menu).
		OnClick(s.showWindow).
		OnDoubleClick(s.showWindow)
	icon.SetTooltip("Claude Dashboard")

	tooltip := newLatest(func(text string) { icon.SetTooltip(text) })
	service.OnUpdate(func(snapshot usage.Snapshot) { tooltip.set(tray.Tooltip(snapshot)) })
}

func (s *shell) showWindow() {
	s.showing.run(s.showOrCreateWindow)
}

func (s *shell) showOrCreateWindow() {
	if w := s.window.Load(); w != nil {
		if w.IsMinimised() {
			w.UnMinimise()
		}
		w.Show().Focus()
		return
	}
	w := s.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:               "Claude Dashboard",
		Width:               960,
		Height:              640,
		DisableResize:       true,
		MaximiseButtonState: application.ButtonHidden,
		BackgroundColour:    application.NewRGB(19, 25, 33),
		URL:                 "/" + os.Getenv(startPageEnv),
		Windows:             application.WindowsWindow{Theme: application.Dark},
	})
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) { s.windowClosing(w) })
	s.window.Store(w)
}

// Letting the window be destroyed frees WebView2. Without close-to-tray, closing it quits.
func (s *shell) windowClosing(w *application.WebviewWindow) {
	s.window.CompareAndSwap(w, nil)
	if !s.app.CloseToTray() {
		s.wails.Quit()
	}
}

func (s *shell) pickPicture() (string, error) {
	path, err := s.wails.Dialog.OpenFile().
		SetTitle("Choose a background picture for the dashboard").
		AddFilter("Pictures (*.jpg, *.jpeg, *.png)", "*.jpg;*.jpeg;*.png").
		PromptForSingleSelection()
	// Wails reports a cancelled dialog as an error with no path; that is not a failure.
	if path == "" {
		return "", nil
	}
	return path, err
}

// showGuard lets only one window open run at a time. A call arriving while one
// is under way (including a re-entrant one on the UI thread while WebView2
// starts) is dropped rather than queued, because waiting there deadlocks.
type showGuard struct {
	mu sync.Mutex
}

func (g *showGuard) run(fn func()) {
	if !g.mu.TryLock() {
		return
	}
	defer g.mu.Unlock()
	fn()
}

// latest applies only the newest value, on its own goroutine: tray calls wait
// for the UI thread, and the code reporting usage must not be held up by that.
type latest struct {
	mu    sync.Mutex
	value string
	wake  chan struct{}
}

func newLatest(apply func(string)) *latest {
	l := &latest{wake: make(chan struct{}, 1)}
	go func() {
		for range l.wake {
			l.mu.Lock()
			value := l.value
			l.mu.Unlock()
			apply(value)
		}
	}()
	return l
}

func (l *latest) set(value string) {
	l.mu.Lock()
	l.value = value
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
