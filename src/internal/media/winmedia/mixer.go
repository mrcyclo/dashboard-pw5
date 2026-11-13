package winmedia

import (
	"math"
	"unsafe"

	"github.com/moutend/go-wca/pkg/wca"
)

type Mixer struct {
	host *Host
}

func (h *Host) Mixer() *Mixer {
	return &Mixer{host: h}
}

func (m *Mixer) Volume() (level int, muted bool, err error) {
	err = m.host.run(func() error {
		return withEndpoint(func(endpoint *wca.IAudioEndpointVolume) error {
			var scalar float32
			if err := endpoint.GetMasterVolumeLevelScalar(&scalar); err != nil {
				return err
			}
			level = int(math.Round(float64(scalar) * 100))
			// COM writes a 4-byte BOOL; handing it a 1-byte Go bool would overwrite its neighbours.
			var raw int32
			if err := endpoint.GetMute((*bool)(unsafe.Pointer(&raw))); err != nil {
				return err
			}
			muted = raw != 0
			return nil
		})
	})
	return level, muted, err
}

func (m *Mixer) SetVolume(level int) error {
	return m.host.run(func() error {
		return withEndpoint(func(endpoint *wca.IAudioEndpointVolume) error {
			return endpoint.SetMasterVolumeLevelScalar(float32(level)/100, nil)
		})
	})
}

func (m *Mixer) SetMuted(muted bool) error {
	return m.host.run(func() error {
		return withEndpoint(func(endpoint *wca.IAudioEndpointVolume) error {
			return endpoint.SetMute(muted, nil)
		})
	})
}

// The default device is looked up on every call so that plugging in headphones
// or switching outputs takes effect without restarting the app.
func withEndpoint(fn func(*wca.IAudioEndpointVolume) error) error {
	var enumerator *wca.IMMDeviceEnumerator
	err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator,
		&enumerator,
	)
	if err != nil {
		return err
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &device); err != nil {
		return err
	}
	defer device.Release()

	var endpoint *wca.IAudioEndpointVolume
	if err := device.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &endpoint); err != nil {
		return err
	}
	defer endpoint.Release()
	return fn(endpoint)
}
