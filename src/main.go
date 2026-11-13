package main

import (
	"context"
	"embed"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"claudedash/internal/applog"
	"claudedash/internal/autostart"
	"claudedash/internal/background"
	"claudedash/internal/config"
	"claudedash/internal/credentials"
	"claudedash/internal/desk"
	"claudedash/internal/discovery"
	"claudedash/internal/httpd"
	"claudedash/internal/media"
	"claudedash/internal/media/winmedia"
	"claudedash/internal/tokensource"
	"claudedash/internal/usage"
	"claudedash/internal/widget"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	maxLogBytes = 1 << 20
	logCapacity = 500
	// Passed by the sign-in autostart entry: start in the tray without opening the window.
	hiddenFlag = "--hidden"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	dir := filepath.Dir(exePath)
	logPath := filepath.Join(dir, "claudedash.log")
	configPath := filepath.Join(dir, "config.json")

	logFile := openLog(logPath)
	if logFile != nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	var lg *applog.Log
	if logFile != nil {
		lg = applog.New(logFile, logCapacity)
	} else {
		lg = applog.New(nil, logCapacity)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		lg.Add(applog.System, "config.json is not valid, using the defaults: %v", err)
	}
	legacyBackground := filepath.Join(dir, "background.txt")
	migrated := config.MigrateBackground(&cfg, legacyBackground)
	// A new id has to be saved, or the Kindle would see a different PC after a restart.
	migrated = config.EnsurePCID(&cfg) || migrated

	tokens := newTokenSource(lg)
	cachePath := filepath.Join(dir, "usage-cache.json")
	service := usage.New(usage.Options{
		TokenFunc:  tokens.token,
		Remembered: loadSnapshot(cachePath),
	})
	service.OnUpdate(func(s usage.Snapshot) {
		if s.Stale {
			lg.Add(applog.Usage, "update failed: %s", s.Error)
			return
		}
		saveSnapshot(cachePath, s)
		lg.Add(applog.Usage, "updated")
	})

	backdrop := background.New(cfg.Background.Path, cfg.Background.FocusX, cfg.Background.FocusY)
	backdrop.SetView(cfg.Background.FocusX, cfg.Background.FocusY, cfg.Background.Zoom)
	mediaService := newMediaService(lg)
	usagePoll := newPoller(service, time.Duration(cfg.CacheSeconds)*time.Second)
	hostname, _ := os.Hostname()
	beacon := discovery.NewBroadcaster(discovery.NewBeacon(cfg.PCID, hostname, cfg.Port), 2*time.Second, discovery.BroadcastTargets)
	beacon.Log = func(format string, args ...any) { lg.Add(applog.Kindle, format, args...) }
	lan := &lanServer{log: lg, beacon: beacon}
	sh := &shell{}
	widgets := widget.Default(mediaService.Do)

	app := desk.New(desk.Deps{
		ConfigPath:       configPath,
		Config:           cfg,
		LogPath:          logPath,
		Usage:            service,
		Media:            mediaService,
		Clients:          lan,
		Background:       backdrop,
		Widgets:          widgets,
		Log:              lg,
		Autostart:        startup{exePath: exePath},
		TokenExpiry:      tokens.expiry,
		Rebind:           lan.bind,
		Listening:        lan.listening,
		SetUsageInterval: func(seconds int) { usagePoll.setInterval(time.Duration(seconds) * time.Second) },
		PickFile:         sh.pickPicture,
		OpenPath:         openPath,
		LanIP: func() string {
			kindleIP, _, _ := lan.LastClient()
			return pickLAN(listAdapters(), kindleIP)
		},
	})

	lan.watch = httpd.WatchClients(
		httpd.WithScreens(httpd.NewHandler(mediaService, backdrop, app), app, backdrop, app),
		time.Now,
		func(format string, args ...any) { lg.Add(applog.Kindle, format, args...) },
	)
	sh.app = app
	sh.wails = application.New(application.Options{
		Name:     "Claude Dashboard",
		Services: []application.Service{application.NewService(app)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		LogLevel: slog.LevelWarn,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "com.mrcyclo.claudedash",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { sh.showWindow() },
		},
		// The window is destroyed when closed to the tray, so the app must outlive it.
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
	})
	// Everything with a side effect waits until here: application.New exits the
	// process when another instance is already running, and that second launch
	// must not write the config, log a port conflict or start polling.
	if migrated {
		if err := config.Save(configPath, cfg); err != nil {
			lg.Add(applog.System, "could not save the config: %v", err)
		} else {
			os.Remove(legacyBackground)
		}
	}
	if err := lan.bind(cfg.Port); err != nil {
		lg.Add(applog.System, "could not open port %d: %v", cfg.Port, err)
	}
	defer lan.close()
	sh.runTray(service)

	go usagePoll.run()
	if !slices.Contains(os.Args[1:], hiddenFlag) {
		sh.showWindow()
	}
	if err := sh.wails.Run(); err != nil {
		lg.Add(applog.System, "the app stopped because of an error: %v", err)
	}
}

// Media control is optional: without it the usage dashboard still works.
func newMediaService(lg *applog.Log) *media.Service {
	host, err := winmedia.Start()
	if err != nil {
		lg.Add(applog.Media, "media controls are unavailable: %v", err)
		return media.New(nil, nil)
	}
	return media.New(host.Player(), host.Mixer())
}

type tokenSource struct {
	source *tokensource.Source
	err    error
	log    *applog.Log
}

func newTokenSource(lg *applog.Log) *tokenSource {
	path, err := credentials.DefaultPath()
	if err != nil {
		return &tokenSource{err: err, log: lg}
	}
	return &tokenSource{source: &tokensource.Source{Path: path}, log: lg}
}

func (t *tokenSource) token() (string, error) {
	if t.err != nil {
		return "", t.err
	}
	token, err := t.source.Token(context.Background())
	if err != nil {
		t.log.Add(applog.Token, "%v", err)
	}
	return token, err
}

func (t *tokenSource) expiry() (time.Time, error) {
	if t.err != nil {
		return time.Time{}, t.err
	}
	return t.source.Status()
}

type startup struct {
	exePath string
}

// Enabled is true only when the entry starts this program, hidden. An entry left
// by an older build or pointing somewhere else reads as off, so turning the
// setting on rewrites it.
func (s startup) Enabled() (bool, error) {
	command, ok, err := autostart.Command()
	if err != nil || !ok {
		return false, err
	}
	return command == autostart.CommandLine(s.exePath, hiddenFlag), nil
}

func (s startup) Enable() error { return autostart.Enable(s.exePath, hiddenFlag) }

func (s startup) Disable() error { return autostart.Disable() }

// The cache holds only usage percentages, never tokens.
func loadSnapshot(path string) usage.Snapshot {
	var snap usage.Snapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return snap
	}
	if json.Unmarshal(data, &snap) != nil {
		return usage.Snapshot{}
	}
	return snap
}

func saveSnapshot(path string, snap usage.Snapshot) {
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0o600)
}

func openLog(path string) *os.File {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return f
}

func openPath(path string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}
