// Package device is everything that talks to the Kindle itself: shell commands,
// FBInk, taking over the screen from the Kindle UI, power, lights and Wi-Fi.
// Property names follow what KOReader uses on the Paperwhite 5.
package device

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Runner runs one shell command and returns its output.
type Runner interface {
	Run(cmd string) (string, error)
}

// ContextRunner runs a command until it ends or ctx is cancelled, with no other limit.
type ContextRunner interface {
	RunContext(ctx context.Context, cmd string) (string, error)
}

// Shell runs commands with /bin/sh. A command that fails or is slow is logged.
type Shell struct {
	Log func(format string, args ...any)
	// Timeout ends a command that hangs, so a stuck fbink or lipc call can never
	// freeze the dashboard. Zero means 10 seconds.
	Timeout time.Duration
	// Interpreter and Env replace "/bin/sh -c" and add environment; tests use them.
	Interpreter []string
	Env         []string
}

const (
	defaultTimeout = 10 * time.Second
	slowCommand    = time.Second
)

func (s Shell) Run(cmd string) (string, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.RunContext(ctx, cmd)
}

func (s Shell) RunContext(ctx context.Context, cmd string) (string, error) {
	interpreter := s.Interpreter
	if len(interpreter) == 0 {
		interpreter = []string{"/bin/sh", "-c"}
	}
	c := exec.CommandContext(ctx, interpreter[0], append(interpreter[1:], cmd)...)
	if len(s.Env) > 0 {
		c.Env = append(os.Environ(), s.Env...)
	}
	// Without this, a killed shell's children could keep the output pipe open.
	c.WaitDelay = time.Second
	start := time.Now()
	out, err := c.CombinedOutput()
	if took := time.Since(start); s.Log != nil && (err != nil || took > slowCommand) {
		s.Log("sh %q -> %v in %v %s", cmd, err, took.Round(time.Millisecond), strings.TrimSpace(string(out)))
	}
	return string(out), err
}

const (
	dump = "/var/tmp/claudedash-fb.dump"
	// MaxLight is the top level of the Paperwhite 5's front light and warmth.
	MaxLight = 24
)

var takeover = []string{
	"cat /dev/fb0 > " + dump,
	"lipc-set-prop com.lab126.pillow disableEnablePillow disable",
	"killall -STOP awesome",
	"killall -STOP volumd",
	"lipc-set-prop com.lab126.powerd preventScreenSaver 1",
	"iptables -I INPUT -p udp --dport 8378 -j ACCEPT",
}

// Pause takes the screen from the Kindle UI and leaves a marker while it is held.
func Pause(r Runner, marker string) error {
	for _, cmd := range takeover {
		r.Run(cmd)
	}
	return os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// Restore gives the screen back, once: without the marker it does nothing.
func Restore(r Runner, marker, fbink string) {
	if _, err := os.Stat(marker); err != nil {
		return
	}
	for _, cmd := range []string{
		"iptables -D INPUT -p udp --dport 8378 -j ACCEPT",
		"lipc-set-prop com.lab126.powerd preventScreenSaver 0",
		"killall -CONT volumd",
		"killall -CONT awesome",
		"cat " + dump + " > /dev/fb0; rm -f " + dump,
		// Writing the framebuffer does not refresh the e-ink panel.
		fbink + " -q -s -f",
		"lipc-set-prop com.lab126.pillow disableEnablePillow enable",
		"lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.home",
	} {
		r.Run(cmd)
	}
	os.Remove(marker)
}

// Display shows parts of a frame on the e-ink screen through the FBInk command.
type Display struct {
	Runner
	FBInk string
	// Dir holds the image file handed to FBInk; /tmp on the device.
	Dir string
}

// Show writes the region r of img and asks FBInk to draw it there. A flash
// clears e-ink ghosting at the cost of a black blink.
func (d Display) Show(img *image.Gray, r image.Rectangle, flash bool) error {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return nil
	}
	path := filepath.Join(d.Dir, "region.png")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	err = encoder.Encode(file, img.SubImage(r))
	file.Close()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s -q -g file=%s,x=%d,y=%d -W GL16", d.FBInk, filepath.ToSlash(path), r.Min.X, r.Min.Y)
	if flash {
		cmd += " -f"
	}
	_, err = d.Run(cmd)
	return err
}

func (d Display) Full(img *image.Gray, flash bool) error {
	return d.Show(img, img.Bounds(), flash)
}

// Battery is the charge in percent.
func Battery(r Runner) (int, bool) {
	out, err := r.Run("lipc-get-prop com.lab126.powerd battLevel")
	if err != nil {
		return 0, false
	}
	level, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, false
	}
	return level, true
}

func clampLevel(level int) int {
	return min(max(level, 0), MaxLight)
}

func SetLight(r Runner, level int) error {
	_, err := r.Run(fmt.Sprintf("lipc-set-prop com.lab126.powerd flIntensity %d", clampLevel(level)))
	return err
}

func SetWarmth(r Runner, level int) error {
	_, err := r.Run(fmt.Sprintf("lipc-set-prop com.lab126.powerd currentAmberLevel %d", clampLevel(level)))
	return err
}

func WifiOn(r Runner) bool {
	out, err := r.Run("lipc-get-prop com.lab126.cmd wirelessEnable")
	return err == nil && strings.TrimSpace(out) == "1"
}

func EnableWifi(r Runner) error {
	_, err := r.Run("lipc-set-prop com.lab126.cmd wirelessEnable 1")
	return err
}

// PowerEvents reports "goingToScreenSaver" and "outOfScreenSaver" until ctx ends.
func PowerEvents(ctx context.Context, r Runner, out chan<- string) {
	const wait = "lipc-wait-event -s 0 com.lab126.powerd goingToScreenSaver,outOfScreenSaver"
	for ctx.Err() == nil {
		var reply string
		var err error
		// The wait has no time limit of its own, and must end with the app.
		if cr, ok := r.(ContextRunner); ok {
			reply, err = cr.RunContext(ctx, wait)
		} else {
			reply, err = r.Run(wait)
		}
		fields := strings.Fields(reply)
		if err != nil || len(fields) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		select {
		case out <- fields[0]:
		case <-ctx.Done():
			return
		}
	}
}
