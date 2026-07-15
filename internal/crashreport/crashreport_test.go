package crashreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesBoundedDurableCrashReport(t *testing.T) {
	directory := t.TempDir()
	reporter := New(directory)
	if err := reporter.Write("playback worker", "boom", []byte("stack evidence")); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(directory, "crash-*.log"))
	if len(files) != 1 {
		t.Fatalf("crash reports = %d", len(files))
	}
	raw, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(raw), "boom") || !strings.Contains(string(raw), "stack evidence") {
		t.Fatalf("crash report = %q, %v", raw, err)
	}
}

func TestReportersKeepDirectoriesIsolated(t *testing.T) {
	firstDirectory, secondDirectory := t.TempDir(), t.TempDir()
	first, second := New(firstDirectory), New(secondDirectory)
	if err := first.Write("first", "boom", nil); err != nil {
		t.Fatal(err)
	}
	if err := second.Write("second", "boom", nil); err != nil {
		t.Fatal(err)
	}
	firstFiles, _ := filepath.Glob(filepath.Join(firstDirectory, "crash-*.log"))
	secondFiles, _ := filepath.Glob(filepath.Join(secondDirectory, "crash-*.log"))
	if len(firstFiles) != 1 || len(secondFiles) != 1 || strings.Contains(firstFiles[0], "second") || strings.Contains(secondFiles[0], "first") {
		t.Fatalf("reporter files were not isolated: %v %v", firstFiles, secondFiles)
	}
}
