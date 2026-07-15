package media

import (
	"image"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
)

func TestFrameSelectionDropsStaleFramesAndKeepsNewestDue(t *testing.T) {
	session := &ffmpegSession{
		state:  LoadPlaying,
		info:   mediaInfo{FPS: 25},
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
	if got := (mediaInfo{}).FrameInterval(); got != time.Second/30 {
		t.Fatalf("default interval = %v", got)
	}
}

func TestPlaybackNeedsRefreshOnlyForActiveDecoderStates(t *testing.T) {
	for _, state := range []LoadState{LoadLoading, LoadReady, LoadPlaying, LoadBuffering} {
		if !playbackNeedsRefresh(state) {
			t.Errorf("state %q should refresh", state)
		}
	}
	for _, state := range []LoadState{LoadIdle, LoadPaused, LoadEnded, LoadFailed, LoadClosed} {
		if playbackNeedsRefresh(state) {
			t.Errorf("state %q should not refresh", state)
		}
	}
}

func TestDecodeSizeCapsFramesToStageResolution(t *testing.T) {
	output := config.VideoOutput{ResolutionWidth: 1920, ResolutionHeight: 1080}
	for _, test := range []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{name: "4k landscape", width: 3840, height: 2160, wantW: 1920, wantH: 1080},
		{name: "4k portrait", width: 2160, height: 3840, wantW: 608, wantH: 1080},
		{name: "already smaller", width: 1280, height: 720, wantW: 1280, wantH: 720},
	} {
		t.Run(test.name, func(t *testing.T) {
			width, height := decodeSize(test.width, test.height, output)
			if width != test.wantW || height != test.wantH {
				t.Fatalf("decode size = %dx%d, want %dx%d", width, height, test.wantW, test.wantH)
			}
		})
	}
}
