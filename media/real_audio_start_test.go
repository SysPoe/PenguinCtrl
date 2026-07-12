package media

import (
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
	player := NewPlayer(instance, settings, audio, new(app.Window), func(string) {}, func(int64) {}, func(error) {})
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
	player := NewPlayer(instance, settings, audio, new(app.Window), func(string) {}, func(int64) {}, func(error) {})
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
