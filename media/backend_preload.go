package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os/exec"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/processgroup"
	_ "golang.org/x/image/webp"
)

func (s *ffmpegSession) rebind(request PlaybackRequest) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
}

func (s *ffmpegSession) Preload(ctx context.Context) error {
	started := time.Now()
	s.setState(LoadLoading)
	settings := s.backend.settings.Snapshot()
	info, err := probeMediaInfo(settings.FFmpegPath, s.path)
	if err != nil {
		return s.fail(err)
	}
	s.info = info
	bufferBytes := int64(0)
	if s.request.Instance.MediaType == "video" && info.HasVideo {
		width, height := decodeSize(info.Width, info.Height, config.VideoOutputFor(settings, s.request.Instance.OutputID))
		bufferBytes = int64(width) * int64(height) * 4 * (decodedFrameBuffer + 2)
	}
	if !s.backend.admission.acquire(ctx, s.ctx, bufferBytes) {
		return s.fail(errors.New("decoder admission cancelled or resource budget exhausted"))
	}
	s.mu.Lock()
	s.admitted, s.admittedBytes = true, bufferBytes
	s.mu.Unlock()
	results := make(chan error, 2)
	components := 0
	if s.request.Instance.MediaType == "video" && info.HasVideo {
		components++
		go func() { results <- s.preloadVideo(settings) }()
	}
	if (s.request.Instance.MediaType == "audio" || s.request.Instance.MediaType == "video") && info.HasAudio && s.backend.audio != nil {
		components++
		go func() { results <- s.preloadAudio(settings) }()
	}
	if components == 0 {
		s.Close()
		return s.fail(errors.New("media has no usable audio or video stream"))
	}
	timer := time.NewTimer(mediaPreloadTimeout)
	defer timer.Stop()
	for range components {
		select {
		case err := <-results:
			if err != nil {
				s.Close()
				return s.fail(err)
			}
		case <-ctx.Done():
			s.Close()
			return s.fail(ctx.Err())
		case <-timer.C:
			s.Close()
			return s.fail(errors.New("media preload timed out"))
		}
	}
	s.mu.Lock()
	s.metrics.LoadLatency = time.Since(started)
	s.mu.Unlock()
	s.setState(LoadReady)
	go func() {
		s.component.Wait()
		s.mu.RLock()
		closed, failed := s.closed, s.metrics.Error != ""
		s.mu.RUnlock()
		if !closed {
			if failed {
				s.setState(LoadFailed)
			} else {
				s.setState(LoadEnded)
			}
			s.doneOnce.Do(func() { close(s.done) })
		}
	}()
	return nil
}

func (s *ffmpegSession) preloadVideo(settings config.Settings) error {
	width, height := decodeSize(s.info.Width, s.info.Height, config.VideoOutputFor(settings, s.request.Instance.OutputID))
	s.info.Width, s.info.Height = width, height
	args := mediaInputArgs(s.request.Position, s.request.Instance.ClipEndMs)
	args = append(args, "-i", s.path, "-map", "0:v:0", "-an")
	if width > 0 && height > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", width, height))
	}
	args = append(args, "-fps_mode", "passthrough", "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	cmd := processgroup.CommandContext(s.ctx, settings.FFmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := processgroup.Start(cmd); err != nil {
		return err
	}
	s.mu.Lock()
	s.videoCmd = cmd
	s.mu.Unlock()
	first := make(chan error, 1)
	s.component.Add(1)
	go s.decodeVideo(cmd, stdout, first, &stderr)
	return <-first
}

// decodeSize caps software decoding and CPU-to-GPU frame uploads to the stage
// resolution. Upscaling stays in Gio, where it is much cheaper than producing
// and transferring oversized RGBA frames for every frame of a 4K/8K source.
func decodeSize(sourceWidth, sourceHeight int, output config.VideoOutput) (int, int) {
	if sourceWidth <= 0 || sourceHeight <= 0 || output.ResolutionWidth <= 0 || output.ResolutionHeight <= 0 {
		return sourceWidth, sourceHeight
	}
	if sourceWidth <= output.ResolutionWidth && sourceHeight <= output.ResolutionHeight {
		return sourceWidth, sourceHeight
	}
	scale := min(float64(output.ResolutionWidth)/float64(sourceWidth), float64(output.ResolutionHeight)/float64(sourceHeight))
	return max(1, int(math.Round(float64(sourceWidth)*scale))), max(1, int(math.Round(float64(sourceHeight)*scale)))
}

func (s *ffmpegSession) decodeVideo(cmd *exec.Cmd, reader io.Reader, first chan<- error, stderr *bytes.Buffer) {
	defer s.component.Done()
	frameSize := s.info.Width * s.info.Height * 4
	interval := s.info.FrameInterval()
	var index int64
	firstSent := false
	for {
		frame := s.acquireFrame()
		if _, err := io.ReadFull(reader, frame.Pix[:frameSize]); err != nil {
			if !firstSent {
				waitErr := cmd.Wait()
				if waitErr == nil {
					waitErr = err
				}
				first <- ffmpegCommandError("decode first video frame", waitErr, stderr.String())
				return
			}
			break
		}
		decoded := decodedFrame{image: frame, pts: s.request.Position + time.Duration(index)*interval}
		select {
		case s.frames <- decoded:
			index++
			s.mu.Lock()
			s.metrics.DecodedFrames++
			s.mu.Unlock()
			if !firstSent {
				firstSent = true
				first <- nil
			}
		case <-s.done:
			return
		}
	}
	if err := cmd.Wait(); err != nil {
		s.setRuntimeError(ffmpegCommandError("video decoder", err, stderr.String()))
	}
}

func (s *ffmpegSession) acquireFrame() *image.RGBA {
	if pooled := s.framePool.Get(); pooled != nil {
		frame := pooled.(*image.RGBA)
		if frame.Rect.Dx() == s.info.Width && frame.Rect.Dy() == s.info.Height {
			return frame
		}
	}
	return image.NewRGBA(image.Rect(0, 0, s.info.Width, s.info.Height))
}

func (s *ffmpegSession) preloadAudio(settings config.Settings) error {
	args := pcmDecodeArgs(s.request.Position, s.request.Instance.ClipEndMs, s.path)
	cmd := processgroup.CommandContext(s.ctx, settings.FFmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := processgroup.Start(cmd); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(stdout, 64*1024)
	if _, err := reader.Peek(4096); err != nil {
		_ = cmd.Process.Kill()
		waitErr := cmd.Wait()
		if waitErr == nil {
			waitErr = err
		}
		return ffmpegCommandError("preload audio", waitErr, stderr.String())
	}
	player, err := s.backend.audio.NewPreparedPlayer(reader, s.request.Instance.Preview)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	s.mu.Lock()
	s.audioGeneration++
	generation := s.audioGeneration
	s.audioCmd, s.audio = cmd, player
	s.audio.SetVolume(dbVolume(s.volume, s.muted))
	player.SetRecoveryHandler(s.recoverAudio)
	s.mu.Unlock()
	s.component.Add(1)
	go s.waitAudioCommand(cmd, &stderr, generation)
	return nil
}

func (s *ffmpegSession) waitAudioCommand(cmd *exec.Cmd, stderr *bytes.Buffer, generation uint64) {
	defer s.component.Done()
	err := cmd.Wait()
	s.mu.RLock()
	current, closed := s.audioGeneration == generation, s.closed
	s.mu.RUnlock()
	if err != nil && current && !closed {
		s.setRuntimeError(ffmpegCommandError("audio decoder", err, stderr.String()))
	}
}
