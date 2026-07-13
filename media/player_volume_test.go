package media

import (
	"math"
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

func TestVisualOpacityFadesOutIndependentlyOfAudioVolume(t *testing.T) {
	player := &Player{
		instance:      playback.Instance{MediaType: "video"},
		volumeDB:      -12,
		visualFadeAt:  time.Now().Add(-500 * time.Millisecond),
		visualFadeFor: time.Second,
	}

	if got := player.visualOpacity(time.Second); math.Abs(float64(got)-0.5) > 0.1 {
		t.Fatalf("visual opacity = %v, want approximately 0.5 halfway through replacement fade", got)
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
