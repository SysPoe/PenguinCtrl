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

func (s *ffmpegSession) Start(clock SessionTimeline) error {
	s.mu.Lock()
	if s.closed || s.state != LoadReady {
		state := s.state
		s.mu.Unlock()
		return fmt.Errorf("media is not ready (state %s)", state)
	}
	audio := s.audio.player
	s.clock = clock
	s.mu.Unlock()
	if audio != nil {
		if err := audio.Start(); err != nil {
			return s.fail("start audio endpoint", err)
		}
		clock.BindMaster(audio.RenderedPosition)
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
		current := s.audio.player == audio
		cmd := s.audio.command
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
	s.video.mu.Lock()
	defer s.video.mu.Unlock()
	if s.video.pending == nil {
		select {
		case frame := <-s.video.frames:
			s.video.pending = &frame
		default:
		}
	}
	interval := s.video.info.FrameInterval()
	due := 0
	for s.video.pending != nil && s.video.pending.pts <= position+interval/2 {
		previous := s.video.current
		s.video.current, s.video.pending = s.video.pending, nil
		if previous != nil && previous.image != nil {
			s.video.framePool.Put(previous.image)
		}
		due++
		select {
		case next := <-s.video.frames:
			s.video.pending = &next
		default:
		}
	}
	var drift time.Duration
	if s.video.current != nil {
		drift = position - s.video.current.pts
		if drift < 0 {
			drift = -drift
		}
	}
	s.mu.Lock()
	if due > 1 {
		s.metrics.DroppedFrames += uint64(due - 1)
	}
	s.metrics.BufferedFrames = len(s.video.frames)
	if s.video.pending != nil {
		s.metrics.BufferedFrames++
	}
	buffering := s.video.current == nil || (position-s.video.current.pts > 2*interval && s.video.pending == nil)
	if buffering && s.state == LoadPlaying {
		s.state = LoadBuffering
		s.metrics.BufferingCount++
	} else if !buffering && s.state == LoadBuffering {
		s.state = LoadPlaying
	}
	s.metrics.State = s.state
	if s.video.current != nil {
		s.metrics.AVDrift = drift
	}
	s.mu.Unlock()
	if s.video.current == nil {
		return nil
	}
	return s.video.current.image
}

func (s *ffmpegSession) SetVolume(db float64) {
	s.mu.Lock()
	s.volume = db
	s.applyVolumeLocked()
	s.mu.Unlock()
}

func (s *ffmpegSession) SetMuted(muted bool) {
	s.mu.Lock()
	s.muted = muted
	s.applyVolumeLocked()
	s.mu.Unlock()
}

func (s *ffmpegSession) applyVolumeLocked() {
	if s.audio.player != nil {
		s.audio.player.SetVolume(dbVolume(s.volume, s.muted))
	}
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
	if s.audio.player != nil {
		metrics.AudioUnderruns = s.audio.player.Underruns()
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
	videoCmd, audioCmd, audio := s.video.command, s.audio.command, s.audio.player
	admitted, admittedBytes := s.admitted, s.admittedBytes
	s.admitted = false
	s.audio.player = nil
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

func (s *ffmpegSession) fail(operation string, err error) error {
	s.setState(LoadFailed)
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *ffmpegSession) setRuntimeError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.metrics.Error = err.Error()
	s.mu.Unlock()
}
