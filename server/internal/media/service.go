package media

import (
	"crypto/sha256"
	"sync"

	"claudedash/internal/media/artwork"
)

var processArt = artwork.Process

type Service struct {
	player Player
	mixer  Mixer

	mu      sync.Mutex
	rawSum  [sha256.Size]byte
	artPNG  []byte
	artHash string
}

// New accepts a nil player or mixer so the rest of the app keeps working when
// Windows media or audio APIs are unavailable.
func New(p Player, m Mixer) *Service {
	return &Service{player: p, mixer: m}
}

func (s *Service) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Service) Art() (pngData []byte, hash string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.artHash == "" {
		return nil, "", false
	}
	return s.artPNG, s.artHash, true
}

func (s *Service) Do(cmd string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	switch cmd {
	case CmdPlayPause, CmdNext, CmdPrevious:
		err = s.sendToPlayer(cmd)
	case CmdVolumeUp:
		err = s.stepVolume(volumeStep)
	case CmdVolumeDown:
		err = s.stepVolume(-volumeStep)
	default:
		err = ErrUnknownCommand
	}
	if err != nil {
		return State{}, err
	}
	return s.snapshot(), nil
}

func (s *Service) sendToPlayer(cmd string) error {
	if s.player == nil {
		return ErrNoSession
	}
	return s.player.Send(cmd)
}

func (s *Service) stepVolume(delta int) error {
	if s.mixer == nil {
		return ErrNoMixer
	}
	level, muted, err := s.mixer.Volume()
	if err != nil {
		return err
	}
	if delta > 0 && muted {
		if err := s.mixer.SetMuted(false); err != nil {
			return err
		}
	}
	return s.mixer.SetVolume(min(max(level+delta, 0), 100))
}

func (s *Service) snapshot() State {
	var state State
	if s.mixer != nil {
		if level, muted, err := s.mixer.Volume(); err == nil {
			state.Volume, state.Muted = &level, &muted
		}
	}
	if s.player == nil {
		s.setArt(nil)
		return state
	}
	track, active, err := s.player.Current()
	if err != nil || !active {
		s.setArt(nil)
		return state
	}
	s.setArt(track.Art)
	state.Active = true
	state.Title = track.Title
	state.Artist = track.Artist
	state.App = AppName(track.AppID)
	state.Playing = track.Playing
	state.Art = s.artHash
	return state
}

// setArt reprocesses the cover only when its bytes change, because the same
// picture arrives again on every poll while a track plays.
func (s *Service) setArt(raw []byte) {
	if len(raw) == 0 {
		s.rawSum, s.artPNG, s.artHash = [sha256.Size]byte{}, nil, ""
		return
	}
	sum := sha256.Sum256(raw)
	if sum == s.rawSum {
		return
	}
	s.rawSum = sum
	pngData, hash, err := processArt(raw)
	if err != nil {
		s.artPNG, s.artHash = nil, ""
		return
	}
	s.artPNG, s.artHash = pngData, hash
}
