// Package crashreport captures fatal runtime output and named goroutine panics.
package crashreport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

const retainedCrashLogCount = 10

// Reporter owns crash output for one process composition root.
type Reporter struct {
	mu        sync.RWMutex
	directory string
	fatalFile *os.File
	fatalPath string
}

// New constructs a reporter that writes to directory, or the OS temporary
// directory when directory is empty.
func New(directory string) *Reporter { return &Reporter{directory: directory} }

// SetDirectory changes where future crash reports are retained.
func (r *Reporter) SetDirectory(directory string) {
	r.mu.Lock()
	r.directory = directory
	r.mu.Unlock()
}

// InstallFatalOutput asks the Go runtime to duplicate every unrecovered panic
// and fatal throw, including those from arbitrary goroutines, into a durable
// session crash file before the process terminates.
func (r *Reporter) InstallFatalOutput() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fatalFile != nil {
		return nil
	}
	directory := r.directory
	if directory == "" {
		directory = os.TempDir()
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, fmt.Sprintf("crash-fatal-%d.log", time.Now().UTC().UnixMilli()))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		return errors.Join(err, file.Close())
	}
	r.fatalFile, r.fatalPath = file, path
	return nil
}

// CloseFatalOutput detaches the runtime crash sink and removes it after a clean exit.
func (r *Reporter) CloseFatalOutput(clean bool) {
	r.mu.Lock()
	file, path := r.fatalFile, r.fatalPath
	r.fatalFile, r.fatalPath = nil, ""
	r.mu.Unlock()
	if file == nil {
		return
	}
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = file.Sync()
	_ = file.Close()
	if clean {
		_ = os.Remove(path)
	}
}

// Go starts owned background work with a durable panic report. Re-panicking is
// deliberate: the external supervisor must restart into a known silent state
// instead of allowing a partially failed show-control process to continue.
func (r *Reporter) Go(name string, work func()) {
	go r.Run(name, work)
}

// Run executes owned work with durable panic reporting on the current
// goroutine. Re-panicking lets the external supervisor restore a known state.
func (r *Reporter) Run(name string, work func()) {
	defer func() {
		if value := recover(); value != nil {
			_ = r.Write(name, value, debug.Stack())
			panic(value)
		}
	}()
	work()
}

// Write persists one named panic and stack trace.
func (r *Reporter) Write(component string, value any, stack []byte) error {
	r.mu.RLock()
	directory := r.directory
	r.mu.RUnlock()
	if directory == "" {
		directory = os.TempDir()
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("crash-%s-%d.log", safeName(component), time.Now().UTC().UnixMilli())
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "timestamp=%s\ncomponent=%s\npanic=%v\n\n%s", time.Now().UTC().Format(time.RFC3339Nano), component, value, stack)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	prune(directory, retainedCrashLogCount)
	return err
}

func safeName(value string) string {
	result := make([]byte, 0, len(value))
	for i := range len(value) {
		char := value[i]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' {
			result = append(result, char)
		}
	}
	if len(result) == 0 {
		return "runtime"
	}
	return string(result)
}

func prune(directory string, keep int) {
	entries, err := filepath.Glob(filepath.Join(directory, "crash-*.log"))
	if err != nil || len(entries) <= keep {
		return
	}
	sort.Strings(entries)
	for _, path := range entries[:len(entries)-keep] {
		_ = os.Remove(path)
	}
}
