package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

func (p *devicePlayer) SetRecoveryHandler(handler func(string) error) {
	p.mu.Lock()
	p.recovery = handler
	p.mu.Unlock()
}

func (p *devicePlayer) recoverTo(deviceID string) bool {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return true
	}
	handler := p.recovery
	p.mu.Unlock()
	return handler != nil && handler(deviceID) == nil
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
