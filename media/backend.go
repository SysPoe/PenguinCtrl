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
	"math"
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

const (
	decodedFrameBuffer = 2
	mediaProbeTimeout  = 10 * time.Second
	maxVideoDimension  = 8192
	maxVideoPixels     = 7680 * 4320
	maxVideoFrameRate  = 240
)

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
	settings   *config.Store
	audio      *AudioSystem
	warmMu     sync.Mutex
	warm       map[string]warmSession
	warming    map[string]struct{}
	warmFailed map[string]time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
}

type warmSession struct {
	session PlaybackSession
	warmed  time.Time
}

func NewFFmpegBackend(settings *config.Store, audio *AudioSystem) *FFmpegBackend {
	ctx, cancel := context.WithCancel(context.Background())
	return &FFmpegBackend{settings: settings, audio: audio, warm: map[string]warmSession{}, warming: map[string]struct{}{}, warmFailed: map[string]time.Time{}, ctx: ctx, cancel: cancel}
}

func (b *FFmpegBackend) Open(request PlaybackRequest) (PlaybackSession, error) {
	key := b.warmKey(request)
	b.warmMu.Lock()
	if b.closed {
		b.warmMu.Unlock()
		return nil, errors.New("media backend is closed")
	}
	if warmed, ok := b.warm[key]; ok {
		delete(b.warm, key)
		b.warmMu.Unlock()
		if session, ok := warmed.session.(*ffmpegSession); ok {
			session.rebind(request)
		}
		return warmed.session, nil
	}
	b.warmMu.Unlock()
	return b.openFresh(request)
}

func (b *FFmpegBackend) openFresh(request PlaybackRequest) (PlaybackSession, error) {
	path, err := sourcePath(request.Instance.Source)
	if err != nil {
		return nil, err
	}
	if request.RequestedAt.IsZero() {
		request.RequestedAt = time.Now()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ffmpegSession{
		backend: b, request: request, path: path, state: LoadIdle,
		frames: make(chan decodedFrame, decodedFrameBuffer), done: make(chan struct{}),
		ctx: ctx, cancel: cancel,
	}, nil
}

// Prewarm starts bounded decoder and audio prebuffer work for imminent cues.
// A later Open atomically claims the ready session; stale entries are closed.
func (b *FFmpegBackend) Prewarm(requests []PlaybackRequest) {
	for _, request := range requests {
		request := request
		key := b.warmKey(request)
		b.warmMu.Lock()
		if b.closed {
			b.warmMu.Unlock()
			return
		}
		_, ready := b.warm[key]
		_, running := b.warming[key]
		failedAt := b.warmFailed[key]
		if ready || running || (!failedAt.IsZero() && time.Since(failedAt) < 30*time.Second) {
			b.warmMu.Unlock()
			continue
		}
		b.warming[key] = struct{}{}
		b.warmMu.Unlock()
		go func() {
			ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
			defer cancel()
			session, err := b.openFresh(request)
			if err == nil {
				err = session.Preload(ctx)
			}
			b.warmMu.Lock()
			delete(b.warming, key)
			if err == nil && !b.closed {
				b.warm[key] = warmSession{session: session, warmed: time.Now()}
				delete(b.warmFailed, key)
			} else if err == nil {
				err = context.Canceled
			} else if !b.closed {
				b.warmFailed[key] = time.Now()
			}
			var stale []PlaybackSession
			for len(b.warm) > 4 {
				oldestKey := ""
				var oldest time.Time
				for candidate, warmed := range b.warm {
					if oldestKey == "" || warmed.warmed.Before(oldest) {
						oldestKey, oldest = candidate, warmed.warmed
					}
				}
				stale = append(stale, b.warm[oldestKey].session)
				delete(b.warm, oldestKey)
			}
			b.warmMu.Unlock()
			if err != nil && session != nil {
				session.Close()
			}
			for _, session := range stale {
				session.Close()
			}
		}()
	}
}

func (b *FFmpegBackend) warmKey(request PlaybackRequest) string {
	settings := b.settings.Snapshot()
	device, policy, backup := config.AudioRoute(settings, request.Instance.Preview)
	video := config.VideoOutputFor(settings, request.Instance.OutputID)
	return fmt.Sprintf("%s|%s|%s|%d|%d|%t|%s|%s|%s|%dx%d", settings.FFmpegPath, request.Instance.MediaType,
		request.Instance.Source, request.Position.Milliseconds(), request.Instance.ClipEndMs, request.Instance.Preview,
		device, policy, backup, video.ResolutionWidth, video.ResolutionHeight)
}

func (b *FFmpegBackend) Close() {
	b.warmMu.Lock()
	if b.closed {
		b.warmMu.Unlock()
		return
	}
	b.closed = true
	b.cancel()
	sessions := make([]PlaybackSession, 0, len(b.warm))
	for _, warmed := range b.warm {
		sessions = append(sessions, warmed.session)
	}
	b.warm = map[string]warmSession{}
	b.warmMu.Unlock()
	for _, session := range sessions {
		session.Close()
	}
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

	mu              sync.RWMutex
	state           LoadState
	metrics         PlaybackMetrics
	muted           bool
	volume          float64
	videoCmd        *exec.Cmd
	audioCmd        *exec.Cmd
	audio           *devicePlayer
	clock           *PlaybackClock
	audioGeneration uint64
	closed          bool
	done            chan struct{}
	doneOnce        sync.Once
	component       sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc

	frames  chan decodedFrame
	frameMu sync.Mutex
	current *decodedFrame
	pending *decodedFrame
}

func (s *ffmpegSession) rebind(request PlaybackRequest) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
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
	settings := s.backend.settings.Snapshot()
	width, height := decodeSize(s.info.width, s.info.height, config.VideoOutputFor(settings, s.request.Instance.OutputID))
	s.info.width, s.info.height = width, height
	args := mediaInputArgs(s.request.Position, s.request.Instance.ClipEndMs)
	args = append(args, "-i", s.path, "-map", "0:v:0", "-an")
	if width > 0 && height > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", width, height))
	}
	args = append(args, "-fps_mode", "passthrough", "-f", "rawvideo", "-pix_fmt", "rgba", "pipe:1")
	cmd := exec.Command(settings.FFmpegPath, args...)
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
	cmd := exec.CommandContext(s.ctx, s.backend.settings.Snapshot().FFmpegPath, args...)
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

func (s *ffmpegSession) recoverAudio(targetDeviceID string) error {
	s.mu.Lock()
	if s.closed || s.clock == nil || s.audio == nil {
		s.mu.Unlock()
		return errors.New("media session is not available for audio recovery")
	}
	position := s.clock.Position()
	oldCommand, oldPlayer := s.audioCmd, s.audio
	s.audioGeneration++
	generation := s.audioGeneration
	volume, muted := s.volume, s.muted
	s.component.Add(1)
	s.mu.Unlock()
	if oldCommand != nil && oldCommand.Process != nil {
		_ = oldCommand.Process.Kill()
	}

	args := mediaInputArgs(position, s.request.Instance.ClipEndMs)
	args = append(args, "-i", s.path, "-map", "0:a:0", "-vn", "-f", "s16le", "-ar", strconv.Itoa(audioSampleRate), "-ac", strconv.Itoa(audioChannels), "pipe:1")
	cmd := exec.CommandContext(s.ctx, s.backend.settings.Snapshot().FFmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.component.Done()
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		s.component.Done()
		return err
	}
	reader := bufio.NewReaderSize(stdout, 64*1024)
	if _, err := reader.Peek(4096); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		s.component.Done()
		return ffmpegCommandError("recover audio", err, stderr.String())
	}
	_, policy, backupID := config.AudioRoute(s.backend.settings.Snapshot(), s.request.Instance.Preview)
	replacement, err := s.backend.audio.newPreparedPlayer(reader, targetDeviceID, policy, backupID)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		s.component.Done()
		return err
	}
	replacement.SetRecoveryHandler(s.recoverAudio)
	replacement.SetVolume(dbVolume(volume, muted))
	if err := replacement.Start(); err != nil {
		_ = replacement.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		s.component.Done()
		return err
	}
	s.mu.Lock()
	if s.closed || s.audioGeneration != generation {
		s.mu.Unlock()
		_ = replacement.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		s.component.Done()
		return errors.New("audio recovery was superseded")
	}
	s.audioCmd, s.audio = cmd, replacement
	s.mu.Unlock()
	_ = oldPlayer.Close()
	go s.waitAudioCommand(cmd, &stderr, generation)
	go s.watchAudioDevice(replacement)
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
	s.clock = clock
	s.mu.Unlock()
	clock.Start()
	if audio != nil {
		if err := audio.Start(); err != nil {
			return s.fail(err)
		}
		go s.watchAudioDevice(audio)
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
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, ffprobePath(ffmpegPath), "-v", "error", "-show_entries", "stream=codec_type,width,height,avg_frame_rate:stream_tags=rotate:stream_side_data=rotation", "-of", "json", source)
	raw, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return mediaInfo{}, fmt.Errorf("ffmpeg probe media streams timed out after %s", mediaProbeTimeout)
		}
		return mediaInfo{}, ffmpegCommandError("probe media streams", err, string(raw))
	}
	return parseMediaInfo(raw)
}

func parseMediaInfo(raw []byte) (mediaInfo, error) {
	var result struct {
		Streams []struct {
			CodecType    string `json:"codec_type"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
			Tags         struct {
				Rotate string `json:"rotate"`
			} `json:"tags"`
			SideData []struct {
				Rotation *float64 `json:"rotation"`
			} `json:"side_data_list"`
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
				rotation, _ := strconv.ParseFloat(stream.Tags.Rotate, 64)
				for _, sideData := range stream.SideData {
					if sideData.Rotation != nil {
						rotation = *sideData.Rotation
						break
					}
				}
				// FFmpeg applies display rotation while decoding. Keep the raw
				// frame dimensions in step with that output or each frame is
				// interpreted with the wrong row stride.
				quarterTurns := math.Round(rotation / 90)
				if math.Abs(rotation-quarterTurns*90) < 0.01 && int(quarterTurns)%2 != 0 {
					info.width, info.height = info.height, info.width
				}
			}
		case "audio":
			info.hasAudio = true
		}
	}
	if info.hasVideo {
		if info.width <= 0 || info.height <= 0 || info.width > maxVideoDimension || info.height > maxVideoDimension || int64(info.width)*int64(info.height) > maxVideoPixels {
			return mediaInfo{}, fmt.Errorf("video dimensions %dx%d exceed the supported decode limit", info.width, info.height)
		}
		if info.fps > maxVideoFrameRate {
			return mediaInfo{}, fmt.Errorf("video frame rate %.2f exceeds the supported %.0f fps limit", info.fps, float64(maxVideoFrameRate))
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
