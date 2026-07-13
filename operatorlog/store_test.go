package operatorlog

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestStoreAcknowledgementAndCueFailure(t *testing.T) {
	store := NewStore()
	cueID := show.NewCueID()
	warning := store.Add(Warning, "Preflight", "disabled cue", cueID, "1")
	failure := store.Add(ShowStopping, "FFmpeg", "decoder exited", cueID, "1")

	latest, ok := store.LatestUnacknowledged()
	if !ok || latest.ID != failure.ID {
		t.Fatalf("latest = %#v, %v", latest, ok)
	}
	if cueFailure, ok := store.CueFailure(cueID); !ok || cueFailure.ID != failure.ID {
		t.Fatalf("cue failure = %#v, %v", cueFailure, ok)
	}
	if !store.Acknowledge(failure.ID) {
		t.Fatal("failure was not acknowledged")
	}
	latest, ok = store.LatestUnacknowledged()
	if !ok || latest.ID != warning.ID {
		t.Fatalf("latest after acknowledgement = %#v, %v", latest, ok)
	}
	store.AcknowledgeAll()
	if _, ok := store.LatestUnacknowledged(); ok {
		t.Fatal("unexpected unacknowledged event")
	}
	if removed := store.ClearAcknowledged(); removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
}

func TestStoreWritesPersistentJSONLEventLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "operator-events.jsonl")
	store := NewStore()
	store.SetLogPath(path)
	store.Add(Recoverable, "Media", "decoder failed", show.CueID{}, "4")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "decoder failed") || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("log = %q", raw)
	}
}

func TestSupportBundleRedactsSettingsAndIncludesIdentity(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"apiToken":"private","host":"console.local"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	store.SetContext("build-123", func() string { return "show-456" })
	store.SetLogPath(filepath.Join(directory, "operator-events.jsonl"))
	store.Add(Warning, "Device", "endpoint changed", show.CueID{}, "")
	destination := filepath.Join(directory, "support.zip")
	if err := store.ExportSupportBundle(destination, settingsPath, filepath.Join(directory, "missing-crashes")); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	files := map[string][]byte{}
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name], err = io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(string(files["configuration/settings.redacted.json"]), "private") || !strings.Contains(string(files["configuration/settings.redacted.json"]), "[REDACTED]") {
		t.Fatalf("settings were not redacted: %s", files["configuration/settings.redacted.json"])
	}
	var manifest supportManifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil || manifest.BuildID != "build-123" || manifest.ShowID != "show-456" {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}
}
