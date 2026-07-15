package media

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"time"

	_ "golang.org/x/image/webp"
)

func (s *ffmpegSession) Start(clock *PlaybackClock) error {
	s.mu.Lock()
	if s.closed || s.state != LoadReady {
		state := s.state
		s.mu.Unlock()
		return fmt.Errorf("media is not ready (state %s)", state)
	}
	audio := s.audio
	s.clock = clock
	s.mu.Unlock()
	clock.Start()
	if audio != nil {
		if err := audio.Start(); err != nil {
			return s.fail(err)
		}
		clock.SetMaster(audio.RenderedPosition)
		go s.watchAudioDevice(audio)
	}
	s.mu.Lock()
	s.metrics.StartLatency = time.Since(s.request.RequestedAt)
	s.mu.Unlock()
	s.setState(LoadPlaying)
	return nil
}

func (s *ffmpegSession) watchAudioDevice(audio *devicePlayer) {
	select {
	case <-audio.Stopped():
		if !audio.UnexpectedStop() {
			return
		}
		s.mu.RLock()
		current := s.audio == audio
		cmd := s.audioCmd
		s.mu.RUnlock()
		if !current {
			return
		}
		s.setRuntimeError(errors.New("audio device stopped unexpectedly after recovery was exhausted"))
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		s.doneOnce.Do(func() { close(s.done) })
	case <-s.done:
	}
}

func (s *ffmpegSession) Frame(position time.Duration) image.Image {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	if s.pending == nil {
		select {
		case frame := <-s.frames:
			s.pending = &frame
		default:
		}
	}
	interval := s.info.frameInterval()
	due := 0
	for s.pending != nil && s.pending.pts <= position+interval/2 {
		previous := s.current
		s.current, s.pending = s.pending, nil
		if previous != nil {
			s.recycleFrame(previous.image)
		}
		due++
		select {
		case next := <-s.frames:
			s.pending = &next
		default:
		}
	}
	s.mu.Lock()
	if due > 1 {
		s.metrics.DroppedFrames += uint64(due - 1)
	}
	s.metrics.BufferedFrames = len(s.frames)
	if s.pending != nil {
		s.metrics.BufferedFrames++
	}
	buffering := s.current == nil || (position-s.current.pts > 2*interval && s.pending == nil)
	if buffering && s.state == LoadPlaying {
		s.state = LoadBuffering
		s.metrics.BufferingCount++
	} else if !buffering && s.state == LoadBuffering {
		s.state = LoadPlaying
	}
	s.metrics.State = s.state
	s.mu.Unlock()
	if s.current == nil {
		return nil
	}
	drift := position - s.current.pts
	if drift < 0 {
		drift = -drift
	}
	s.mu.Lock()
	// TODO(micro): metrics updates for DroppedFrames/BufferedFrames and AVDrift take separate Lock/Unlock pairs in one Frame call; merge into one critical section.
	s.metrics.AVDrift = drift
	s.mu.Unlock()
	return s.current.image
}

func (s *ffmpegSession) SetVolume(db float64) {
	// TODO(micro): SetVolume/SetMuted duplicate the same lock+audio.SetVolume(dbVolume(...)) body; share one applyVolumeLocked helper.
	s.mu.Lock()
	s.volume = db
	if s.audio != nil {
		s.audio.SetVolume(dbVolume(db, s.muted))
	}
	s.mu.Unlock()
}

func (s *ffmpegSession) SetMuted(muted bool) {
	s.mu.Lock()
	s.muted = muted
	if s.audio != nil {
		s.audio.SetVolume(dbVolume(s.volume, muted))
	}
	s.mu.Unlock()
}

func (s *ffmpegSession) State() LoadState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *ffmpegSession) Metrics() PlaybackMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	metrics := s.metrics
	if s.audio != nil {
		metrics.AudioUnderruns = s.audio.Underruns()
	}
	metrics.State = s.state
	return metrics
}

func (s *ffmpegSession) Done() <-chan struct{} { return s.done }

func (s *ffmpegSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed, s.state, s.metrics.State = true, LoadClosed, LoadClosed
	videoCmd, audioCmd, audio := s.videoCmd, s.audioCmd, s.audio
	admitted, admittedBytes := s.admitted, s.admittedBytes
	s.admitted = false
	s.audio = nil
	s.mu.Unlock()
	s.doneOnce.Do(func() { close(s.done) })
	s.cancel()
	if videoCmd != nil && videoCmd.Process != nil {
		_ = videoCmd.Process.Kill()
	}
	if audioCmd != nil && audioCmd.Process != nil {
		_ = audioCmd.Process.Kill()
	}
	if audio != nil {
		_ = audio.Close()
	}
	if admitted {
		s.backend.admission.release(admittedBytes)
	}
	s.backend.release(s)
}

func (s *ffmpegSession) setState(state LoadState) {
	s.mu.Lock()
	s.state, s.metrics.State = state, state
	s.mu.Unlock()
}

// TODO(micro): fail wraps setState without enriching err; either wrap with context or have callers setState and return err directly.
func (s *ffmpegSession) fail(err error) error { s.setState(LoadFailed); return err }

func (s *ffmpegSession) setRuntimeError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.metrics.Error = err.Error()
	s.mu.Unlock()
}
