package touch

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"testing"
)

const pw5Devices = `I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="gpio-keys"
H: Handlers=kbd event0
B: EV=3

I: Bus=0018 Vendor=0000 Product=0000 Version=0000
N: Name="pt_mt"
H: Handlers=event1 perfmgr
B: EV=b
B: ABS=e618000 0
`

func TestFindDevice(t *testing.T) {
	if got := FindDevice(pw5Devices); got != "/dev/input/event1" {
		t.Fatalf("got %q", got)
	}
	withAccel := "N: Name=\"accel\"\nH: Handlers=event3\nB: ABS=7\n\n" + pw5Devices
	if got := FindDevice(withAccel); got != "/dev/input/event1" {
		t.Fatalf("an accelerometer is not a touch screen: %q", got)
	}
	if FindDevice("N: Name=\"gpio-keys\"\nH: Handlers=kbd event0\n") != "" {
		t.Fatal("no touch screen")
	}
}

func event(typ, code uint16, value int32) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[8:], typ)
	binary.LittleEndian.PutUint16(b[10:], code)
	binary.LittleEndian.PutUint32(b[12:], uint32(value))
	return b
}

// The sequence the spike logged on the PW5: tracking id, X, Y, touch size, pressure, SYN, ..., tracking id -1.
func pw5Tap(x, y int32) [][]byte {
	return [][]byte{
		event(3, 0x39, 0), event(3, 0x35, x), event(3, 0x36, y), event(3, 0x3a, 76), event(3, 0x30, 69), event(0, 0, 0),
		event(3, 0x35, x-1), event(0, 0, 0),
		event(3, 0x39, -1), event(0, 0, 0),
	}
}

func TestTrackingIDEndsATap(t *testing.T) {
	var d Decoder
	var taps []image.Point
	for _, e := range pw5Tap(246, 1270) {
		if x, y, ok := d.Feed(e); ok {
			taps = append(taps, image.Pt(x, y))
		}
	}
	if len(taps) != 1 || taps[0] != image.Pt(245, 1270) {
		t.Fatalf("%v", taps)
	}
}

func TestButtonReleaseEndsATap(t *testing.T) {
	var d Decoder
	d.Feed(event(3, 0x00, 300))
	d.Feed(event(3, 0x01, 400))
	if _, _, ok := d.Feed(event(1, 0x14a, 1)); ok {
		t.Fatal("pressing is not a tap")
	}
	if x, y, ok := d.Feed(event(1, 0x14a, 0)); !ok || x != 300 || y != 400 {
		t.Fatalf("%d %d %v", x, y, ok)
	}
}

func TestMovementAloneIsNotATap(t *testing.T) {
	var d Decoder
	for _, e := range pw5Tap(10, 10)[:8] {
		if _, _, ok := d.Feed(e); ok {
			t.Fatal("no lift yet")
		}
	}
}

// chunks returns the stream in pieces, as a device read may.
type chunks struct{ parts [][]byte }

func (c *chunks) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.parts[0])
	c.parts[0] = c.parts[0][n:]
	if len(c.parts[0]) == 0 {
		c.parts = c.parts[1:]
	}
	return n, nil
}

func TestReadJoinsShortReads(t *testing.T) {
	stream := bytes.Join(pw5Tap(640, 947), nil)
	r := &chunks{parts: [][]byte{stream[:10], stream[10:16], stream[16:]}}
	out := make(chan image.Point, 4)
	err := Read(r, out)
	if err != io.EOF {
		t.Fatalf("err %v", err)
	}
	if len(out) != 1 || <-out != image.Pt(639, 947) {
		t.Fatal("one tap")
	}
}
