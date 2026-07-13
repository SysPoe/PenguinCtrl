package crashreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesBoundedDurableCrashReport(t *testing.T) {
	directory := t.TempDir()
	SetDirectory(directory)
	if err := Write("playback worker", "boom", []byte("stack evidence")); err != nil {
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
