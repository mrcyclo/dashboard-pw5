package widget

import (
	"strconv"

	"claudedash/internal/media"
)

const volumeStep = 5

type mediaKind struct {
	do func(cmd string) (media.State, error)
}

// NewMedia is the music widget; do runs a media command (see media.Service.Do).
func NewMedia(do func(cmd string) (media.State, error)) Kind {
	return mediaKind{do: do}
}

func (mediaKind) Name() string    { return "media" }
func (mediaKind) Label() string   { return "Music" }
func (mediaKind) Icon() string    { return "music" }
func (mediaKind) Sizes() []string { return []string{"6x2", "6x1"} }

func (k mediaKind) Actions() map[string]ActionFunc {
	actions := map[string]ActionFunc{}
	for _, cmd := range []string{media.CmdPlayPause, media.CmdNext, media.CmdPrevious, media.CmdVolumeUp, media.CmdVolumeDown} {
		actions["media."+cmd] = func() error {
			if k.do == nil {
				return media.ErrNoSession
			}
			_, err := k.do(cmd)
			return err
		}
	}
	return actions
}

func heading(s media.State) string {
	if !s.Active {
		return "Music"
	}
	text := "Paused"
	if s.Playing {
		text = "Playing"
	}
	if s.App != "" {
		text += " · " + s.App
	}
	return text
}

func cover(s media.State, side int) Node {
	if s.Active && s.Art != "" {
		return Image("/media/art.png?v="+s.Art, side, side)
	}
	return Icon("note", side)
}

func playback(s media.State, side int) []Node {
	icon, other := "play", "pause"
	if s.Active && s.Playing {
		icon, other = "pause", "play"
	}
	buttons := []Node{
		Button("previous", side, "media."+media.CmdPrevious),
		Button(icon, side, "media."+media.CmdPlayPause).ID("pp").Patch(Toggle("pp", "icon", icon, other)),
		Button("next", side, "media."+media.CmdNext),
	}
	if !s.Active {
		for _, b := range buttons {
			b.Disabled()
		}
	}
	return buttons
}

func track(s media.State, titleSize, artistSize int) []Node {
	if !s.Active {
		return []Node{Text("Nothing playing", artistSize)}
	}
	var lines []Node
	if s.Title != "" {
		lines = append(lines, Text(s.Title, titleSize).Bold().ID("title"))
	}
	if s.Artist != "" {
		lines = append(lines, Text(s.Artist, artistSize))
	}
	return lines
}

// volumeRow is the quieter button, the level bar, the louder button and the number.
func volumeRow(s media.State) (Node, bool) {
	if s.Volume == nil {
		return nil, false
	}
	level, text := *s.Volume, strconv.Itoa(*s.Volume)
	if s.Muted != nil && *s.Muted {
		level, text = 0, "Mute"
	}
	down := Button("volume-low", 100, "media."+media.CmdVolumeDown).Plain().Patch(
		Add("vol-bar", "percent", -volumeStep, 0, 100),
		Add("vol-num", "text", -volumeStep, 0, 100),
	)
	up := Button("volume-high", 100, "media."+media.CmdVolumeUp).Plain().Patch(
		Add("vol-bar", "percent", volumeStep, 0, 100),
		Add("vol-num", "text", volumeStep, 0, 100),
	)
	return Row(16, down, Bar(level, 16).ID("vol-bar").Grow(1), up, Text(text, 43).Bold().ID("vol-num")), true
}

func (mediaKind) Build(size string, d Data) Node {
	s := d.Media
	if size == "6x1" {
		lines := append([]Node{Text(heading(s), 25).Dim()}, track(s, 33, 27)...)
		children := append([]Node{cover(s, 100), Col(2, lines...).ID("track").Grow(1)}, playback(s, 100)...)
		return Row(16, children...)
	}

	row := append([]Node{cover(s, 120), Col(4, track(s, 37, 31)...).ID("track").Grow(1)}, playback(s, 120)...)
	children := []Node{Text(heading(s), 29).Bold(), Rule(), Space(4), Row(16, row...)}
	if volume, ok := volumeRow(s); ok {
		children = append(children, Space(4), volume)
	}
	return Col(8, children...)
}
