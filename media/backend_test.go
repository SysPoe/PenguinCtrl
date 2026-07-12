package media

import (
	"image"
	"testing"
	"time"
)

func TestParseFrameRate(t *testing.T) {
	for input, want := range map[string]float64{"30000/1001": 29.97002997002997, "25/1": 25, "60": 60, "0/0": 0} {
		if got := parseFrameRate(input); got != want {
			t.Errorf("parseFrameRate(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestFrameSelectionDropsStaleFramesAndKeepsNewestDue(t *testing.T) {
	session := &ffmpegSession{
		state:  LoadPlaying,
		info:   mediaInfo{fps: 25},
		frames: make(chan decodedFrame, 3),
	}
	for i := range 3 {
		session.frames <- decodedFrame{
			image: image.NewRGBA(image.Rect(0, 0, 1, 1)),
			pts:   time.Duration(i) * 40 * time.Millisecond,
		}
	}
	got := session.Frame(90 * time.Millisecond)
	if got == nil || session.current == nil || session.current.pts != 80*time.Millisecond {
		t.Fatalf("selected frame = %#v", session.current)
	}
	metrics := session.Metrics()
	if metrics.DroppedFrames != 2 {
		t.Fatalf("dropped frames = %d, want 2", metrics.DroppedFrames)
	}
}

func TestMediaInfoFrameIntervalDefaultsSafely(t *testing.T) {
	if got := (mediaInfo{}).frameInterval(); got != time.Second/30 {
		t.Fatalf("default interval = %v", got)
	}
}
