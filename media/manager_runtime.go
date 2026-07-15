package media

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

// mediaRuntime owns decoder sessions and hardware-audio resources as one reset
// generation. Callers never close either resource independently.
type mediaRuntime struct {
	mu       sync.RWMutex
	settings *config.Store
	audio    *AudioSystem
	decoder  RuntimeBackend
	closed   bool
}

func newMediaRuntime(settings *config.Store) (*mediaRuntime, error) {
	audio, err := NewAudioSystem(settings)
	if err != nil {
		log.Printf("initialize audio output: %v", err)
	}
	return &mediaRuntime{settings: settings, audio: audio, decoder: NewFFmpegBackend(settings, audio)}, err
}

func (runtime *mediaRuntime) prewarm(specs []playback.PreloadSpec) {
	requests := make([]PlaybackRequest, 0, len(specs))
	for _, spec := range specs {
		instance := playback.Instance{
			CueID: spec.CueID, CueNumber: spec.CueNumber, MediaType: spec.MediaType,
			Source: spec.Source, OutputID: spec.OutputID, ClipStartMs: spec.ClipStartMs,
			ClipEndMs: spec.ClipEndMs, Preview: spec.Preview,
		}
		requests = append(requests, PlaybackRequest{
			Instance: instance,
			Position: time.Duration(max(int64(0), spec.ClipStartMs)) * time.Millisecond,
		})
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if runtime.decoder != nil {
		runtime.decoder.Prewarm(requests)
	}
}

func (runtime *mediaRuntime) backend() PlaybackBackend {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.decoder
}

func (runtime *mediaRuntime) mixerMetrics() []AudioMixerMetrics {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if runtime.audio == nil {
		return nil
	}
	return runtime.audio.Metrics()
}

func (runtime *mediaRuntime) audioSnapshot() ([]AudioDevice, []AudioMixerMetrics, error) {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if runtime.audio == nil {
		return nil, nil, errors.New("audio output is unavailable")
	}
	devices, err := runtime.audio.Devices()
	return devices, runtime.audio.Metrics(), err
}

func (runtime *mediaRuntime) reset() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.decoder != nil {
		runtime.decoder.Close()
	}
	if runtime.audio != nil {
		runtime.audio.Close()
	}
	audio, err := NewAudioSystem(runtime.settings)
	runtime.audio = audio
	runtime.decoder = NewFFmpegBackend(runtime.settings, audio)
	return err
}

func (runtime *mediaRuntime) close() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return
	}
	runtime.closed = true
	if runtime.decoder != nil {
		runtime.decoder.Close()
		runtime.decoder = nil
	}
	if runtime.audio != nil {
		runtime.audio.Close()
		runtime.audio = nil
	}
}
