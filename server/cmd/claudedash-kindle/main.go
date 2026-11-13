// Command claudedash-kindle is the Claude Dashboard app for a jailbroken Kindle
// Paperwhite 5. It finds the Claude Dashboard PC app on the LAN and shows its
// widgets full screen until the exit button is tapped.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"claudedash/internal/discovery"
	"claudedash/internal/kindle/app"
	"claudedash/internal/kindle/device"
	"claudedash/internal/kindle/render"
	"claudedash/internal/kindle/touch"
)

const (
	appDir  = "/mnt/us/claudedash"
	fbink   = "/mnt/us/libkh/bin/fbink"
	marker  = "/var/tmp/claudedash.paused"
	workDir = "/tmp/claudedash"

	logLimit = 500 << 10
	logKeep  = 250 << 10
)

func main() {
	logf := openLog(filepath.Join(appDir, "claudedash.log"))
	logf("starting")
	shell := device.Shell{Log: logf}

	if _, err := os.Stat(fbink); err != nil {
		logf("FBInk missing: %v", err)
		shell.Run(`eips 1 1 "FBInk not found. Reinstall the jailbreak hotfix."`)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer stop()

	device.Pause(shell, marker)
	err := run(ctx, shell, logf)
	device.Restore(shell, marker, fbink)
	if err != nil {
		logf("stopped: %v", err)
		os.Exit(1)
	}
	logf("exit")
}

func run(ctx context.Context, shell device.Shell, logf func(string, ...any)) error {
	os.MkdirAll(workDir, 0o755)
	fonts, err := render.LoadFonts(filepath.Join(appDir, "NotoSans-Regular.ttf"), filepath.Join(appDir, "NotoSans-Bold.ttf"))
	if err != nil {
		return fmt.Errorf("fonts: %w", err)
	}

	beacons := make(chan discovery.Seen, 8)
	if conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", discovery.Port)); err == nil {
		defer conn.Close()
		go discovery.Listen(conn, beacons)
	} else {
		logf("discovery: %v", err)
	}

	taps := make(chan image.Point, 8)
	go readTouch(taps, logf)

	power := make(chan string, 4)
	go device.PowerEvents(ctx, shell, power)

	return app.Run(ctx, app.Config{
		Dir:     appDir,
		Fonts:   fonts,
		Display: device.Display{Runner: shell, FBInk: fbink, Dir: workDir},
		Runner:  shell,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		Beacons: beacons,
		Taps:    taps,
		Power:   power,
		Manual:  manualAddress(filepath.Join(appDir, "config.json")),
		Log:     logf,
	})
}

// readTouch sends taps until the touch screen cannot be read, then closes taps.
func readTouch(taps chan<- image.Point, logf func(string, ...any)) {
	defer close(taps)
	devices, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		logf("touch: %v", err)
		return
	}
	path := touch.FindDevice(string(devices))
	if path == "" {
		logf("touch: no touch screen listed")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		logf("touch: %v", err)
		return
	}
	defer f.Close()
	// Grabbing keeps the paused Kindle UI from also seeing the taps.
	if err := grab(f); err != nil {
		logf("touch grab: %v", err)
	}
	logf("touch: reading %s", path)
	logf("touch: %v", touch.Read(f, taps))
}

// manualAddress reads the optional config.json {"host": "...", "port": 8377}.
func manualAddress(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var c struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if json.Unmarshal(data, &c) != nil || c.Host == "" {
		return ""
	}
	if c.Port == 0 {
		c.Port = 8377
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// openLog appends to the log, first cutting it to its tail when it grew too big.
func openLog(path string) func(string, ...any) {
	if info, err := os.Stat(path); err == nil && info.Size() > logLimit {
		if f, err := os.Open(path); err == nil {
			f.Seek(-logKeep, io.SeekEnd)
			tail, _ := io.ReadAll(f)
			f.Close()
			os.WriteFile(path, tail, 0o644)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func(string, ...any) {}
	}
	return func(format string, args ...any) {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
		f.Sync()
	}
}
