package media

import (
	"math"
	"testing"
	"time"

	"github.com/syspoe/cusus/playback"
)

func TestVideoVolumeDoesNotChangeVisualOpacity(t *testing.T) {
	player := &Player{
		instance:           playback.Instance{MediaType: "video"},
		playerSessionState: playerSessionState{volumeDB: -40},
	}

	if got := player.visualOpacity(time.Second); got != 1 {
		t.Fatalf("visual opacity = %v, want 1 at -40 dB", got)
	}
}

func TestVisualOpacityFadesOutIndependentlyOfAudioVolume(t *testing.T) {
	player := &Player{
		instance:           playback.Instance{MediaType: "video"},
		playerSessionState: playerSessionState{volumeDB: -12},
		playerPresentationState: playerPresentationState{
			visualFadeAt: time.Now().Add(-500 * time.Millisecond), visualFadeFor: time.Second,
		},
	}

	want := srgbOpacity(0.5)
	if got := player.visualOpacity(time.Second); math.Abs(float64(got)-want) > 0.1 {
		t.Fatalf("visual opacity = %v, want approximately %v halfway through replacement fade", got, want)
	}
}

func TestImageOpacityUsesLinearLightFade(t *testing.T) {
	player := &Player{
		instance:           playback.Instance{MediaType: "image", FadeInMs: 2000},
		playerSessionState: playerSessionState{volumeDB: -40},
	}

	want := srgbOpacity(0.5)
	if got := player.visualOpacity(time.Second); math.Abs(float64(got)-want) > 0.0001 {
		t.Fatalf("visual opacity = %v, want %v halfway through linear-light fade", got, want)
	}
}

func TestSRGBOpacityPreservesFadeEndpoints(t *testing.T) {
	if got := srgbOpacity(0); got != 0 {
		t.Fatalf("opacity at fade start = %v, want 0", got)
	}
	if got := srgbOpacity(1); got != 1 {
		t.Fatalf("opacity at fade end = %v, want 1", got)
	}
}
