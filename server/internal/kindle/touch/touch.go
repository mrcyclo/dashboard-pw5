// Package touch turns raw Linux input events from the Kindle's touch screen
// into taps. Coordinates on the Paperwhite 5 already match the portrait screen.
package touch

import (
	"encoding/binary"
	"image"
	"io"
	"regexp"
	"strings"
)

// eventSize is struct input_event on 32-bit ARM: an 8-byte timeval, then type, code, value.
const eventSize = 16

const (
	evKey      = 0x01
	evAbs      = 0x03
	absX       = 0x00
	absY       = 0x01
	absMTX     = 0x35
	absMTY     = 0x36
	absMTTrack = 0x39
	btnTouch   = 0x14a
)

var handler = regexp.MustCompile(`event\d+`)

// FindDevice picks the touch screen from the text of /proc/bus/input/devices:
// the first device with absolute axes that is not an accelerometer.
func FindDevice(procDevices string) string {
	for _, block := range strings.Split(procDevices, "\n\n") {
		var name, event string
		var absolute bool
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "N: "):
				name = strings.ToLower(line)
			case strings.HasPrefix(line, "H: "):
				event = handler.FindString(line)
			case strings.HasPrefix(line, "B: ABS="):
				absolute = true
			}
		}
		if absolute && event != "" && !strings.Contains(name, "accel") {
			return "/dev/input/" + event
		}
	}
	return ""
}

// Decoder follows one finger and reports where it lifted.
type Decoder struct {
	x, y int
}

// Feed takes one 16-byte event and reports a tap when the finger lifts. The
// PW5 reports a lift as tracking id -1; other panels release BTN_TOUCH.
func (d *Decoder) Feed(event []byte) (x, y int, tapped bool) {
	if len(event) < eventSize {
		return 0, 0, false
	}
	typ := binary.LittleEndian.Uint16(event[8:])
	code := binary.LittleEndian.Uint16(event[10:])
	value := int(int32(binary.LittleEndian.Uint32(event[12:])))
	switch {
	case typ == evAbs && (code == absMTX || code == absX):
		d.x = value
	case typ == evAbs && (code == absMTY || code == absY):
		d.y = value
	case typ == evAbs && code == absMTTrack && value == -1:
		return d.x, d.y, true
	case typ == evKey && code == btnTouch && value == 0:
		return d.x, d.y, true
	}
	return 0, 0, false
}

// Read decodes r until it fails, sending each tap. It returns the read error.
func Read(r io.Reader, taps chan<- image.Point) error {
	var d Decoder
	event := make([]byte, eventSize)
	for {
		if _, err := io.ReadFull(r, event); err != nil {
			return err
		}
		if x, y, ok := d.Feed(event); ok {
			taps <- image.Pt(x, y)
		}
	}
}
