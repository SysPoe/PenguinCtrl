package operatorlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	eventLogMaxBytes    = 5 * 1024 * 1024
	eventLogGenerations = 4
)

type durableLog struct {
	mu        sync.Mutex
	path      string
	lastError error
}

func (l *durableLog) SetPath(path string) {
	l.mu.Lock()
	l.path = strings.TrimSpace(path)
	l.mu.Unlock()
}

func (l *durableLog) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

func (l *durableLog) Append(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.path == "" {
		return nil
	}
	err := appendEventLog(l.path, event)
	if err != nil {
		l.lastError = err
	}
	return err
}

func (l *durableLog) Error() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastError
}

func appendEventLog(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= eventLogMaxBytes {
		if err := rotateEventLogs(path, eventLogGenerations); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(raw, '\n'))
	return err
}

func rotateEventLogs(path string, generations int) error {
	_ = os.Remove(path + "." + strconv.Itoa(generations))
	for generation := generations - 1; generation >= 1; generation-- {
		from, to := path+"."+strconv.Itoa(generation), path+"."+strconv.Itoa(generation+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(path, path+".1")
}
