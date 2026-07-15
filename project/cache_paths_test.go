package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCacheLayoutOwnsAllProjectCacheNamespaces(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-cache")
	layout := cacheLayoutAt(userRoot)
	wantRoot := filepath.Join(userRoot, cacheApplicationDirectory)
	if layout.Root != wantRoot || layout.Shows != filepath.Join(wantRoot, cacheShowsDirectory) || layout.Transcoded != filepath.Join(wantRoot, cacheTranscodedDirectory) {
		t.Fatalf("cache layout = %#v", layout)
	}
	if got, want := layout.objectRoots(), []string{layout.Shows, layout.Transcoded}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cache object roots = %v, want %v", got, want)
	}
}

func TestTranscodeCacheHitUsesSharedLayout(t *testing.T) {
	setTestCache(t)
	layout, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	want := filepath.Join(layout.Transcoded, hash+"-audio.opus")
	if err := os.MkdirAll(layout.Transcoded, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("cached transcode"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := transcode(filepath.Join(t.TempDir(), "missing-ffmpeg"), "unused.wav", "audio", hash)
	if err != nil || got != want {
		t.Fatalf("transcode cache hit = %q, %v; want %q", got, err, want)
	}
}

func TestMaintainCacheUsesSharedLayoutRoot(t *testing.T) {
	setTestCache(t)
	layout, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(layout.Shows, "unused", "media", "clip.webm")
	if err := os.MkdirAll(filepath.Dir(object), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(object, []byte("unused cache object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MaintainCache(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.Shows, "unused")); !os.IsNotExist(err) {
		t.Fatalf("shared-layout cache object still exists: %v", err)
	}
}
