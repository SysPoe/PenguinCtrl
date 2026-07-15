package media

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	audioFillBufferBytes = 32 * 1024
	audioRingBackoff     = 2 * time.Millisecond
)

// devicePlayer owns one PCM source's prebuffer, callback, and close lifecycle.
// Endpoint failover policy and decoder recovery handlers live in its registry.
type devicePlayer struct {
	reader         io.Reader
	mixer          *endpointMixer
	registry       *audioMixerRegistry
	volume         atomic.Uint64
	ring           *pcmRing
	done           chan struct{}
	ready          chan struct{}
	readyOnce      sync.Once
	stopped        chan struct{}
	stoppedOnce    sync.Once
	intentional    atomic.Bool
	eof            atomic.Bool
	underruns      atomic.Uint64
	mu             sync.Mutex
	closed         bool
	started        bool
	renderedFrames atomic.Uint64
}

func newDevicePlayer(reader io.Reader, mixer *endpointMixer, registry *audioMixerRegistry) *devicePlayer {
	player := &devicePlayer{
		reader: reader, mixer: mixer, registry: registry, ring: newPCMRing(audioRingBytes),
		done: make(chan struct{}), ready: make(chan struct{}), stopped: make(chan struct{}),
	}
	player.volume.Store(1)
	return player
}

func (p *devicePlayer) SetRecoveryHandler(handler func(string) error) {
	p.mu.Lock()
	registry := p.registry
	p.mu.Unlock()
	if registry != nil {
		registry.setRecoveryHandler(p, handler)
	}
}

func (p *devicePlayer) Start() error {
	select {
	case <-p.ready:
	case <-time.After(time.Second):
		return errors.New("audio prebuffer timed out")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("audio player is closed")
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

func (p *devicePlayer) mixInto(output []byte) {
	volume := math.Float64frombits(p.volume.Load())
	n := p.ring.mix(output, volume)
	p.renderedFrames.Add(uint64(len(output) / (audioChannels * 2)))
	if n < len(output) && !p.eof.Load() {
		p.underruns.Add(1)
	}
}

func (p *devicePlayer) RenderedPosition() time.Duration {
	frames := p.renderedFrames.Load()
	return time.Duration(frames) * time.Second / audioSampleRate
}

func (p *devicePlayer) fillRing() {
	buffer := make([]byte, audioFillBufferBytes)
	backoff := time.NewTimer(audioRingBackoff)
	if !backoff.Stop() {
		<-backoff.C
	}
	defer backoff.Stop()
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
				backoff.Reset(audioRingBackoff)
				select {
				case <-p.done:
					return
				case <-backoff.C:
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
	p.volume.Store(math.Float64bits(max(0, min(maxGainLinear, volume))))
}

func (p *devicePlayer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.intentional.Store(true)
	close(p.done)
	mixer, registry := p.mixer, p.registry
	p.mixer, p.registry = nil, nil
	p.mu.Unlock()
	if mixer != nil {
		mixer.remove(p)
	}
	if registry != nil {
		registry.removeSource(p)
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

func (r *pcmRing) writeBytes(offset uint64, src []byte) int {
	n := min(len(src), len(r.data))
	start := int(offset % uint64(len(r.data)))
	first := min(n, len(r.data)-start)
	copy(r.data[start:start+first], src[:first])
	copy(r.data[:n-first], src[first:n])
	return n
}

func (r *pcmRing) write(src []byte) int {
	write := r.writePos.Load()
	n := min(len(src), r.writeBytesAvailable())
	if n <= 0 {
		return 0
	}
	r.writeBytes(write, src[:n])
	r.writePos.Store(write + uint64(n))
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
