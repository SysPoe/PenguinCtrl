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

func TestReleasedManifestFixturesMigrateAndValidate(t *testing.T) {
	for _, name := range []string{"manifest-v1.json", "manifest-v2.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var manifest Manifest
			if err := decodeManifest(bytes.NewReader(raw), &manifest); err != nil {
				t.Fatal(err)
			}
			original := manifest.Version
			if err := migrateManifest(&manifest); err != nil {
				t.Fatal(err)
			}
			if err := validateManifestSchema(manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Version != Version || manifest.OriginalVersion != original || len(manifest.Show.Cues) != 1 {
				t.Fatalf("migrated manifest = %+v", manifest)
			}
			if name == "manifest-v2.json" && len(manifest.Show.Extensions["fixture.example"]) == 0 {
				t.Fatal("supported extension was not preserved with the show")
			}
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(encoded, []byte(`"cueNumber"`)) || bytes.Contains(encoded, []byte(`"CueNumber"`)) {
				t.Fatalf("manifest did not use explicit version-2 field names: %s", encoded)
			}
		})
	}
}

func TestLoadMigratesWithoutRewritingSourceArchive(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := writeRawManifestArchive(t, raw)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	setTestCache(t)
	manifest, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("migration rewrote the source archive")
	}
	if manifest.Version != Version || manifest.OriginalVersion != 1 {
		t.Fatalf("loaded migration identity = %d/%d", manifest.Version, manifest.OriginalVersion)
	}
}

func TestFutureManifestVersionIsRejectedWithUpdateGuidance(t *testing.T) {
	manifest := Manifest{Format: Format, Version: Version + 1}
	err := migrateManifest(&manifest)
	if err == nil || !strings.Contains(err.Error(), "update CuSus") || !strings.Contains(err.Error(), "only copy") {
		t.Fatalf("future-version error = %v", err)
	}
}

func TestManifestValidationRejectsDomainInvariantViolations(t *testing.T) {
	first := show.NewSoundCue()
	first.CueNumber = "1"
	second := show.NewWaitCue()
	second.CueNumber = "2"
	base := Manifest{Format: Format, Version: Version, Show: show.Show{Cues: []show.Cue{first, second}}}

	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"payload mismatch", func(manifest *Manifest) { manifest.Show.Cues[0].Play.Video = &show.VideoPlay{} }, "payload"},
		{"inconsistent group title", func(manifest *Manifest) {
			id := show.NewGroupID()
			manifest.Show.Cues[0].GroupID, manifest.Show.Cues[0].GroupTitle = id, "Act One"
			manifest.Show.Cues[1].GroupID, manifest.Show.Cues[1].GroupTitle = id, "Act 1"
		}, "inconsistent title"},
		{"unknown link target", func(manifest *Manifest) {
			manifest.Show.Cues[0].Link = show.CueLink{Mode: show.CueLinkStartPlay, Target: show.CueTarget{Kind: show.CueTargetCue, CueID: show.NewCueID()}}
		}, "unknown cue ID"},
		{"unknown media group", func(manifest *Manifest) {
			manifest.Show.Cues[1].Play.Wait.Kind = show.WaitMediaEnd
			manifest.Show.Cues[1].Play.Wait.Media = show.MediaTarget{Kind: show.MediaTargetGroup, GroupID: show.NewGroupID()}
		}, "unknown media group"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := base
			manifest.Show = show.CloneShow(base.Show)
			test.mutate(&manifest)
			err := validateManifestSchema(manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSaveEmitsCurrentGoldenSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "manifest-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden Manifest
	if err := decodeManifest(bytes.NewReader(raw), &golden); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err := Save(&archive, golden.Show, "ffmpeg"); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(encoded, []byte(`"version": 3`)) || !bytes.Contains(encoded, []byte(`"cueNumber": "1"`)) {
			t.Fatalf("saved manifest is not current schema: %s", encoded)
		}
		return
	}
	t.Fatal("saved archive has no manifest")
}

func writeRawManifestArchive(t *testing.T, manifest []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.cusus")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("manifest.json")
	if err == nil {
		_, err = entry.Write(manifest)
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
