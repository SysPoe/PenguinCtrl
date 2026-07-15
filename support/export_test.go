package support

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func TestExportRedactsSettingsAndIncludesIdentity(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"apiToken":"private","host":"console.local"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := operatorlog.NewStore()
	store.SetContext("build-123", func() string { return "show-456" })
	store.SetLogPath(filepath.Join(directory, "operator-events.jsonl"))
	store.Add(operatorlog.Warning, "Device", "endpoint changed", show.CueID{}, "")
	destination := filepath.Join(directory, "support.zip")
	if err := Export(destination, store.DiagnosticSnapshot(), settingsPath, filepath.Join(directory, "missing-crashes")); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close support archive: %v", err)
		}
	})
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
	var metadata manifest
	if err := json.Unmarshal(files["manifest.json"], &metadata); err != nil || metadata.BuildID != "build-123" || metadata.ShowID != "show-456" {
		t.Fatalf("manifest = %+v, %v", metadata, err)
	}
}
