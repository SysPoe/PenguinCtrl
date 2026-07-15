package playback

import (
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestReduceMediaControlOwnsPauseResumeAndSeekState(t *testing.T) {
	now := time.Unix(100, 0)
	instance := &liveInstance{
		Instance:            Instance{ID: "media", BackendStarted: true, ClipStartMs: 100, ClipEndMs: 1000, PositionMs: 500},
		positionAt:          now.Add(-time.Second),
		endScheduled:        true,
		lifecycleGeneration: 2,
	}
	reschedule := []string{}
	reduceMediaControl(instance, &show.MediaControlPlay{Action: show.MediaControlPause}, now, &reschedule)
	if !instance.Paused || !instance.positionAt.IsZero() || instance.endScheduled || instance.lifecycleGeneration != 3 {
		t.Fatalf("pause state = %#v", instance)
	}

	seek := int64(5000)
	reduceMediaControl(instance, &show.MediaControlPlay{Action: show.MediaControlSeek, SeekToMs: &seek}, now, &reschedule)
	if instance.PositionMs != instance.ClipEndMs || !instance.positionAt.IsZero() {
		t.Fatalf("paused seek state = %#v", instance)
	}

	reduceMediaControl(instance, &show.MediaControlPlay{Action: show.MediaControlResume}, now, &reschedule)
	if instance.Paused || !instance.positionAt.Equal(now) || len(reschedule) != 1 || reschedule[0] != instance.ID {
		t.Fatalf("resume state = %#v reschedule %#v", instance, reschedule)
	}
}
