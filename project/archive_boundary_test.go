package project

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestPortableArchiveExtractionHydrationRoundTrip(t *testing.T) {
	directory := t.TempDir()
	fixtures := []struct {
		name    string
		content string
		cue     func() show.Cue
		setFile func(*show.Cue, string)
	}{
		{"sound.opus", "audio bytes", show.NewSoundCue, func(cue *show.Cue, path string) { cue.Play.Sound.File = path }},
		{"video.mp4", "video bytes", show.NewVideoCue, func(cue *show.Cue, path string) { cue.Play.Video.File = path }},
		{"image.webp", "image bytes", show.NewImageCue, func(cue *show.Cue, path string) { cue.Play.Image.File = path }},
	}
	current := show.Show{Title: "portable"}
	for index, fixture := range fixtures {
		path := filepath.Join(directory, fixture.name)
		if err := os.WriteFile(path, []byte(fixture.content), 0o644); err != nil {
			t.Fatal(err)
		}
		cue := fixture.cue()
		cue.CueNumber = string(rune('1' + index))
		fixture.setFile(&cue, path)
		current.Cues = append(current.Cues, cue)
	}

	var first bytes.Buffer
	firstManifest, err := Save(&first, current, filepath.Join(directory, "missing-ffmpeg"))
	if err != nil {
		t.Fatal(err)
	}
	assertPortableCuePaths(t, firstManifest.Show)
	archivePath := filepath.Join(directory, "portable.cusus")
	if err := os.WriteFile(archivePath, first.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	setTestCache(t)

	extracted, err := Extract(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := currentCacheLayout()
	if err != nil {
		t.Fatal(err)
	}
	if relative, err := filepath.Rel(cache.Shows, extracted.root); err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		t.Fatalf("extraction root %q is outside shows cache %q", extracted.root, cache.Shows)
	}
	assertPortableCuePaths(t, extracted.Manifest.Show)
	portableBefore := mediaCuePaths(extracted.Manifest.Show)
	runtimeShow := extracted.HydrateShow()
	for index, path := range mediaCuePaths(runtimeShow) {
		if !filepath.IsAbs(path) {
			t.Fatalf("runtime cue %d path = %q, want absolute cache path", index, path)
		}
		content, err := os.ReadFile(path)
		if err != nil || string(content) != fixtures[index].content {
			t.Fatalf("runtime cue %d content = %q, %v", index, content, err)
		}
	}
	if got := mediaCuePaths(extracted.Manifest.Show); !equalStrings(got, portableBefore) {
		t.Fatalf("hydration mutated portable manifest paths: got %v, want %v", got, portableBefore)
	}

	var second bytes.Buffer
	secondManifest, err := Save(&second, runtimeShow, filepath.Join(directory, "missing-ffmpeg"))
	if err != nil {
		t.Fatal(err)
	}
	assertPortableCuePaths(t, secondManifest.Show)
	archivedManifest := readManifestFromArchive(t, second.Bytes())
	assertPortableCuePaths(t, archivedManifest.Show)
	for _, runtimePath := range mediaCuePaths(runtimeShow) {
		if bytes.Contains(second.Bytes(), []byte(runtimePath)) {
			t.Fatalf("archive contains machine-local cache path %q", runtimePath)
		}
	}
}

func assertPortableCuePaths(t *testing.T, current show.Show) {
	t.Helper()
	for index, path := range mediaCuePaths(current) {
		if filepath.IsAbs(path) || !strings.HasPrefix(filepath.ToSlash(path), "media/") {
			t.Fatalf("cue %d path = %q, want archive-relative media path", index, path)
		}
	}
}

func mediaCuePaths(current show.Show) []string {
	result := make([]string, 0, len(current.Cues))
	for _, cue := range current.Cues {
		switch cue.Type {
		case show.CueTypeSound:
			result = append(result, cue.Play.Sound.File)
		case show.CueTypeVideo:
			result = append(result, cue.Play.Video.File)
		case show.CueTypeImage:
			result = append(result, cue.Play.Image.File)
		}
	}
	return result
}

func readManifestFromArchive(t *testing.T, raw []byte) Manifest {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		if entry.Name != "manifest.json" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		encoded, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		var manifest Manifest
		if err := json.Unmarshal(encoded, &manifest); err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	t.Fatal("archive has no manifest.json")
	return Manifest{}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
