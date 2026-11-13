package desk

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"claudedash/internal/applog"
	"claudedash/internal/config"
	"claudedash/internal/widget"
)

// Launcher starts programs and draws their icons; launcher.Windows on the PC.
type Launcher interface {
	Icon(path string, size int) ([]byte, error)
	// Target is the program a path starts, "" when it is not one.
	Target(path string) string
	DisplayName(path string) string
	Open(path, target string) error
}

// AppState is one program of the apps widget as the window lists it.
type AppState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Icon is the hash of its picture at /apps/<id>.png, "" when it has none.
	Icon string `json:"icon,omitempty"`
}

// appIconSize is the side of the picture the Kindle gets, a little over the tile's 88 px.
const appIconSize = 128

type appIcon struct {
	data []byte
	hash string
}

var (
	errUnknownApp   = errors.New("That app is no longer in the list")
	errAppListed    = errors.New("This program is already in the list")
	errAppNotOpened = errors.New("Could not open the program")
)

// icon draws a program's icon once and keeps it; a failure is kept too, as an empty icon.
func (a *App) icon(path string) appIcon {
	a.iconsMu.Lock()
	defer a.iconsMu.Unlock()
	if cached, ok := a.icons[path]; ok {
		return cached
	}
	var out appIcon
	if a.deps.Launcher != nil {
		if data, err := a.deps.Launcher.Icon(path, appIconSize); err == nil {
			sum := sha256.Sum256(data)
			out = appIcon{data: data, hash: hex.EncodeToString(sum[:])[:12]}
		}
	}
	a.icons[path] = out
	return out
}

func (a *App) appTiles(apps []config.App) []widget.AppTile {
	out := []widget.AppTile{}
	for _, app := range apps {
		out = append(out, widget.AppTile{ID: app.ID, Name: app.Name, Icon: a.icon(app.Path).hash})
	}
	return out
}

func (a *App) appStates(apps []config.App) []AppState {
	out := []AppState{}
	for _, tile := range a.appTiles(apps) {
		out = append(out, AppState{ID: tile.ID, Name: tile.Name, Icon: tile.Icon})
	}
	return out
}

func (a *App) findApp(id string) (config.App, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, app := range a.config.Apps {
		if app.ID == id {
			return app, true
		}
	}
	return config.App{}, false
}

// AppIcon implements httpd.ScreenSource: the picture of one app tile.
func (a *App) AppIcon(id string) ([]byte, string, bool) {
	app, ok := a.findApp(id)
	if !ok {
		return nil, "", false
	}
	icon := a.icon(app.Path)
	return icon.data, icon.hash, icon.data != nil
}

// AddApp asks for a program or shortcut and adds it to the end of the apps widget.
func (a *App) AddApp() (State, error) {
	if a.deps.PickApp == nil {
		return a.GetState(), nil
	}
	path, err := a.deps.PickApp()
	if err != nil || path == "" {
		return a.GetState(), err
	}
	a.mu.Lock()
	listed := slices.ContainsFunc(a.config.Apps, func(app config.App) bool { return strings.EqualFold(app.Path, path) })
	a.mu.Unlock()
	if listed {
		return a.GetState(), errAppListed
	}
	app := config.App{ID: newAppID(), Name: a.deps.Launcher.DisplayName(path), Path: path}
	a.icon(path)
	if err := a.update(func(c *config.Config) { c.Apps = append(slices.Clone(c.Apps), app) }); err != nil {
		a.deps.Log.Add(applog.System, "could not save the app list: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	a.deps.Log.Add(applog.Kindle, "app added: %s", app.Name)
	return a.GetState(), nil
}

// RemoveApp takes a program off the apps widget.
func (a *App) RemoveApp(id string) (State, error) {
	app, ok := a.findApp(id)
	if !ok {
		return a.GetState(), errUnknownApp
	}
	err := a.update(func(c *config.Config) {
		c.Apps = slices.DeleteFunc(slices.Clone(c.Apps), func(x config.App) bool { return x.ID == id })
	})
	if err != nil {
		a.deps.Log.Add(applog.System, "could not save the app list: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	a.deps.Log.Add(applog.Kindle, "app removed: %s", app.Name)
	return a.GetState(), nil
}

// MoveApp moves a program by delta places in the list, stopping at either end.
func (a *App) MoveApp(id string, delta int) (State, error) {
	if _, ok := a.findApp(id); !ok {
		return a.GetState(), errUnknownApp
	}
	err := a.update(func(c *config.Config) {
		apps := slices.Clone(c.Apps)
		from := slices.IndexFunc(apps, func(x config.App) bool { return x.ID == id })
		to := min(max(from+delta, 0), len(apps)-1)
		app := apps[from]
		apps = slices.Delete(apps, from, from+1)
		c.Apps = slices.Insert(apps, to, app)
	})
	if err != nil {
		a.deps.Log.Add(applog.System, "could not save the app list: %v", err)
		return a.GetState(), errors.New(messageSaveFailed)
	}
	return a.GetState(), nil
}

// openApp starts the program, or brings its window to the front when it runs.
func (a *App) openApp(id string) error {
	app, ok := a.findApp(id)
	if !ok || a.deps.Launcher == nil {
		return widget.ErrUnknownAction
	}
	if err := a.deps.Launcher.Open(app.Path, a.deps.Launcher.Target(app.Path)); err != nil {
		a.deps.Log.Add(applog.Kindle, "could not open %s: %v", app.Name, err)
		return errAppNotOpened
	}
	a.deps.Log.Add(applog.Kindle, "opened %s", app.Name)
	return nil
}

func newAppID() string {
	var raw [4]byte
	rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}
