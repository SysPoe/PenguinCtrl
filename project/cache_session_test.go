package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCacheMaintainerOwnsSessionProtectionLifecycle(t *testing.T) {
	setTestCache(t)
	content := []byte("active session media")
	asset := testAsset(content)
	manifest := Manifest{
		Format:  Format,
		Version: Version,
		Show: show.Show{Cues: []show.Cue{{
			Type: show.CueTypeSound,
			Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: asset.Path}}},
		}}},
		Assets: []Asset{asset},
	}
	archivePath := writeTestArchive(t, manifest, []testZipEntry{{name: asset.Path, body: content}})

	var quota uint64
	maintainer := &CacheMaintainer{
		active:   func() bool { return false },
		limits:   func() (uint64, uint64) { return quota, 0 },
		sessions: make(map[*ProjectSession][]string),
	}
	session, err := maintainer.OpenSession(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	protected := session.ProtectedPaths()
	if len(protected) != 1 {
		t.Fatalf("session protected paths = %v; want one extraction root", protected)
	}
	activeObject, err := inspectCacheObject(protected[0])
	if err != nil {
		t.Fatal(err)
	}
	quota = activeObject.size

	cache, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(cache.Transcoded, "stale-object")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "media.webm"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := maintainer.MaintainNow(); err != nil {
		t.Fatalf("maintain protected session: %v", err)
	}
	if _, err := os.Stat(protected[0]); err != nil {
		t.Fatalf("active session cache was evicted: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale cache object still exists: %v", err)
	}

	session.Close()
	session.Close()
	quota = 0
	if err := maintainer.MaintainNow(); err != nil {
		t.Fatalf("maintain after session close: %v", err)
	}
	if _, err := os.Stat(protected[0]); !os.IsNotExist(err) {
		t.Fatalf("closed session cache still exists: %v", err)
	}
}

func TestCacheMaintainerSkipsMaintenanceWhilePlaybackIsActive(t *testing.T) {
	setTestCache(t)
	cache, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(cache.Transcoded, "active-playback")
	if err := os.MkdirAll(object, 0o755); err != nil {
		t.Fatal(err)
	}

	maintainer := &CacheMaintainer{
		active:   func() bool { return true },
		limits:   func() (uint64, uint64) { return 0, 0 },
		sessions: make(map[*ProjectSession][]string),
	}
	if err := maintainer.MaintainNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(object); err != nil {
		t.Fatalf("active playback cache was evicted: %v", err)
	}
}

func TestCacheMaintainerPreservesCompatibilityProtection(t *testing.T) {
	setTestCache(t)
	cache, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(cache.Shows, "legacy-protected")
	stale := filepath.Join(cache.Transcoded, "legacy-stale")
	for _, object := range []string{protected, stale} {
		if err := os.MkdirAll(object, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(object, "media"), []byte("same-size"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	protectedObject, err := inspectCacheObject(protected)
	if err != nil {
		t.Fatal(err)
	}
	maintainer := &CacheMaintainer{
		active:    func() bool { return false },
		protected: func() []string { return []string{filepath.Join(protected, "media")} },
		limits:    func() (uint64, uint64) { return protectedObject.size, 0 },
	}
	if err := maintainer.MaintainNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(protected); err != nil {
		t.Fatalf("compatibility-protected cache was evicted: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("unprotected cache still exists: %v", err)
	}
}
