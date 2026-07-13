//go:build !windows

package redundancy

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type systemInterlock struct{ file *os.File }

func acquireSystemInterlock(path string) (*systemInterlock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrInterlockBusy
		}
		return nil, err
	}
	return &systemInterlock{file: file}, nil
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
	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
