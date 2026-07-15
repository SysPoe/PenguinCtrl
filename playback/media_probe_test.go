package playback

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func TestMediaProbesAreSerialized(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	if !engine.mediaCatalog.acquireProbe() {
		t.Fatal("first probe slot was unavailable")
	}
	acquired := make(chan struct{})
	go func() {
		if engine.mediaCatalog.acquireProbe() {
			close(acquired)
			engine.mediaCatalog.releaseProbe()
		}
	}()
	select {
	case <-acquired:
		t.Fatal("second media probe ran concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	engine.mediaCatalog.releaseProbe()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("queued media probe did not start after slot release")
	}
	engine.cancel()
}
