package media

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/playback"
)

func TestLoadImageAllowsNilWindow(t *testing.T) {
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

	player := &Player{instance: playback.Instance{MediaType: "image", Source: path}}
	if err := player.loadImage(); err != nil {
		t.Fatalf("load image without window: %v", err)
	}
	if player.frame == nil {
		t.Fatal("image frame was not retained")
	}
}
