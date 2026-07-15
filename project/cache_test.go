package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheMaintenanceEvictsLRUAndProtectsActiveAssets(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "transcoded", "old.webm")
	protected := filepath.Join(root, "shows", "active", "media", "live.webm")
	newer := filepath.Join(root, "transcoded", "new.webm")
	for _, path := range []string{old, protected, newer} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, 16), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ago := time.Now().Add(-time.Hour)
	_ = os.Chtimes(old, ago, ago)
	if err := maintainCacheRoot(root, 32, 0, []string{protected}, func(string) (uint64, error) { return 1 << 30, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old cache object still exists: %v", err)
	}
	if _, err := os.Stat(protected); err != nil {
		t.Fatalf("protected asset was removed: %v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatalf("newer cache object was removed: %v", err)
	}
}

func TestCacheInspectionAndTouchReportFilesystemFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := inspectCacheObject(missing); err == nil {
		t.Fatal("missing cache object inspection unexpectedly succeeded")
	}
	if err := touchCachePath(missing); err == nil {
		t.Fatal("touching a missing cache path unexpectedly succeeded")
	}
}
