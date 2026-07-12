package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/internal/atomicfile"
)

func TestUpdateReplacesExistingSettingsOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot()
	settings.DefaultPlayback = "42"
	if err := store.Update(settings); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().DefaultPlayback; got != "42" {
		t.Fatalf("reopened playback = %q, want 42", got)
	}
	if _, err := os.Stat(atomicfile.BackupPath(path)); err != nil {
		t.Fatalf("last-known-good backup missing: %v", err)
	}
}

func TestFailedUpdateDoesNotChangeInMemorySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	blockingParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(blockingParent, "settings.json")
	candidate := before
	candidate.DefaultPlayback = "99"
	if err := store.Update(candidate); err == nil {
		t.Fatal("update unexpectedly succeeded")
	}
	if got := store.Snapshot().DefaultPlayback; got != before.DefaultPlayback {
		t.Fatalf("failed update changed memory to %q", got)
	}
}

func TestOpenRecoversInterruptedReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := []byte(`{"ffmpegPath":"ffmpeg","defaultPlayback":"7","defaultMediaOutput":"main","variables":{},"remoteTargets":[]}`)
	if err := os.WriteFile(atomicfile.BackupPath(path), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot().DefaultPlayback; got != "7" {
		t.Fatalf("recovered playback = %q, want 7", got)
	}
}

func TestOpenFallsBackFromCorruptPrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot()
	settings.DefaultPlayback = "8"
	if err := store.Update(settings); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// The backup is the generation before playback 8, which is the default 1.
	if got := recovered.Snapshot().DefaultPlayback; got != "1" {
		t.Fatalf("backup playback = %q, want 1", got)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt primary was not preserved: %v, %v", matches, err)
	}
}
