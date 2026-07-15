package project

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/internal/atomicfile"
	"github.com/syspoe/cusus/show"
)

func TestSaveAtPathRetainsPreviousArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "show.cusus")
	ffmpeg := filepath.Join(t.TempDir(), "missing-ffmpeg")
	if _, err := SaveAtPathWithProgress(path, show.Show{Title: "First"}, ffmpeg, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveAtPathWithProgress(path, show.Show{Title: "Second"}, ffmpeg, nil); err != nil {
		t.Fatal(err)
	}
	current, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	previous, _, err := Load(atomicfile.BackupPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if current.Show.Title != "Second" || previous.Show.Title != "First" {
		t.Fatalf("current/previous titles = %q/%q", current.Show.Title, previous.Show.Title)
	}
}

func TestSaveBundlesSupportedVideoWithoutTranscoding(t *testing.T) {
	source := filepath.Join(t.TempDir(), "clip.mp4")
	content := []byte("representative video bytes")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}
	current := show.Show{Cues: []show.Cue{{
		CueNumber: "1",
		Type:      show.CueTypeVideo,
		Play:      show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: source}}},
	}}}
	var archive bytes.Buffer
	var updates []SaveProgress
	manifest, err := SaveWithProgress(&archive, current, filepath.Join(t.TempDir(), "missing-ffmpeg"), func(progress SaveProgress) {
		updates = append(updates, progress)
	})
	if err != nil {
		t.Fatalf("SaveWithProgress() error = %v", err)
	}
	if len(manifest.Assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(manifest.Assets))
	}
	asset := manifest.Assets[0]
	if asset.Format != "mp4" || filepath.Ext(asset.Path) != ".mp4" {
		t.Fatalf("video asset format/path = %q/%q", asset.Format, asset.Path)
	}
	if len(updates) != 1 || updates[0].Current != 1 || updates[0].Total != 1 || updates[0].Kind != "video" {
		t.Fatalf("save progress = %+v", updates)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		if entry.Name != asset.Path {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		bundled, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bundled, content) {
			t.Fatalf("bundled video = %q, want original bytes", bundled)
		}
		return
	}
	t.Fatalf("archive has no %q entry", asset.Path)
}

func TestSaveWithProgressDoesNotMutateShowSnapshot(t *testing.T) {
	source := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(source, []byte("representative video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := show.Show{Cues: []show.Cue{{
		CueNumber: "1",
		Type:      show.CueTypeVideo,
		Play:      show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: source}}},
	}}}
	var archive bytes.Buffer
	if _, err := SaveWithProgress(&archive, current, filepath.Join(t.TempDir(), "missing-ffmpeg"), nil); err != nil {
		t.Fatalf("SaveWithProgress() error = %v", err)
	}
	if got := current.Cues[0].Play.Video.File; got != source {
		t.Fatalf("caller media path = %q, want %q", got, source)
	}
	if got := current.Cues[0].ID; got != (show.CueID{}) {
		t.Fatalf("caller cue ID = %v, want zero value", got)
	}
}

func TestSavePreservesMediaFilenameAndNumbersCollisions(t *testing.T) {
	cues := make([]show.Cue, 0, 3)
	for i, content := range []string{"first", "second", "third"} {
		directory := filepath.Join(t.TempDir(), content)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(directory, "abcd.opus")
		if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cues = append(cues, show.Cue{
			CueNumber: string(rune('1' + i)),
			Type:      show.CueTypeSound,
			Play:      show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: source}}},
		})
	}

	var archive bytes.Buffer
	manifest, err := Save(&archive, show.Show{Cues: cues}, filepath.Join(t.TempDir(), "missing-ffmpeg"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	want := []string{"media/abcd.opus", "media/abcd-1.opus", "media/abcd-2.opus"}
	for i, cue := range manifest.Show.Cues {
		if got := cue.Play.Sound.File; got != want[i] {
			t.Errorf("cue %d media path = %q, want %q", i, got, want[i])
		}
	}
}

func TestUniqueAssetPathUsesConvertedExtension(t *testing.T) {
	used := map[string]struct{}{}
	if got := uniqueAssetPath(filepath.Join("source", "abcd.wav"), ".opus", used); got != "media/abcd.opus" {
		t.Fatalf("first media path = %q, want %q", got, "media/abcd.opus")
	}
	if got := uniqueAssetPath(filepath.Join("other", "abcd.mp3"), ".opus", used); got != "media/abcd-1.opus" {
		t.Fatalf("second media path = %q, want %q", got, "media/abcd-1.opus")
	}
}
