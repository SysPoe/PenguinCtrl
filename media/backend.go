package media

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os/exec"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
	_ "golang.org/x/image/webp"
)

const (
	decodedFrameBuffer        = 2
	mediaProbeTimeout         = 10 * time.Second
	maxVideoDimension         = 8192
	maxVideoPixels            = 7680 * 4320
	maxVideoFrameRate         = 240
	maxVideoBitRate           = 500_000_000
	maxAudioBitRate           = 20_000_000
	maxDecoderSessions        = 12
	maxVideoBufferBytes int64 = 512 << 20
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
	AVDrift        time.Duration
	Error          string
}

type PlaybackRequest struct {
	Instance    playback.Instance
	Position    time.Duration
	RequestedAt time.Time
}

// PlaybackBackend is the decoder seam used by Player. Implementations preload
// bounded media data, then start all device output from a shared clock.
// TODO(macro): Seam only covers Open; Prewarm/Close/admission live only on
// *FFmpegBackend and are reached by Manager via concrete type. Either widen the
// interface for the shared runtime lifecycle or keep a separate RuntimeBackend
// so Player's seam and Manager's ownership seam are intentional, not accidental.
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
	active     map[*ffmpegSession]struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
	admission  *decoderAdmission
}

type warmSession struct {
	session PlaybackSession
	warmed  time.Time
}

type decoderAdmission struct {
	mu       sync.Mutex
	sessions int
	bytes    int64
	notify   chan struct{}
}

func newDecoderAdmission() *decoderAdmission {
	return &decoderAdmission{notify: make(chan struct{}, 1)}
}

func (a *decoderAdmission) acquire(ctx, sessionCtx context.Context, bytes int64) bool {
	for {
		a.mu.Lock()
		if a.sessions < maxDecoderSessions && a.bytes+bytes <= maxVideoBufferBytes {
			a.sessions++
			a.bytes += bytes
			a.mu.Unlock()
			return true
		}
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-sessionCtx.Done():
			return false
		case <-a.notify:
		}
	}
}

func (a *decoderAdmission) release(bytes int64) {
	a.mu.Lock()
	a.sessions = max(0, a.sessions-1)
	a.bytes = max(int64(0), a.bytes-bytes)
	a.mu.Unlock()
	select {
	case a.notify <- struct{}{}:
	default:
	}
}

func NewFFmpegBackend(settings *config.Store, audio *AudioSystem) *FFmpegBackend {
	ctx, cancel := context.WithCancel(context.Background())
	return &FFmpegBackend{settings: settings, audio: audio, warm: map[string]warmSession{}, warming: map[string]struct{}{}, warmFailed: map[string]time.Time{}, active: map[*ffmpegSession]struct{}{}, ctx: ctx, cancel: cancel, admission: newDecoderAdmission()}
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
		if warmed.session.State() != LoadReady {
			warmed.session.Close()
			return b.openFresh(request)
		}
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
	session := &ffmpegSession{
		backend: b, request: request, path: path, state: LoadIdle,
		frames: make(chan decodedFrame, decodedFrameBuffer), done: make(chan struct{}),
		ctx: ctx, cancel: cancel,
	}
	b.warmMu.Lock()
	if b.closed {
		b.warmMu.Unlock()
		cancel()
		return nil, errors.New("media backend is closed")
	}
	b.active[session] = struct{}{}
	b.warmMu.Unlock()
	return session, nil
}

func (b *FFmpegBackend) release(session *ffmpegSession) {
	if b == nil || session == nil {
		return
	}
	b.warmMu.Lock()
	delete(b.active, session)
	b.warmMu.Unlock()
}

// Prewarm starts bounded decoder and audio prebuffer work for imminent cues.
// A later Open atomically claims the ready session; stale entries are closed.
func (b *FFmpegBackend) Prewarm(requests []PlaybackRequest) {
	for _, request := range requests {
		// TODO(micro): Remove this pre-Go-1.22 loop-variable copy; each iteration now has its own request variable.
		// TODO(micro): request := request is a no-op under Go 1.22+ loop semantics (module is go 1.26.4); drop the rebinding.
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
		// TODO(micro): 30s failure cooldown, 15s preload timeout, and warm cache size 4 are magic; name constants (and share the 15s timeout with ffmpegSession.Preload).
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
			// TODO(micro): Rewrite this mutually exclusive err/closed chain as a switch so each state transition is explicit.
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
	sessions := make([]*ffmpegSession, 0, len(b.active))
	for session := range b.active {
		sessions = append(sessions, session)
	}
	warmSessions := make([]PlaybackSession, 0, len(b.warm))
	for _, warmed := range b.warm {
		if session, ok := warmed.session.(*ffmpegSession); ok {
			if _, active := b.active[session]; active {
				continue
			}
		}
		warmSessions = append(warmSessions, warmed.session)
	}
	b.warm = map[string]warmSession{}
	b.active = map[*ffmpegSession]struct{}{}
	b.warmMu.Unlock()
	for _, session := range sessions {
		session.Close()
	}
	for _, session := range warmSessions {
		session.Close()
	}
}

type decodedFrame struct {
	image *image.RGBA
	pts   time.Duration
}

// TODO(macro): ffmpegSession is a dual A/V process owner — separate FFmpeg
// video/audio cmds, frame queue, devicePlayer, clock master binding, and full
// audio-device recovery all live on one type. Split into a video demux/decode
// pipeline and an audio pipeline that share only request/lifecycle/clock so
// recovery and metrics do not drag video state through audio endpoint failures.
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

	frames        chan decodedFrame
	frameMu       sync.Mutex
	current       *decodedFrame
	pending       *decodedFrame
	framePool     sync.Pool
	admitted      bool
	admittedBytes int64
}
