package media

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/syspoe/cusus/config"
)

const (
	audioSampleRate = 48000
	audioChannels   = 2
	audioRingBytes  = audioSampleRate * audioChannels * 2 * 2 // two seconds of S16 stereo
	audioPrebuffer  = 4096
)

// AudioDevice is a selectable hardware playback device. An empty ID means the
// operating system's current default device.
type AudioDevice struct {
	ID        string
	Name      string
	IsDefault bool
}

type AudioSystem struct {
	settings *config.Store
	context  *malgo.AllocatedContext
}

func NewAudioSystem(settings *config.Store) (*AudioSystem, error) {
	context, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio system: %w", err)
	}
	return &AudioSystem{settings: settings, context: context}, nil
}

func (a *AudioSystem) Devices() ([]AudioDevice, error) {
	if a == nil || a.context == nil {
		return nil, fmt.Errorf("audio system is unavailable")
	}
	devices, err := a.context.Devices(malgo.Playback)
	if err != nil {
		return nil, fmt.Errorf("list audio devices: %w", err)
	}
	result := make([]AudioDevice, 0, len(devices))
	for i := range devices {
		name := strings.TrimSpace(devices[i].Name())
		if name == "" {
			name = "Unnamed audio device"
		}
		result = append(result, AudioDevice{ID: devices[i].ID.String(), Name: name, IsDefault: devices[i].IsDefault != 0})
	}
	return result, nil
}

func (a *AudioSystem) NewPlayer(reader io.Reader, preview bool) (*devicePlayer, error) {
	player, err := a.NewPreparedPlayer(reader, preview)
	if err != nil {
		return nil, err
	}
	if err := player.Start(); err != nil {
		_ = player.Close()
		return nil, err
	}
	return player, nil
}

// NewPreparedPlayer opens a device without starting its callback so audio and
// video can be buffered before the shared clock begins.
func (a *AudioSystem) NewPreparedPlayer(reader io.Reader, preview bool) (*devicePlayer, error) {
	if a == nil || a.context == nil {
		return nil, fmt.Errorf("audio output is unavailable")
	}
	settings := a.settings.Snapshot()
	deviceID := settings.PlaybackAudioDevice
	if preview {
		deviceID = settings.PreviewAudioDevice
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = audioChannels
	deviceConfig.SampleRate = audioSampleRate
	if deviceID != "" {
		devices, err := a.context.Devices(malgo.Playback)
		if err != nil {
			return nil, fmt.Errorf("list audio devices: %w", err)
		}
		found := false
		for i := range devices {
			if devices[i].ID.String() == deviceID {
				deviceConfig.Playback.DeviceID = devices[i].ID.Pointer()
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("selected audio device is unavailable")
		}
	}

	player := &devicePlayer{
		reader: reader, ring: newPCMRing(audioRingBytes), done: make(chan struct{}),
		ready: make(chan struct{}), stopped: make(chan struct{}),
	}
	player.volume.Store(1)
	callbacks := malgo.DeviceCallbacks{Data: player.readSamples, Stop: player.deviceStopped}
	device, err := malgo.InitDevice(a.context.Context, deviceConfig, callbacks)
	if err != nil {
		return nil, fmt.Errorf("open audio device: %w", err)
	}
	player.device = device
	go player.fillRing()
	return player, nil
}

type devicePlayer struct {
	reader      io.Reader
	device      *malgo.Device
	volume      atomic.Uint64
	ring        *pcmRing
	done        chan struct{}
	ready       chan struct{}
	readyOnce   sync.Once
	stopped     chan struct{}
	stoppedOnce sync.Once
	intentional atomic.Bool
	eof         atomic.Bool
	underruns   atomic.Uint64
	mu          sync.Mutex
	closed      bool
	started     bool
}

func (p *devicePlayer) Start() error {
	select {
	case <-p.ready:
	case <-time.After(time.Second):
		return fmt.Errorf("audio prebuffer timed out")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("audio player is closed")
	}
	if p.started {
		return nil
	}
	if err := p.device.Start(); err != nil {
		return fmt.Errorf("start audio device: %w", err)
	}
	p.started = true
	return nil
}

func (p *devicePlayer) readSamples(output, _ []byte, _ uint32) {
	n := p.ring.read(output)
	clear(output[n:])
	if n < len(output) && !p.eof.Load() {
		p.underruns.Add(1)
	}
	volume := float64FromBits(p.volume.Load())
	if volume >= 0.9999 {
		return
	}
	for i := 0; i+1 < len(output); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(output[i:]))
		scaled := int16(float64(sample) * volume)
		binary.LittleEndian.PutUint16(output[i:], uint16(scaled))
	}
}

func (p *devicePlayer) fillRing() {
	buffer := make([]byte, 32*1024)
	for {
		n, err := p.reader.Read(buffer)
		written := 0
		for written < n {
			count := p.ring.write(buffer[written:n])
			written += count
			if p.ring.available() >= audioPrebuffer {
				p.readyOnce.Do(func() { close(p.ready) })
			}
			if written < n {
				select {
				case <-p.done:
					return
				case <-time.After(2 * time.Millisecond):
				}
			}
		}
		if err != nil {
			p.eof.Store(true)
			p.readyOnce.Do(func() { close(p.ready) })
			return
		}
		select {
		case <-p.done:
			return
		default:
		}
	}
}

func (p *devicePlayer) deviceStopped()           { p.stoppedOnce.Do(func() { close(p.stopped) }) }
func (p *devicePlayer) Stopped() <-chan struct{} { return p.stopped }
func (p *devicePlayer) UnexpectedStop() bool     { return !p.intentional.Load() }
func (p *devicePlayer) Underruns() uint64        { return p.underruns.Load() }

func (p *devicePlayer) SetVolume(volume float64) {
	p.volume.Store(float64Bits(max(0, min(1, volume))))
}

func (p *devicePlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	p.intentional.Store(true)
	close(p.done)
	if p.device != nil {
		p.device.Uninit()
		p.device = nil
	}
	return nil
}

// pcmRing is a single-producer/single-consumer lock-free byte ring. The audio
// callback only reads already-decoded samples and therefore never waits on
// FFmpeg, the filesystem, a mutex, or an allocation.
type pcmRing struct {
	data     []byte
	readPos  atomic.Uint64
	writePos atomic.Uint64
}

func newPCMRing(size int) *pcmRing { return &pcmRing{data: make([]byte, size)} }

func (r *pcmRing) available() int {
	return int(r.writePos.Load() - r.readPos.Load())
}

func (r *pcmRing) writeBytesAvailable() int { return len(r.data) - r.available() }

func (r *pcmRing) writeBytes(dst []byte, offset uint64, src []byte) int {
	n := min(len(src), len(r.data))
	start := int(offset % uint64(len(r.data)))
	first := min(n, len(r.data)-start)
	copy(dst[start:start+first], src[:first])
	copy(dst[:n-first], src[first:n])
	return n
}

func (r *pcmRing) write(src []byte) int {
	write := r.writePos.Load()
	n := min(len(src), r.writeBytesAvailable())
	if n <= 0 {
		return 0
	}
	r.writeBytes(r.data, write, src[:n])
	r.writePos.Store(write + uint64(n))
	return n
}

func (r *pcmRing) read(dst []byte) int {
	read := r.readPos.Load()
	n := min(len(dst), r.available())
	if n <= 0 {
		return 0
	}
	start := int(read % uint64(len(r.data)))
	first := min(n, len(r.data)-start)
	copy(dst[:first], r.data[start:start+first])
	copy(dst[first:n], r.data[:n-first])
	r.readPos.Store(read + uint64(n))
	return n
}

func float64Bits(value float64) uint64     { return math.Float64bits(value) }
func float64FromBits(value uint64) float64 { return math.Float64frombits(value) }
