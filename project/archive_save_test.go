package project

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestSaveBundlesSupportedVideoWithoutTranscoding(t *testing.T) {
	source := filepath.Join(t.TempDir(), "clip.mp4")
	content := []byte("representative video bytes")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}
	current := show.Show{Cues: []show.Cue{{
		CueNumber: "1",
		Type:      show.CueTypeVideo,
		Play:      show.CuePlay{Video: &show.VideoPlay{File: source}},
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
