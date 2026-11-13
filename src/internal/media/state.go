package media

import "errors"

var (
	ErrNoSession      = errors.New("media: no active media session")
	ErrUnknownCommand = errors.New("media: unknown command")
	ErrNoMixer        = errors.New("media: volume control unavailable")
)

const (
	CmdPlayPause  = "play-pause"
	CmdNext       = "next"
	CmdPrevious   = "previous"
	CmdVolumeUp   = "volume-up"
	CmdVolumeDown = "volume-down"

	volumeStep = 5
)

type Track struct {
	Title   string
	Artist  string
	AppID   string
	Playing bool
	// Raw cover art as the playing app supplied it; nil when there is none.
	Art []byte
}

type Player interface {
	// Current reports false when Windows has no media session.
	Current() (Track, bool, error)
	// Send returns ErrNoSession when there is nothing to control.
	Send(cmd string) error
}

type Mixer interface {
	Volume() (level int, muted bool, err error)
	SetVolume(level int) error
	SetMuted(muted bool) error
}

type State struct {
	Active  bool   `json:"active"`
	Title   string `json:"title,omitempty"`
	Artist  string `json:"artist,omitempty"`
	App     string `json:"app,omitempty"`
	Playing bool   `json:"playing,omitempty"`
	Art     string `json:"art,omitempty"`
	Volume  *int   `json:"volume,omitempty"`
	Muted   *bool  `json:"muted,omitempty"`
}
