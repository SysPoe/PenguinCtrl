package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

func TestRealAudioFirstStartCompletes(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	mediaPath := filepath.Join(t.TempDir(), "silence.wav")
	if output, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "0.5", mediaPath).CombinedOutput(); err != nil {
		t.Fatalf("create test media: %v: %s", err, output)
	}
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := NewAudioSystem(settings)
	if err != nil {
		t.Skipf("audio system is unavailable: %v", err)
	}
	instance := playback.Instance{
		ID: "real-audio", CueID: show.NewCueID(), MediaType: "audio", Source: mediaPath,
		OutputID: "main", DurationMs: 500,
	}
	backend := NewFFmpegBackend(settings, audio)
	defer backend.Close()
	player := NewPlayerWithBackend(instance, settings, backend, new(app.Window), func(string) {}, func(int64) {}, func(error) {})
	done := make(chan error, 1)
	go func() { done <- player.Start() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first playback start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first playback start hung")
	}
	player.Close(false)
}

func TestRealVideoFirstStartCompletes(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	mediaPath := filepath.Join(t.TempDir(), "video.mp4")
	if output, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=320x180:r=25", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "0.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", mediaPath).CombinedOutput(); err != nil {
		t.Fatalf("create test media: %v: %s", err, output)
	}
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := NewAudioSystem(settings)
	if err != nil {
		t.Skipf("audio system is unavailable: %v", err)
	}
	instance := playback.Instance{
		ID: "real-video", CueID: show.NewCueID(), MediaType: "video", Source: mediaPath,
		OutputID: "main", DurationMs: 500,
	}
	backend := NewFFmpegBackend(settings, audio)
	defer backend.Close()
	player := NewPlayerWithBackend(instance, settings, backend, new(app.Window), func(string) {}, func(int64) {}, func(error) {})
	done := make(chan error, 1)
	go func() { done <- player.Start() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first video start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first video start hung")
	}
	player.Close(false)
}

func TestRealRotatedVideoDecodesWithDisplayDimensions(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.mp4")
	rotatedPath := filepath.Join(dir, "rotated.mp4")
	if output, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25", "-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p", basePath).CombinedOutput(); err != nil {
		t.Fatalf("create base video: %v: %s", err, output)
	}
	if output, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-display_rotation:v:0", "90", "-i", basePath, "-c", "copy", rotatedPath).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot create display-rotation metadata: %v: %s", err, output)
	}

	settings, err := config.Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	backend := NewFFmpegBackend(settings, nil)
	session, err := backend.Open(PlaybackRequest{Instance: playback.Instance{
		ID: "rotated-video", CueID: show.NewCueID(), MediaType: "video", Source: rotatedPath, OutputID: "main",
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Preload(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock := NewPlaybackClock(0)
	if err := session.Start(clock); err != nil {
		t.Fatal(err)
	}
	frame := session.Frame(0)
	if frame == nil {
		t.Fatal("first decoded frame is nil")
	}
	if got := frame.Bounds().Size(); got.X != 180 || got.Y != 320 {
		t.Fatalf("decoded frame = %dx%d, want display dimensions 180x320", got.X, got.Y)
	}
}
