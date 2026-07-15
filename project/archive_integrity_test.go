package project

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestLoadVerifiesAndPublishesCompleteArchive(t *testing.T) {
	content := []byte("packaged media")
	asset := testAsset(content)
	manifest := Manifest{
		Format: Format, Version: Version, Assets: []Asset{asset},
		Show: show.Show{Cues: []show.Cue{{Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: asset.Path}}}}}},
	}
	path := writeTestArchive(t, manifest, []testZipEntry{{name: asset.Path, body: content}})
	setTestCache(t)

	loaded, files, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(files) != 1 || len(loaded.Show.Cues) != 1 {
		t.Fatalf("loaded assets/cues = %d/%d", len(files), len(loaded.Show.Cues))
	}
	resolved := loaded.Show.Cues[0].Play.Sound.File
	if resolved != files[0].Source {
		t.Fatalf("resolved cue path = %q, file source = %q", resolved, files[0].Source)
	}
	if got, err := os.ReadFile(resolved); err != nil || string(got) != string(content) {
		t.Fatalf("published media = %q, %v", got, err)
	}
}

func TestLoadRejectsMissingAsset(t *testing.T) {
	asset := testAsset([]byte("missing"))
	path := writeTestArchive(t, Manifest{Format: Format, Version: Version, Assets: []Asset{asset}}, nil)
	setTestCache(t)
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "missing from archive") {
		t.Fatalf("Load() error = %v, want missing asset", err)
	}
}

func TestLoadRejectsAssetHashMismatch(t *testing.T) {
	asset := testAsset([]byte("expected"))
	actual := []byte("tampered")
	asset.Size = int64(len(actual))
	path := writeTestArchive(t, Manifest{Format: Format, Version: Version, Assets: []Asset{asset}}, []testZipEntry{{name: asset.Path, body: actual}})
	setTestCache(t)
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "SHA-256 verification") {
		t.Fatalf("Load() error = %v, want hash mismatch", err)
	}
}

func TestLoadRejectsDuplicateArchiveEntry(t *testing.T) {
	asset := testAsset([]byte("duplicate"))
	path := writeTestArchive(t, Manifest{Format: Format, Version: Version, Assets: []Asset{asset}}, []testZipEntry{
		{name: asset.Path, body: []byte("duplicate")},
		{name: asset.Path, body: []byte("duplicate")},
	})
	setTestCache(t)
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate archive entry") {
		t.Fatalf("Load() error = %v, want duplicate entry", err)
	}
}

func TestLoadRejectsCueReferenceOutsideManifest(t *testing.T) {
	manifest := Manifest{
		Format: Format, Version: Version,
		Show: show.Show{Cues: []show.Cue{{CueNumber: "12", Type: show.CueTypeVideo, Play: show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: "media/not-declared.webm"}}}}}},
	}
	path := writeTestArchive(t, manifest, nil)
	setTestCache(t)
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "references undeclared archive asset") {
		t.Fatalf("Load() error = %v, want undeclared asset", err)
	}
}

type testZipEntry struct {
	name string
	body []byte
}

func testAsset(content []byte) Asset {
	contentHash := fmt.Sprintf("%x", sha256.Sum256(content))
	return Asset{
		ID: "asset-audio", Name: "sound.opus", Kind: "audio", Path: "media/sound.opus",
		SourceSHA256: strings.Repeat("1", sha256.Size*2), ContentSHA256: contentHash,
		Format: "opus", Size: int64(len(content)),
	}
}

func writeTestArchive(t *testing.T, manifest Manifest, entries []testZipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "show.cusus")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	raw, err := json.Marshal(manifest)
	if err == nil {
		var entryWriter interface{ Write([]byte) (int, error) }
		entryWriter, err = writer.Create("manifest.json")
		if err == nil {
			_, err = entryWriter.Write(raw)
		}
	}
	for _, entry := range entries {
		if err != nil {
			break
		}
		var entryWriter interface{ Write([]byte) (int, error) }
		entryWriter, err = writer.Create(entry.name)
		if err == nil {
			_, err = entryWriter.Write(entry.body)
		}
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func setTestCache(t *testing.T) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
}
