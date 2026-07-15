package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestProjectSessionOwnsPortableAndRuntimeMediaBoundaries(t *testing.T) {
	content := []byte("session media")
	asset := testAsset(content)
	manifest := Manifest{
		Format:  Format,
		Version: Version,
		Show: show.Show{Cues: []show.Cue{{
			CueNumber: "1",
			Type:      show.CueTypeSound,
			Play:      show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: asset.Path}}},
		}}},
		Assets: []Asset{asset},
	}
	path := writeTestArchive(t, manifest, []testZipEntry{{name: asset.Path, body: content}})
	setTestCache(t)

	session, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := session.ArchivePath(); got != filepath.Clean(path) {
		t.Fatalf("ArchivePath() = %q; want %q", got, filepath.Clean(path))
	}

	portable := session.PortableManifest()
	if got := portable.Show.Cues[0].Play.Sound.File; got != asset.Path {
		t.Fatalf("portable cue path = %q; want %q", got, asset.Path)
	}
	files := session.MediaFiles("audio")
	if len(files) != 1 {
		t.Fatalf("MediaFiles(audio) = %d entries; want 1", len(files))
	}
	runtimeShow := session.RuntimeShow()
	runtimePath := runtimeShow.Cues[0].Play.Sound.File
	if runtimePath != files[0].Source || !filepath.IsAbs(runtimePath) {
		t.Fatalf("runtime cue/library paths = %q/%q; want matching absolute paths", runtimePath, files[0].Source)
	}
	if got, err := os.ReadFile(runtimePath); err != nil || string(got) != string(content) {
		t.Fatalf("runtime media = %q, %v", got, err)
	}

	protected := session.ProtectedPaths()
	if len(protected) != 1 || !cacheObjectProtected(protected[0], []string{runtimePath}) {
		t.Fatalf("ProtectedPaths() = %v; want cache object containing %q", protected, runtimePath)
	}
}

func TestProjectSessionSnapshotsDoNotMutateOwnedState(t *testing.T) {
	asset := testAsset([]byte("snapshot media"))
	archive := ExtractedArchive{
		Manifest: Manifest{
			Format:  Format,
			Version: Version,
			Show: show.Show{Cues: []show.Cue{{
				Type: show.CueTypeSound,
				Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: asset.Path}}},
			}}},
			Assets:     []Asset{asset},
			Extensions: map[string]json.RawMessage{"test": json.RawMessage(`{"enabled":true}`)},
		},
		Files: []File{{Name: asset.Name, Source: filepath.Join("cache", asset.Path), Hash: asset.SourceSHA256, Kind: asset.Kind}},
		root:  filepath.Join("cache", "show-object"),
	}
	session := newProjectSession("show.cusus", archive)

	portable := session.PortableManifest()
	portable.Show.Cues[0].Play.Sound.File = "changed"
	portable.Assets[0].Path = "changed"
	portable.Extensions["test"][0] = 'x'
	runtimeShow := session.RuntimeShow()
	runtimeShow.Cues[0].Play.Sound.File = "changed"
	files := session.MediaFiles("")
	files[0].Source = "changed"
	protected := session.ProtectedPaths()
	protected[0] = "changed"

	again := session.PortableManifest()
	if got := again.Show.Cues[0].Play.Sound.File; got != asset.Path {
		t.Fatalf("portable snapshot mutated owned cue path to %q", got)
	}
	if got := again.Assets[0].Path; got != asset.Path {
		t.Fatalf("portable snapshot mutated owned asset path to %q", got)
	}
	if got := string(again.Extensions["test"]); !strings.Contains(got, "enabled") {
		t.Fatalf("portable snapshot mutated owned extension to %q", got)
	}
	if got := session.RuntimeShow().Cues[0].Play.Sound.File; got == "changed" {
		t.Fatal("runtime snapshot mutated owned show")
	}
	if got := session.MediaFiles("")[0].Source; got == "changed" {
		t.Fatal("library snapshot mutated owned media")
	}
	if got := session.ProtectedPaths()[0]; got == "changed" {
		t.Fatal("protection snapshot mutated owned cache path")
	}
}
