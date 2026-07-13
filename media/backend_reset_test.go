package media

import (
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

func TestBackendCloseForceClosesActiveSessions(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("open settings: %v", err)
	}
	backend := NewFFmpegBackend(settings, nil)
	opened, err := backend.Open(PlaybackRequest{Instance: playback.Instance{Source: "not-decoded.wav"}})
	if err != nil {
		t.Fatalf("open tracked session: %v", err)
	}
	session := opened.(*ffmpegSession)

	backend.Close()

	session.mu.RLock()
	closed := session.closed
	session.mu.RUnlock()
	if !closed {
		t.Fatal("backend close left an active playback session running")
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("backend close did not signal the active session")
	}
	if _, err := backend.Open(PlaybackRequest{Instance: playback.Instance{Source: "after-reset.wav"}}); err == nil {
		t.Fatal("closed backend accepted a new playback session")
	}
}
