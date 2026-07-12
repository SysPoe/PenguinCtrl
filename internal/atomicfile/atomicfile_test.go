package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteRetainsAndRecoversLastKnownGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Write(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(BackupPath(path))
	if err != nil || string(backup) != "first" {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Recover(path); err != nil {
		t.Fatal(err)
	}
	recovered, err := os.ReadFile(path)
	if err != nil || string(recovered) != "first" {
		t.Fatalf("recovered = %q, %v", recovered, err)
	}
}
