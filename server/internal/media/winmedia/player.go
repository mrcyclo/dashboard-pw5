package winmedia

import (
	"strconv"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/media/control"
	"github.com/saltosystems/winrt-go/windows/storage/streams"

	"claudedash/internal/media"
)

const (
	iidRandomAccessStream = "905A0FE1-BC53-11DF-8C49-001E4FC686DA"
	iidInputStream        = "905A0FE2-BC53-11DF-8C49-001E4FC686DA"

	// Slots counted after the six IInspectable methods.
	slotRandomAccessStreamSize = 6
	slotInputStreamReadAsync   = 6
	slotAsyncWithProgressGet   = 10

	// A cover larger than this is not worth shipping to an e-ink screen.
	maxThumbnailBytes = 16 << 20

	// Kept well under the Kindle's two second request timeout.
	commandSettleTimeout = 700 * time.Millisecond
	commandSettlePoll    = 30 * time.Millisecond
)

// Chrome and others publish the cover a moment after the title, so the cover is
// read again for a few polls after a track change before the cached one is trusted.
const coverSettlePolls = 3

type Player struct {
	host *Host

	// Only touched on the COM thread.
	coverKey   string
	coverPolls int
	cover      []byte
}

func (h *Host) Player() *Player {
	return &Player{host: h}
}

func (p *Player) Current() (track media.Track, active bool, err error) {
	err = p.host.run(func() error {
		session, err := currentSession()
		if err != nil || session == nil {
			return err
		}
		defer session.Release()
		active = true
		track = p.readTrack(session)
		return nil
	})
	return track, active, err
}

func (p *Player) Send(cmd string) error {
	return p.host.run(func() error {
		session, err := currentSession()
		if err != nil {
			return err
		}
		if session == nil {
			return media.ErrNoSession
		}
		defer session.Release()

		var op *foundation.IAsyncOperation
		switch cmd {
		case media.CmdPlayPause:
			op, err = session.TryTogglePlayPauseAsync()
		case media.CmdNext:
			op, err = session.TrySkipNextAsync()
		case media.CmdPrevious:
			op, err = session.TrySkipPreviousAsync()
		default:
			return media.ErrUnknownCommand
		}
		// The boolean result only says whether the app accepted the request; an
		// app at the first track legitimately refuses "previous", so it is ignored.
		before := sessionMark(session, cmd)
		if _, err := await(op, err); err != nil {
			// The session can close between looking it up and using it.
			return media.ErrNoSession
		}
		waitForMark(session, cmd, before)
		return nil
	})
}

// sessionMark captures the part of the session a command is expected to change.
func sessionMark(session *control.GlobalSystemMediaTransportControlsSession, cmd string) string {
	if cmd == media.CmdPlayPause {
		info, err := session.GetPlaybackInfo()
		if err != nil || info == nil {
			return ""
		}
		defer info.Release()
		status, _ := info.GetPlaybackStatus()
		return strconv.Itoa(int(status))
	}
	raw, err := await(session.TryGetMediaPropertiesAsync())
	if err != nil || raw == nil {
		return ""
	}
	props := (*control.GlobalSystemMediaTransportControlsSessionMediaProperties)(raw)
	defer props.Release()
	title, _ := props.GetTitle()
	artist, _ := props.GetArtist()
	return title + "|" + artist
}

// Apps apply a command a moment after accepting it. Waiting briefly here lets
// the reply to the command already carry the new state.
func waitForMark(session *control.GlobalSystemMediaTransportControlsSession, cmd, before string) {
	deadline := time.Now().Add(commandSettleTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(commandSettlePoll)
		if sessionMark(session, cmd) != before {
			return
		}
	}
}

// The manager is requested on every call: it is cheap, and a cached one goes
// stale when Windows restarts its media service.
func currentSession() (*control.GlobalSystemMediaTransportControlsSession, error) {
	p, err := await(control.GlobalSystemMediaTransportControlsSessionManagerRequestAsync())
	if err != nil {
		return nil, err
	}
	manager := (*control.GlobalSystemMediaTransportControlsSessionManager)(p)
	defer manager.Release()
	return manager.GetCurrentSession()
}

func (p *Player) readTrack(session *control.GlobalSystemMediaTransportControlsSession) media.Track {
	var track media.Track
	track.AppID, _ = session.GetSourceAppUserModelId()

	if info, err := session.GetPlaybackInfo(); err == nil && info != nil {
		status, _ := info.GetPlaybackStatus()
		track.Playing = status == control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPlaying
		info.Release()
	}

	raw, err := await(session.TryGetMediaPropertiesAsync())
	if err != nil || raw == nil {
		return track
	}
	props := (*control.GlobalSystemMediaTransportControlsSessionMediaProperties)(raw)
	defer props.Release()
	track.Title, _ = props.GetTitle()
	track.Artist, _ = props.GetArtist()

	track.Art = p.coverFor(track, props)
	return track
}

// Reading the cover is the expensive part of a poll, and it only changes with
// the track, so it is cached per track.
func (p *Player) coverFor(track media.Track, props *control.GlobalSystemMediaTransportControlsSessionMediaProperties) []byte {
	key := track.AppID + "|" + track.Title + "|" + track.Artist
	if key != p.coverKey {
		p.coverKey, p.coverPolls, p.cover = key, 0, nil
	}
	if p.cover != nil && p.coverPolls >= coverSettlePolls {
		return p.cover
	}
	p.coverPolls++

	thumbnail, err := props.GetThumbnail()
	if err != nil || thumbnail == nil {
		p.cover = nil
		return nil
	}
	defer thumbnail.Release()
	p.cover, _ = readThumbnail(thumbnail)
	return p.cover
}

func readThumbnail(ref *streams.IRandomAccessStreamReference) ([]byte, error) {
	stream, err := await(ref.OpenReadAsync())
	if err != nil || stream == nil {
		return nil, err
	}
	unknown := (*ole.IUnknown)(stream)
	defer unknown.Release()

	randomAccess, err := unknown.QueryInterface(ole.NewGUID(iidRandomAccessStream))
	if err != nil {
		return nil, err
	}
	defer randomAccess.Release()
	var size uint64
	if err := callOut(unsafe.Pointer(randomAccess), slotRandomAccessStreamSize, unsafe.Pointer(&size)); err != nil {
		return nil, err
	}
	if size == 0 || size > maxThumbnailBytes {
		return nil, nil
	}

	input, err := unknown.QueryInterface(ole.NewGUID(iidInputStream))
	if err != nil {
		return nil, err
	}
	defer input.Release()

	buffer, err := streams.BufferCreate(uint32(size))
	if err != nil {
		return nil, err
	}
	defer buffer.Release()
	target, err := buffer.QueryInterface(ole.NewGUID(streams.GUIDIBuffer))
	if err != nil {
		return nil, err
	}
	defer target.Release()

	var readOp unsafe.Pointer
	err = callReadAsync(
		unsafe.Pointer(input),
		slotInputStreamReadAsync,
		unsafe.Pointer(target),
		uint32(size),
		unsafe.Pointer(&readOp),
	)
	if err != nil {
		return nil, err
	}
	defer release(readOp)
	if err := waitDone(readOp); err != nil {
		return nil, err
	}
	var filled unsafe.Pointer
	if err := callOut(readOp, slotAsyncWithProgressGet, unsafe.Pointer(&filled)); err != nil {
		return nil, err
	}
	defer release(filled)

	result := (*streams.IBuffer)(filled)
	length, err := result.GetLength()
	if err != nil {
		return nil, err
	}
	reader, err := streams.DataReaderFromBuffer(result)
	if err != nil {
		return nil, err
	}
	defer reader.Release()
	return reader.ReadBytes(length)
}
