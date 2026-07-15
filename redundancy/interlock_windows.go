//go:build windows

package redundancy

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type systemInterlock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireSystemInterlock(path string) (*systemInterlock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock := &systemInterlock{file: file}
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.overlapped)
	if err != nil {
		_ = file.Close()
		// TODO(micro): map additional Windows lock errors if observed in the field; currently only LOCK/SHARING_VIOLATION → ErrInterlockBusy
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, ErrInterlockBusy
		}
		return nil, err
	}
	return lock, nil
}

func (l *systemInterlock) Touch(payload []byte) error {
	if err := l.file.Truncate(0); err != nil {
		return err
	}
	if _, err := l.file.Seek(0, 0); err != nil {
		return err
	}
	if _, err := l.file.Write(payload); err != nil {
		return err
	}
	return l.file.Sync()
}

func (l *systemInterlock) Close() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
