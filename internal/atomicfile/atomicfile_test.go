package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFuncFailureKeepsPublishedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Write(path, []byte("published"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("encode failed")
	err := WriteFunc(path, 0o600, func(file *os.File) error {
		if _, err := file.WriteString("partial"); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WriteFunc error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "published" {
		t.Fatalf("published file = %q", raw)
	}
}

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
