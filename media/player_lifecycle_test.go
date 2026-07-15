package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syspoe/cusus/playback"
)

type rejectingPlaybackBackend struct{ err error }

func (b rejectingPlaybackBackend) Open(PlaybackRequest) (PlaybackSession, error) { return nil, b.err }

func TestPlayerCloseCancelsAndJoinsOwnedWork(t *testing.T) {
	player := &Player{}
	player.ctx, player.cancel = context.WithCancel(context.Background())
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
