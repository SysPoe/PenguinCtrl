package media

import (
	"context"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/syspoe/cusus/internal/taskgroup"
	"github.com/syspoe/cusus/playback"
)

type rejectingPlaybackBackend struct{ err error }

func (b rejectingPlaybackBackend) Open(PlaybackRequest) (PlaybackSession, error) { return nil, b.err }

type recordingPlaybackBackend struct{ session *recordingPlaybackSession }

func (b recordingPlaybackBackend) Open(PlaybackRequest) (PlaybackSession, error) {
	return b.session, nil
}

type recordingPlaybackSession struct {
	mu      sync.Mutex
	volumes []float64
	done    chan struct{}
	once    sync.Once
}

func (*recordingPlaybackSession) Preload(context.Context) error   { return nil }
func (*recordingPlaybackSession) Frame(time.Duration) image.Image { return nil }
func (*recordingPlaybackSession) SetMuted(bool)                   {}
func (*recordingPlaybackSession) State() LoadState                { return LoadReady }
func (*recordingPlaybackSession) Metrics() PlaybackMetrics {
	return PlaybackMetrics{State: LoadPlaying}
}
func (s *recordingPlaybackSession) Done() <-chan struct{} { return s.done }
func (s *recordingPlaybackSession) Start(clock *PlaybackClock) error {
	clock.Start()
	return nil
}
func (s *recordingPlaybackSession) SetVolume(db float64) {
	s.mu.Lock()
	s.volumes = append(s.volumes, db)
	s.mu.Unlock()
}
func (s *recordingPlaybackSession) Close() { s.once.Do(func() { close(s.done) }) }

func (s *recordingPlaybackSession) latestVolume() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.volumes) == 0 {
		return 0
	}
	return s.volumes[len(s.volumes)-1]
}

func (s *recordingPlaybackSession) firstVolume() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.volumes[0]
}

func TestSoundFadeInAndFadeOutDriveSessionGain(t *testing.T) {
	session := &recordingPlaybackSession{done: make(chan struct{})}
	player := NewPlayerWithBackend(
		playback.Instance{MediaType: playback.MediaTypeAudio, FadeInMs: 60, LevelDB: -6, DurationMs: 1000},
		nil, recordingPlaybackBackend{session: session}, nil, func(string) {}, nil, nil,
	)
	if err := player.Start(); err != nil {
		t.Fatal(err)
	}
	if got := session.firstVolume(); got != muteFloorDB {
		t.Fatalf("initial session volume = %v dB, want mute floor %v dB", got, muteFloorDB)
	}
	waitForVolume(t, session, -6)
	player.Control(playback.Event{Control: "fade-out", FadeMs: 60})
	waitForVolume(t, session, muteFloorDB)
	player.Close(false)
}

func waitForVolume(t *testing.T, session *recordingPlaybackSession, want float64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := session.latestVolume(); got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("latest session volume = %v dB, want %v dB", session.latestVolume(), want)
}

func TestPlayerCloseCancelsAndJoinsOwnedWork(t *testing.T) {
	workers := taskgroup.NewUnbounded(context.Background(), nil)
	player := &Player{workers: workers, ctx: workers.Context()}
	started := make(chan struct{})
	exited := make(chan struct{})
	if !player.goOwned(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(exited)
	}) {
		t.Fatal("player task was rejected")
	}
	<-started
	player.Close(false)
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("player work outlived close")
	}
}

func TestPlayerControlsReportRestartFailures(t *testing.T) {
	restartErr := errors.New("decoder restart failed")
	tests := []struct {
		name    string
		prepare func(*Player)
		act     func(*Player)
	}{
		{
			name: "seek",
			act: func(player *Player) {
				position := int64(1250)
				player.Control(playback.Event{Control: "seek", PositionMs: &position})
			},
		},
		{
			name:    "resume",
			prepare: func(player *Player) { player.paused = true },
			act:     func(player *Player) { player.resume() },
		},
		{
			name:    "reveal",
			prepare: func(player *Player) { player.decodeVisible = false },
			act:     func(player *Player) { player.SetDecodeVisible(true) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failures := make(chan error, 1)
			player := NewPlayerWithBackend(
				playback.Instance{MediaType: "video"}, nil,
				rejectingPlaybackBackend{err: restartErr}, nil, nil, nil,
				func(err error) { failures <- err },
			)
			if test.prepare != nil {
				test.prepare(player)
			}
			test.act(player)
			select {
			case err := <-failures:
				if !errors.Is(err, restartErr) {
					t.Fatalf("reported error = %v, want %v", err, restartErr)
				}
			case <-time.After(time.Second):
				t.Fatal("restart failure was not reported")
			}
			player.Close(false)
		})
	}
}

func TestPlayerVolumeInvalidationAllowsNilWindow(t *testing.T) {
	player := &Player{}
	player.applyVolume(-12)
	if player.volumeDB != -12 {
		t.Fatalf("volume = %v, want -12", player.volumeDB)
	}
}
