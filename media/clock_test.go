package media

import (
	"testing"
	"time"
)

func TestPlaybackClockPauseResumeAndSeek(t *testing.T) {
	now := time.Unix(100, 0)
	clock := newPlaybackClock(2*time.Second, func() time.Time { return now })
	clock.Start()
	now = now.Add(750 * time.Millisecond)
	if got := clock.Position(); got != 2750*time.Millisecond {
		t.Fatalf("running position = %v", got)
	}
	clock.Pause()
	now = now.Add(time.Hour)
	if got := clock.Position(); got != 2750*time.Millisecond {
		t.Fatalf("paused position changed to %v", got)
	}
	clock.Seek(5 * time.Second)
	clock.Start()
	now = now.Add(125 * time.Millisecond)
	if got := clock.Position(); got != 5125*time.Millisecond {
		t.Fatalf("resumed position = %v", got)
	}
}
