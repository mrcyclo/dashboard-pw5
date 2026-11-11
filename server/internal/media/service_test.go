package media

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
)

type fakePlayer struct {
	track  Track
	active bool
	err    error
	sent   []string
	sendFn func(cmd string) error
}

func (p *fakePlayer) Current() (Track, bool, error) { return p.track, p.active, p.err }

func (p *fakePlayer) Send(cmd string) error {
	p.sent = append(p.sent, cmd)
	if p.sendFn != nil {
		return p.sendFn(cmd)
	}
	if !p.active {
		return ErrNoSession
	}
	return nil
}

type fakeMixer struct {
	level   int
	muted   bool
	err     error
	unmuted int
}

func (m *fakeMixer) Volume() (int, bool, error) { return m.level, m.muted, m.err }

func (m *fakeMixer) SetVolume(level int) error {
	m.level = level
	return nil
}

func (m *fakeMixer) SetMuted(muted bool) error {
	m.muted = muted
	if !muted {
		m.unmuted++
	}
	return nil
}

func cover(shade uint8) []byte {
	img := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func playing() *fakePlayer {
	return &fakePlayer{active: true, track: Track{
		Title: "Jóga", Artist: "Björk", AppID: "Spotify.exe", Playing: true, Art: cover(100),
	}}
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func TestSnapshotWithSession(t *testing.T) {
	s := New(playing(), &fakeMixer{level: 60})
	got := s.Snapshot()
	if got.Art == "" {
		t.Fatal("art hash missing")
	}
	want := State{
		Active: true, Title: "Jóga", Artist: "Björk", App: "Spotify",
		Playing: true, Art: got.Art, Volume: intPtr(60), Muted: boolPtr(false),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSnapshotWithoutSessionHasOnlyVolume(t *testing.T) {
	s := New(&fakePlayer{}, &fakeMixer{level: 30, muted: true})
	out, _ := json.Marshal(s.Snapshot())
	if string(out) != `{"active":false,"volume":30,"muted":true}` {
		t.Fatalf("got %s", out)
	}
}

func TestSnapshotOmitsEmptyFields(t *testing.T) {
	p := &fakePlayer{active: true, track: Track{Title: "Only title", AppID: "308046B0AF4A39CC"}}
	out, _ := json.Marshal(New(p, &fakeMixer{err: errors.New("no device")}).Snapshot())
	if string(out) != `{"active":true,"title":"Only title"}` {
		t.Fatalf("got %s", out)
	}
}

func TestPlayerErrorMeansNoSession(t *testing.T) {
	p := playing()
	p.err = errors.New("winrt failed")
	if got := New(p, &fakeMixer{level: 10}).Snapshot(); got.Active || got.Title != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestNilPlayerAndMixerAreSafe(t *testing.T) {
	s := New(nil, nil)
	out, _ := json.Marshal(s.Snapshot())
	if string(out) != `{"active":false}` {
		t.Fatalf("got %s", out)
	}
	if _, err := s.Do("next"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Do("volume-up"); err == nil {
		t.Fatal("volume without a mixer should fail")
	}
}

func TestBrokenArtIsDropped(t *testing.T) {
	p := playing()
	p.track.Art = []byte("garbage")
	s := New(p, &fakeMixer{})
	if got := s.Snapshot(); got.Art != "" || !got.Active {
		t.Fatalf("got %+v", got)
	}
	if _, _, ok := s.Art(); ok {
		t.Fatal("no art expected")
	}
}

func TestArtFollowsTheTrack(t *testing.T) {
	p := playing()
	s := New(p, &fakeMixer{})
	first := s.Snapshot().Art
	data, hash, ok := s.Art()
	if !ok || hash != first || !strings.HasPrefix(string(data), "\x89PNG") {
		t.Fatalf("art ok=%v hash=%q", ok, hash)
	}
	if again := s.Snapshot().Art; again != first {
		t.Fatalf("same cover changed hash: %q %q", first, again)
	}
	p.track.Art = cover(200)
	second := s.Snapshot().Art
	if second == "" || second == first {
		t.Fatalf("new cover should change hash: %q %q", first, second)
	}
	p.track.Art = nil
	if got := s.Snapshot(); got.Art != "" {
		t.Fatalf("got %+v", got)
	}
	if _, _, ok := s.Art(); ok {
		t.Fatal("art should be gone with the cover")
	}
}

func TestSameCoverIsProcessedOnce(t *testing.T) {
	calls := 0
	original := processArt
	processArt = func(raw []byte) ([]byte, string, error) {
		calls++
		return original(raw)
	}
	defer func() { processArt = original }()
	s := New(playing(), &fakeMixer{})
	for i := 0; i < 5; i++ {
		s.Snapshot()
	}
	if calls != 1 {
		t.Fatalf("processed %d times", calls)
	}
}

func TestPlaybackCommandsReachThePlayer(t *testing.T) {
	p := playing()
	s := New(p, &fakeMixer{})
	for _, cmd := range []string{"play-pause", "next", "previous"} {
		if state, err := s.Do(cmd); err != nil || !state.Active {
			t.Fatalf("%s: %+v %v", cmd, state, err)
		}
	}
	if !reflect.DeepEqual(p.sent, []string{"play-pause", "next", "previous"}) {
		t.Fatalf("sent %v", p.sent)
	}
}

func TestPlaybackCommandWithoutSession(t *testing.T) {
	if _, err := New(&fakePlayer{}, &fakeMixer{}).Do("next"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
}

func TestSessionVanishingMidCommand(t *testing.T) {
	p := playing()
	p.sendFn = func(string) error {
		p.active = false
		return ErrNoSession
	}
	if _, err := New(p, &fakeMixer{}).Do("play-pause"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
}

func TestVolumeSteps(t *testing.T) {
	cases := []struct {
		cmd        string
		from, want int
	}{
		{"volume-up", 60, 65}, {"volume-up", 98, 100}, {"volume-up", 100, 100},
		{"volume-down", 60, 55}, {"volume-down", 3, 0}, {"volume-down", 0, 0},
	}
	for _, c := range cases {
		m := &fakeMixer{level: c.from}
		state, err := New(&fakePlayer{}, m).Do(c.cmd)
		if err != nil || m.level != c.want || state.Volume == nil || *state.Volume != c.want {
			t.Errorf("%s from %d: mixer %d state %+v err %v", c.cmd, c.from, m.level, state, err)
		}
	}
}

func TestRepeatedVolumeUpNeverOvershoots(t *testing.T) {
	m := &fakeMixer{level: 80}
	s := New(&fakePlayer{}, m)
	for i := 0; i < 10; i++ {
		s.Do("volume-up")
	}
	if m.level != 100 {
		t.Fatalf("level %d", m.level)
	}
}

func TestVolumeUpUnmutes(t *testing.T) {
	m := &fakeMixer{level: 20, muted: true}
	state, _ := New(&fakePlayer{}, m).Do("volume-up")
	if m.muted || m.unmuted != 1 || *state.Muted {
		t.Fatalf("mixer %+v state %+v", m, state)
	}
	m = &fakeMixer{level: 20, muted: true}
	New(&fakePlayer{}, m).Do("volume-down")
	if !m.muted {
		t.Fatal("volume-down must not unmute")
	}
}

func TestUnknownCommandTouchesNothing(t *testing.T) {
	p, m := playing(), &fakeMixer{level: 50}
	if _, err := New(p, m).Do("shutdown"); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("got %v", err)
	}
	if len(p.sent) != 0 || m.level != 50 {
		t.Fatal("unknown command had side effects")
	}
}

func TestAppName(t *testing.T) {
	cases := map[string]string{
		"Spotify.exe": "Spotify",
		"chrome.exe":  "Chrome",
		"Chrome":      "Chrome",
		"msedge.exe":  "Edge",
		"MSEdge":      "Edge",
		"firefox.exe": "Firefox",
		"SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify": "Spotify",
		"308046B0AF4A39CB": "Firefox",
		"308046B0AF4A39CC": "",
		"foobar2000.exe":   "Foobar2000",
		"":                 "",
	}
	for in, want := range cases {
		if got := AppName(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestUnusualTextSurvivesJSON(t *testing.T) {
	p := playing()
	p.track.Title = "塵外客 \"quoted\" \U0001F3B5 " + strings.Repeat("größe ", 200)
	out, err := json.Marshal(New(p, &fakeMixer{}).Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(out, &back); err != nil || back.Title != p.track.Title {
		t.Fatalf("round trip failed: %v", err)
	}
}
