// TODO(micro): Add a package comment and Go-style docs for the exported crash-output lifecycle functions.
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

// TODO(macro): Package-level mutable state makes crash reporting a hidden
// process singleton (directory, fatal file) that every caller must configure
// globally. Prefer an explicit Reporter value constructed at process start and
// passed/injected so tests and secondary binaries do not race on shared state.
var state struct {
	sync.RWMutex
	directory string
	fatalFile *os.File
	fatalPath string
}

func SetDirectory(directory string) {
	state.Lock()
	state.directory = directory
	state.Unlock()
}

// InstallFatalOutput asks the Go runtime to duplicate every unrecovered panic
// and fatal throw, including those from arbitrary goroutines, into a durable
// session crash file before the process terminates.
func InstallFatalOutput() error {
	state.Lock()
	defer state.Unlock()
	if state.fatalFile != nil {
		return nil
	}
	directory := state.directory
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
	state.fatalFile, state.fatalPath = file, path
	return nil
}

func CloseFatalOutput(clean bool) {
	state.Lock()
	file, path := state.fatalFile, state.fatalPath
	state.fatalFile, state.fatalPath = nil, ""
	state.Unlock()
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
func Go(name string, work func()) {
	go func() {
		defer func() {
			if value := recover(); value != nil {
				_ = Write(name, value, debug.Stack())
				panic(value)
			}
		}()
		work()
	}()
}

func Write(component string, value any, stack []byte) error {
	state.RLock()
	directory := state.directory
	state.RUnlock()
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
