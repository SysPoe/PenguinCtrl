package media

import (
	"encoding/binary"
	"errors"
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

type AudioMixerMetrics struct {
	EndpointID     string
	ActiveSources  int
	Recovering     bool
	Failed         bool
	RecoveryCount  uint64
	LastCallback   time.Time
	MaxCallback    time.Duration
	TotalUnderruns uint64
}

type AudioSystem struct {
	settings *config.Store
	context  *malgo.AllocatedContext
	mu       sync.Mutex
	mixers   map[string]*endpointMixer
	closed   bool
}

func NewAudioSystem(settings *config.Store) (*AudioSystem, error) {
	context, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio system: %w", err)
	}
	return &AudioSystem{settings: settings, context: context, mixers: map[string]*endpointMixer{}}, nil
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

	mixer, err := a.mixer(deviceID)
	if err != nil {
		return nil, err
	}

	player := &devicePlayer{
		reader: reader, ring: newPCMRing(audioRingBytes), done: make(chan struct{}),
		ready: make(chan struct{}), stopped: make(chan struct{}), mixer: mixer,
	}
	player.volume.Store(1)
	go player.fillRing()
	return player, nil
}

func (a *AudioSystem) mixer(deviceID string) (*endpointMixer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.context == nil {
		return nil, errors.New("audio system is closed")
	}
	if mixer := a.mixers[deviceID]; mixer != nil {
		return mixer, nil
	}
	mixer := &endpointMixer{system: a, deviceID: deviceID}
	mixer.sources.Store([]*devicePlayer{})
	if err := mixer.openDeviceLocked(); err != nil {
		return nil, err
	}
	a.mixers[deviceID] = mixer
	return mixer, nil
}

func (a *AudioSystem) deviceConfig(deviceID string) (malgo.DeviceConfig, error) {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = audioChannels
	deviceConfig.SampleRate = audioSampleRate
	if deviceID == "" {
		return deviceConfig, nil
	}
	devices, err := a.context.Devices(malgo.Playback)
	if err != nil {
		return deviceConfig, fmt.Errorf("list audio devices: %w", err)
	}
	for i := range devices {
		if devices[i].ID.String() == deviceID {
			deviceConfig.Playback.DeviceID = devices[i].ID.Pointer()
			return deviceConfig, nil
		}
	}
	return deviceConfig, errors.New("selected audio device is unavailable")
}

func (a *AudioSystem) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	mixers := make([]*endpointMixer, 0, len(a.mixers))
	for _, mixer := range a.mixers {
		mixers = append(mixers, mixer)
	}
	a.mixers = nil
	a.mu.Unlock()
	for _, mixer := range mixers {
		mixer.close()
	}
	if a.context != nil {
		_ = a.context.Uninit()
		a.context.Free()
		a.context = nil
	}
}

func (a *AudioSystem) Metrics() []AudioMixerMetrics {
	a.mu.Lock()
	mixers := make([]*endpointMixer, 0, len(a.mixers))
	for _, mixer := range a.mixers {
		mixers = append(mixers, mixer)
	}
	a.mu.Unlock()
	result := make([]AudioMixerMetrics, 0, len(mixers))
	for _, mixer := range mixers {
		sources := mixer.sources.Load().([]*devicePlayer)
		metrics := AudioMixerMetrics{
			EndpointID: mixer.deviceID, ActiveSources: len(sources), Recovering: mixer.recovering.Load(),
			Failed: mixer.failed.Load(), RecoveryCount: mixer.recoveries.Load(),
			MaxCallback: time.Duration(mixer.callbackMax.Load()),
		}
		if at := mixer.callbackAt.Load(); at > 0 {
			metrics.LastCallback = time.Unix(0, at)
		}
		for _, source := range sources {
			metrics.TotalUnderruns += source.Underruns()
		}
		result = append(result, metrics)
	}
	return result
}

type endpointMixer struct {
	system      *AudioSystem
	deviceID    string
	mu          sync.Mutex
	device      *malgo.Device
	sources     atomic.Value // immutable []*devicePlayer, read without locks by callback
	started     bool
	closed      bool
	recovering  atomic.Bool
	failed      atomic.Bool
	recoveries  atomic.Uint64
	callbackAt  atomic.Int64
	callbackMax atomic.Int64
}

func (m *endpointMixer) openDeviceLocked() error {
	config, err := m.system.deviceConfig(m.deviceID)
	if err != nil {
		return err
	}
	callbacks := malgo.DeviceCallbacks{Data: m.mix, Stop: m.deviceStopped}
	device, err := malgo.InitDevice(m.system.context.Context, config, callbacks)
	if err != nil {
		return fmt.Errorf("open audio device: %w", err)
	}
	m.device = device
	return nil
}

func (m *endpointMixer) add(player *devicePlayer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.device == nil {
		return errors.New("audio mixer is unavailable")
	}
	current := m.sources.Load().([]*devicePlayer)
	for _, source := range current {
		if source == player {
			return nil
		}
	}
	next := append(append([]*devicePlayer(nil), current...), player)
	m.sources.Store(next)
	if !m.started {
		if err := m.device.Start(); err != nil {
			m.sources.Store(current)
			return fmt.Errorf("start audio device: %w", err)
		}
		m.started = true
	}
	return nil
}

func (m *endpointMixer) remove(player *devicePlayer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.sources.Load().([]*devicePlayer)
	next := make([]*devicePlayer, 0, len(current))
	for _, source := range current {
		if source != player {
			next = append(next, source)
		}
	}
	m.sources.Store(next)
}

func (m *endpointMixer) mix(output, _ []byte, _ uint32) {
	started := time.Now()
	clear(output)
	for _, source := range m.sources.Load().([]*devicePlayer) {
		source.mixInto(output)
	}
	now := time.Now()
	m.callbackAt.Store(now.UnixNano())
	duration := now.Sub(started).Nanoseconds()
	for previous := m.callbackMax.Load(); duration > previous && !m.callbackMax.CompareAndSwap(previous, duration); previous = m.callbackMax.Load() {
	}
}

func (m *endpointMixer) deviceStopped() {
	m.mu.Lock()
	intentional := m.closed
	m.started = false
	m.mu.Unlock()
	if intentional || !m.recovering.CompareAndSwap(false, true) {
		return
	}
	go m.recover()
}

func (m *endpointMixer) recover() {
	defer m.recovering.Store(false)
	backoff := 250 * time.Millisecond
	for attempt := 0; attempt < 6; attempt++ {
		time.Sleep(backoff)
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}
		old := m.device
		m.device = nil
		m.mu.Unlock()
		if old != nil {
			old.Uninit()
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}
		err := m.openDeviceLocked()
		if err == nil && len(m.sources.Load().([]*devicePlayer)) > 0 {
			err = m.device.Start()
			m.started = err == nil
		}
		m.mu.Unlock()
		if err == nil {
			m.failed.Store(false)
			m.recoveries.Add(1)
			return
		}
		backoff = min(4*time.Second, backoff*2)
	}
	for _, source := range m.sources.Load().([]*devicePlayer) {
		source.stoppedOnce.Do(func() { close(source.stopped) })
	}
	m.failed.Store(true)
}

func (m *endpointMixer) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	device := m.device
	m.device = nil
	m.mu.Unlock()
	if device != nil {
		device.Uninit()
	}
}

type devicePlayer struct {
	reader      io.Reader
	mixer       *endpointMixer
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
	if p.mixer == nil {
		return errors.New("audio mixer is unavailable")
	}
	if err := p.mixer.add(p); err != nil {
		return err
	}
	p.started = true
	return nil
}

func (p *devicePlayer) readSamples(output, _ []byte, _ uint32) {
	clear(output)
	p.mixInto(output)
}

func (p *devicePlayer) mixInto(output []byte) {
	volume := float64FromBits(p.volume.Load())
	n := p.ring.mix(output, volume)
	if n < len(output) && !p.eof.Load() {
		p.underruns.Add(1)
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

func (p *devicePlayer) Stopped() <-chan struct{} { return p.stopped }
func (p *devicePlayer) UnexpectedStop() bool     { return !p.intentional.Load() }
func (p *devicePlayer) Underruns() uint64        { return p.underruns.Load() }

func (p *devicePlayer) SetVolume(volume float64) {
	p.volume.Store(float64Bits(max(0, min(math.Pow(10, 12.0/20), volume))))
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
	if p.mixer != nil {
		p.mixer.remove(p)
		p.mixer = nil
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

// mix consumes ready S16 samples and adds them to an existing output buffer.
// It performs no allocation and takes no lock, keeping endpoint callbacks
// independent from decoders, filesystems, and UI work.
func (r *pcmRing) mix(output []byte, gain float64) int {
	read := r.readPos.Load()
	n := min(len(output), r.available())
	n -= n % 2
	for i := 0; i < n; i += 2 {
		first := r.data[int((read+uint64(i))%uint64(len(r.data)))]
		second := r.data[int((read+uint64(i+1))%uint64(len(r.data)))]
		incoming := int16(uint16(first) | uint16(second)<<8)
		existing := int16(binary.LittleEndian.Uint16(output[i:]))
		mixed := float64(existing) + float64(incoming)*gain
		mixed = max(-32768.0, min(32767.0, mixed))
		binary.LittleEndian.PutUint16(output[i:], uint16(int16(mixed)))
	}
	r.readPos.Store(read + uint64(n))
	return n
}

func float64Bits(value float64) uint64     { return math.Float64bits(value) }
func float64FromBits(value uint64) float64 { return math.Float64frombits(value) }
