package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// BackupPath is the last-known-good generation retained beside a published
// file. Keeping one generation makes an interruption between the two Windows
// rename operations recoverable on next startup.
func BackupPath(path string) string { return path + ".backup" }

// Recover restores the last-known-good generation when the primary path is
// absent. It is safe to call on every startup.
func Recover(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	backup := BackupPath(path)
	if _, err := os.Stat(backup); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Rename(backup, path); err != nil {
		return fmt.Errorf("restore backup: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}

// Write atomically publishes data and retains the previous primary as a
// last-known-good backup. The temporary file is created on the same volume,
// flushed, and closed before any rename occurs.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	// TODO(micro): Make cleanup explicit if Remove failure matters, or assign it to _ to document intentional best effort.
	defer os.Remove(tmpPath)
	// TODO(micro): Stop shadowing the outer err here; both Chmod and Write failures are currently discarded before the Sync check.
	if err := tmp.Chmod(perm); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}

	backup := BackupPath(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.Rename(tmpPath, path); err != nil {
			return fmt.Errorf("publish file: %w", err)
		}
		return syncDirectory(dir)
	} else if err != nil {
		return fmt.Errorf("inspect destination: %w", err)
	}

	// The primary remains intact until the fully flushed candidate exists.
	// Removing an older backup cannot lose the current primary.
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old backup: %w", err)
	}
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("preserve previous file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("publish replacement: %w", err)
	}
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return nil // best effort on platforms that do not permit directory handles
	}
	// TODO(micro): Assign this best-effort close result to _ explicitly so the ignored error is intentional.
	defer dir.Close()
	_ = dir.Sync() // Windows and some filesystems may not support this operation.
	return nil
}
