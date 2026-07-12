package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

const decodedFrameBuffer = 2

type LoadState string

const (
	LoadIdle      LoadState = "idle"
	LoadLoading   LoadState = "loading"
	LoadReady     LoadState = "ready"
	LoadPlaying   LoadState = "playing"
	LoadBuffering LoadState = "buffering"
	LoadPaused    LoadState = "paused"
	LoadEnded     LoadState = "ended"
	LoadFailed    LoadState = "failed"
	LoadClosed    LoadState = "closed"
)

type PlaybackMetrics struct {
	State          LoadState
	LoadLatency    time.Duration
	StartLatency   time.Duration
	DecodedFrames  uint64
	DroppedFrames  uint64
	BufferingCount uint64
	BufferedFrames int
	AudioUnderruns uint64
	Error          string
}

type PlaybackRequest struct {
	Instance    playback.Instance
	Position    time.Duration
	RequestedAt time.Time
}

// PlaybackBackend is the decoder seam used by Player. Implementations preload
// bounded media data, then start all device output from a shared clock.
type PlaybackBackend interface {
	Open(PlaybackRequest) (PlaybackSession, error)
}

type PlaybackSession interface {
	Preload(context.Context) error
	Start(*PlaybackClock) error
	Frame(time.Duration) image.Image
	SetVolume(float64)
	SetMuted(bool)
	State() LoadState
	Metrics() PlaybackMetrics
	Done() <-chan struct{}
	Close()
}

type FFmpegBackend struct {
	settings *config.Store
	audio    *AudioSystem
}

func NewFFmpegBackend(settings *config.Store, audio *AudioSystem) *FFmpegBackend {
	return &FFmpegBackend{settings: settings, audio: audio}
}

func (b *FFmpegBackend) Open(request PlaybackRequest) (PlaybackSession, error) {
	path, err := sourcePath(request.Instance.Source)
	if err != nil {
		return nil, err
	}
	if request.RequestedAt.IsZero() {
		request.RequestedAt = time.Now()
	}
	return &ffmpegSession{
		backend: b, request: request, path: path, state: LoadIdle,
		frames: make(chan decodedFrame, decodedFrameBuffer), done: make(chan struct{}),
	}, nil
}

type decodedFrame struct {
	image *image.RGBA
	pts   time.Duration
}

type ffmpegSession struct {
	backend *FFmpegBackend
	request PlaybackRequest
	path    string
	info    mediaInfo

	mu        sync.RWMutex
	state     LoadState
	metrics   PlaybackMetrics
	muted     bool
	volume    float64
	videoCmd  *exec.Cmd
	audioCmd  *exec.Cmd
	audio     *devicePlayer
	closed    bool
	done      chan struct{}
	doneOnce  sync.Once
	component sync.WaitGroup

	frames  chan decodedFrame
	frameMu sync.Mutex
	current *decodedFrame
	pending *decodedFrame
}

func (s *ffmpegSession) Preload(ctx context.Context) error {
	started := time.Now()
	s.setState(LoadLoading)
	info, err := probeMediaInfo(s.backend.settings.Snapshot().FFmpegPath, s.path)
	if err != nil {
		return s.fail(err)
	}
	s.info = info
	type result struct{ err error }
	results := make(chan result, 2)
	components := 0
	if s.request.Instance.MediaType == "video" && info.hasVideo {
		components++
		go func() { results <- result{err: s.preloadVideo()} }()
	}
	if (s.request.Instance.MediaType == "audio" || s.request.Instance.MediaType == "video") && info.hasAudio && s.backend.audio != nil {
		components++
		go func() { results <- result{err: s.preloadAudio()} }()
	}
	if components == 0 {
		return s.fail(errors.New("media has no usable audio or video stream"))
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for range components {
		select {
		case result := <-results:
			if result.err != nil {
				s.Close()
				return s.fail(result.err)
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

func (s *ffmpegSession) preloadVideo() error {
	args := mediaInputArgs(s.request.Position, s.request.Instance.ClipEndMs)
	args = append(args, "-i", s.path, "-map", "0:v:0", "-an", "-fps_mode", "passthrough", "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	cmd := exec.Command(s.backend.settings.Snapshot().FFmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
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

func (s *ffmpegSession) decodeVideo(cmd *exec.Cmd, reader io.Reader, first chan<- error, stderr *bytes.Buffer) {
	defer s.component.Done()
	frameSize := s.info.width * s.info.height * 4
	interval := s.info.frameInterval()
	var index int64
	firstSent := false
	for {
		frame := image.NewRGBA(image.Rect(0, 0, s.info.width, s.info.height))
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

func (s *ffmpegSession) preloadAudio() error {
	args := mediaInputArgs(s.request.Position, s.request.Instance.ClipEndMs)
	args = append(args, "-i", s.path, "-map", "0:a:0", "-vn", "-f", "s16le", "-ar", strconv.Itoa(audioSampleRate), "-ac", strconv.Itoa(audioChannels), "pipe:1")
	cmd := exec.Command(s.backend.settings.Snapshot().FFmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
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
	s.audioCmd, s.audio = cmd, player
	s.audio.SetVolume(dbVolume(s.volume, s.muted))
	s.mu.Unlock()
	s.component.Add(1)
	go func() {
		defer s.component.Done()
		if err := cmd.Wait(); err != nil {
			s.setRuntimeError(ffmpegCommandError("audio decoder", err, stderr.String()))
		}
	}()
	return nil
}

func (s *ffmpegSession) Start(clock *PlaybackClock) error {
	s.mu.Lock()
	if s.closed || s.state != LoadReady {
		state := s.state
		s.mu.Unlock()
		return fmt.Errorf("media is not ready (state %s)", state)
	}
	audio := s.audio
	s.mu.Unlock()
	clock.Start()
	if audio != nil {
		if err := audio.Start(); err != nil {
			return s.fail(err)
		}
		go func() {
			select {
			case <-audio.Stopped():
				if !audio.UnexpectedStop() {
					return
				}
				err := errors.New("audio device stopped unexpectedly")
				s.setRuntimeError(err)
				s.mu.RLock()
				cmd := s.audioCmd
				s.mu.RUnlock()
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				s.doneOnce.Do(func() { close(s.done) })
			case <-s.done:
			}
		}()
	}
	s.mu.Lock()
	s.metrics.StartLatency = time.Since(s.request.RequestedAt)
	s.mu.Unlock()
	s.setState(LoadPlaying)
	return nil
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
		s.current, s.pending = s.pending, nil
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
	return s.current.image
}

func (s *ffmpegSession) SetVolume(db float64) {
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
	s.audio = nil
	s.mu.Unlock()
	s.doneOnce.Do(func() { close(s.done) })
	if videoCmd != nil && videoCmd.Process != nil {
		_ = videoCmd.Process.Kill()
	}
	if audioCmd != nil && audioCmd.Process != nil {
		_ = audioCmd.Process.Kill()
	}
	if audio != nil {
		_ = audio.Close()
	}
}

func (s *ffmpegSession) setState(state LoadState) {
	s.mu.Lock()
	s.state, s.metrics.State = state, state
	s.mu.Unlock()
}

func (s *ffmpegSession) fail(err error) error { s.setState(LoadFailed); return err }

func (s *ffmpegSession) setRuntimeError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.metrics.Error = err.Error()
	s.mu.Unlock()
}

func ffmpegCommandError(operation string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("ffmpeg %s failed: %w", operation, err)
	}
	return fmt.Errorf("ffmpeg %s failed: %w: %s", operation, err, detail)
}

type mediaInfo struct {
	width, height int
	fps           float64
	hasVideo      bool
	hasAudio      bool
}

func (i mediaInfo) frameInterval() time.Duration {
	if i.fps <= 0 {
		return time.Second / 30
	}
	return time.Duration(float64(time.Second) / i.fps)
}

func probeMediaInfo(ffmpegPath, source string) (mediaInfo, error) {
	command := exec.Command(ffprobePath(ffmpegPath), "-v", "error", "-show_entries", "stream=codec_type,width,height,avg_frame_rate", "-of", "json", source)
	raw, err := command.CombinedOutput()
	if err != nil {
		return mediaInfo{}, ffmpegCommandError("probe media streams", err, string(raw))
	}
	var result struct {
		Streams []struct {
			CodecType    string `json:"codec_type"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return mediaInfo{}, err
	}
	var info mediaInfo
	for _, stream := range result.Streams {
		switch stream.CodecType {
		case "video":
			if !info.hasVideo {
				info.hasVideo, info.width, info.height = true, stream.Width, stream.Height
				info.fps = parseFrameRate(stream.AvgFrameRate)
			}
		case "audio":
			info.hasAudio = true
		}
	}
	return info, nil
}

func ValidateSource(ffmpegPath, source string, cueType show.CueType) error {
	if cueType == show.CueTypeImage {
		path, err := sourcePath(source)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open image: %w", err)
		}
		defer file.Close()
		if _, _, err := image.DecodeConfig(file); err != nil {
			return fmt.Errorf("unsupported image: %w", err)
		}
		return nil
	}
	info, err := probeMediaInfo(ffmpegPath, source)
	if err != nil {
		return err
	}
	if cueType == show.CueTypeSound && !info.hasAudio {
		return errors.New("audio stream could not be opened: file has no audio stream")
	}
	if cueType == show.CueTypeVideo && !info.hasVideo {
		return errors.New("video stream could not be opened: file has no video stream")
	}
	return nil
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		numerator, errN := strconv.ParseFloat(parts[0], 64)
		denominator, errD := strconv.ParseFloat(parts[1], 64)
		if errN == nil && errD == nil && denominator > 0 {
			return numerator / denominator
		}
	}
	fps, _ := strconv.ParseFloat(value, 64)
	return fps
}
