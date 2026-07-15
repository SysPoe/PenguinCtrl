package media

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

func TestImageSessionLoadsStillWithoutWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "still.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	still := image.NewRGBA(image.Rect(0, 0, 1, 1))
	still.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	if err := png.Encode(file, still); err != nil {
		_ = file.Close()
		t.Fatalf("encode image: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}

	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	backend := NewFFmpegBackend(settings, nil)
	defer backend.Close()
	session, err := backend.Open(PlaybackRequest{Instance: playback.Instance{MediaType: playback.MediaTypeImage, Source: path}, RequestedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Preload(context.Background()); err != nil {
		t.Fatalf("preload image session: %v", err)
	}
	clock := NewPlaybackClock(0)
	if err := session.Start(clock); err != nil {
		t.Fatalf("start image session: %v", err)
	}
	if session.Frame(0) == nil {
		t.Fatal("image session did not retain its frame")
	}
}
