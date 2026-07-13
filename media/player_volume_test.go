package media

import (
	"testing"
	"time"

	"github.com/syspoe/cusus/playback"
)

func TestVideoVolumeDoesNotChangeVisualOpacity(t *testing.T) {
	player := &Player{
		instance: playback.Instance{MediaType: "video"},
		volumeDB: -40,
	}

	if got := player.visualOpacity(time.Second); got != 1 {
		t.Fatalf("visual opacity = %v, want 1 at -40 dB", got)
	}
}

func TestVisualOpacityStillFollowsPictureFade(t *testing.T) {
	player := &Player{
		instance: playback.Instance{MediaType: "video", FadeInMs: 2000},
		volumeDB: -40,
	}

	if got := player.visualOpacity(time.Second); got != 0.5 {
		t.Fatalf("visual opacity = %v, want 0.5 halfway through fade", got)
	}
}
