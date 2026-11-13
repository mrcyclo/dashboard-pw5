package device

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	commands []string
	replies  map[string]string
	fail     map[string]error
}

func (f *fakeRunner) Run(cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	for prefix, err := range f.fail {
		if strings.HasPrefix(cmd, prefix) {
			return "", err
		}
	}
	for prefix, reply := range f.replies {
		if strings.HasPrefix(cmd, prefix) {
			return reply, nil
		}
	}
	return "", nil
}

func newFake() *fakeRunner {
	return &fakeRunner{replies: map[string]string{}, fail: map[string]error{}}
}

func TestPauseAndRestore(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "paused")
	r := newFake()
	if err := Pause(r, marker); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cat /dev/fb0 > /var/tmp/claudedash-fb.dump",
		"lipc-set-prop com.lab126.pillow disableEnablePillow disable",
		"killall -STOP awesome",
		"killall -STOP volumd",
		"lipc-set-prop com.lab126.powerd preventScreenSaver 1",
		"iptables -I INPUT -p udp --dport 8378 -j ACCEPT",
	}
	if !reflect.DeepEqual(r.commands, want) {
		t.Fatalf("pause:\n%s", strings.Join(r.commands, "\n"))
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("pausing leaves a marker")
	}

	r.commands = nil
	Restore(r, marker, "/mnt/us/libkh/bin/fbink")
	want = []string{
		"iptables -D INPUT -p udp --dport 8378 -j ACCEPT",
		"lipc-set-prop com.lab126.powerd preventScreenSaver 0",
		"killall -CONT volumd",
		"killall -CONT awesome",
		"cat /var/tmp/claudedash-fb.dump > /dev/fb0; rm -f /var/tmp/claudedash-fb.dump",
		"/mnt/us/libkh/bin/fbink -q -s -f",
		"lipc-set-prop com.lab126.pillow disableEnablePillow enable",
		"lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.home",
	}
	if !reflect.DeepEqual(r.commands, want) {
		t.Fatalf("restore:\n%s", strings.Join(r.commands, "\n"))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("restoring removes the marker")
	}

	r.commands = nil
	Restore(r, marker, "fbink")
	if len(r.commands) != 0 {
		t.Fatal("restoring twice does nothing the second time")
	}
}

func TestDisplayShowsARegion(t *testing.T) {
	dir := t.TempDir()
	r := newFake()
	d := Display{Runner: r, FBInk: "/fb", Dir: dir}
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	if err := d.Show(img, image.Rect(10, 20, 40, 60), false); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || r.commands[0] != "/fb -q -g file="+filepath.ToSlash(filepath.Join(dir, "region.png"))+",x=10,y=20 -W GL16" {
		t.Fatalf("%v", r.commands)
	}
	file, _ := os.Open(filepath.Join(dir, "region.png"))
	written, err := png.Decode(file)
	file.Close()
	if err != nil || written.Bounds().Dx() != 30 || written.Bounds().Dy() != 40 {
		t.Fatalf("%v %v", err, written.Bounds())
	}

	r.commands = nil
	d.Full(img, true)
	if !strings.HasSuffix(r.commands[0], ",x=0,y=0 -W GL16 -f") {
		t.Fatalf("%v", r.commands)
	}

	r.fail["/fb"] = errors.New("exit status 1")
	if err := d.Show(img, image.Rect(0, 0, 10, 10), false); err == nil {
		t.Fatal("a failing FBInk is reported")
	}
}

func TestBattery(t *testing.T) {
	r := newFake()
	r.replies["lipc-get-prop com.lab126.powerd battLevel"] = "76\n"
	if level, ok := Battery(r); !ok || level != 76 {
		t.Fatalf("%d %v", level, ok)
	}
	r.replies["lipc-get-prop com.lab126.powerd battLevel"] = "abc"
	if _, ok := Battery(r); ok {
		t.Fatal("not a number")
	}
}

func TestLightsAndWifi(t *testing.T) {
	r := newFake()
	SetLight(r, 12)
	SetWarmth(r, 30)
	EnableWifi(r)
	want := []string{
		"lipc-set-prop com.lab126.powerd flIntensity 12",
		"lipc-set-prop com.lab126.powerd currentAmberLevel 24",
		"lipc-set-prop com.lab126.cmd wirelessEnable 1",
	}
	if !reflect.DeepEqual(r.commands, want) {
		t.Fatalf("%v", r.commands)
	}
	r.replies["lipc-get-prop com.lab126.cmd wirelessEnable"] = "0\n"
	if WifiOn(r) {
		t.Fatal("off")
	}
	r.replies["lipc-get-prop com.lab126.cmd wirelessEnable"] = "1\n"
	if !WifiOn(r) {
		t.Fatal("on")
	}
}

func TestPowerEvents(t *testing.T) {
	r := newFake()
	r.replies["lipc-wait-event"] = "goingToScreenSaver 2\n"
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan string, 4)
	go PowerEvents(ctx, r, out)
	select {
	case got := <-out:
		if got != "goingToScreenSaver" {
			t.Fatalf("%q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	cancel()
}
