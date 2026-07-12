package media

import (
	"image"
	"strings"
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

func TestParseMediaInfoUsesAutorotatedFrameDimensions(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","width":1080,"height":1920,"avg_frame_rate":"30000/1001","side_data_list":[{"side_data_type":"Mastering display metadata"},{"rotation":-90}]},{"codec_type":"audio"}]}`
	info, err := parseMediaInfo([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !info.hasVideo || !info.hasAudio {
		t.Fatalf("streams not detected: %#v", info)
	}
	if info.width != 1920 || info.height != 1080 {
		t.Fatalf("decoded dimensions = %dx%d, want 1920x1080", info.width, info.height)
	}
	if info.fps < 29.9 || info.fps > 30 {
		t.Fatalf("fps = %v, want approximately 29.97", info.fps)
	}
}

func TestParseMediaInfoUsesLegacyRotateTag(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","width":1920,"height":1080,"avg_frame_rate":"25/1","tags":{"rotate":"90"}}]}`
	info, err := parseMediaInfo([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if info.width != 1080 || info.height != 1920 {
		t.Fatalf("decoded dimensions = %dx%d, want 1080x1920", info.width, info.height)
	}
}

func TestParseMediaInfoRejectsInvalidJSON(t *testing.T) {
	_, err := parseMediaInfo([]byte(`{"streams":`))
	if err == nil || !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("error = %v, want JSON parse failure", err)
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
