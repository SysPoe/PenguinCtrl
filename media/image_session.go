package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

// imageSession implements the same playback lifecycle as decoded media while
// retaining one immutable frame and no external process.
type imageSession struct {
	mu          sync.RWMutex
	path        string
	requestedAt time.Time
	frame       image.Image
	state       LoadState
	metrics     PlaybackMetrics
	done        chan struct{}
	doneOnce    sync.Once
}

func newImageSession(path string, requestedAt time.Time) *imageSession {
	return &imageSession{path: path, requestedAt: requestedAt, state: LoadIdle, done: make(chan struct{})}
}

func (session *imageSession) Preload(context.Context) error {
	session.mu.Lock()
	if session.state == LoadClosed {
		session.mu.Unlock()
		return errors.New("image session is closed")
	}
	session.state = LoadLoading
	session.mu.Unlock()

	file, err := os.Open(session.path)
	if err != nil {
		return session.fail(fmt.Errorf("open image: %w", err))
	}
	defer file.Close()
	frame, format, err := image.Decode(bufio.NewReader(file))
	if err != nil {
		return session.fail(fmt.Errorf("decode image (detected format %q): %w", format, err))
	}

	session.mu.Lock()
	session.frame = frame
	session.state = LoadReady
	session.metrics.State = LoadReady
	session.metrics.LoadLatency = time.Since(session.requestedAt)
	session.mu.Unlock()
	return nil
}

func (session *imageSession) Start(clock *PlaybackClock) error {
	session.mu.Lock()
	if session.state != LoadReady {
		state := session.state
		session.mu.Unlock()
		return fmt.Errorf("image is not ready (state %s)", state)
	}
	clock.Start()
	session.state = LoadPlaying
	session.metrics.State = LoadPlaying
	session.metrics.StartLatency = time.Since(session.requestedAt)
	session.mu.Unlock()
	return nil
}

func (session *imageSession) Frame(time.Duration) image.Image {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.frame
}

func (*imageSession) SetVolume(float64) {}
func (*imageSession) SetMuted(bool)     {}

func (session *imageSession) State() LoadState {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.state
}

func (session *imageSession) Metrics() PlaybackMetrics {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.metrics
}

func (session *imageSession) Done() <-chan struct{} { return session.done }

func (session *imageSession) Close() {
	session.mu.Lock()
	session.state = LoadClosed
	session.metrics.State = LoadClosed
	session.frame = nil
	session.mu.Unlock()
	session.doneOnce.Do(func() { close(session.done) })
}

func (session *imageSession) fail(err error) error {
	session.mu.Lock()
	session.state = LoadFailed
	session.metrics.State = LoadFailed
	session.metrics.Error = err.Error()
	session.mu.Unlock()
	return err
}
