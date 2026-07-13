package media

import (
	"context"
	"testing"
	"time"
)

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
