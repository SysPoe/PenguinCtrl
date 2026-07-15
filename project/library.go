package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/syspoe/cusus/internal/mediapath"
)

// File is a media file currently available to the open project.
type File struct {
	Name   string `json:"name"`
	Source string `json:"-"`
	Hash   string `json:"sha256"`
	Kind   string `json:"kind"`
}

// Library keeps a content-addressed list of local media available to the open
// session. File.Source is always a runtime OS path; portable media/... paths and
// archive metadata belong to Manifest and are converted by ProjectSession.
// Selecting the same bytes twice returns the original entry rather than adding
// a duplicate.
type Library struct {
	mu    sync.RWMutex
	files []File
}

func NewLibrary() *Library { return &Library{} }

func (l *Library) Add(source, kind string) (File, bool, error) {
	path, err := LocalPath(source)
	if err != nil {
		return File{}, false, err
	}
	hash, err := HashFile(path)
	if err != nil {
		return File{}, false, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, existing := range l.files {
		if existing.Hash == hash && existing.Kind == kind {
			return existing, true, nil
		}
	}
	entry := File{Name: filepath.Base(path), Source: path, Hash: hash, Kind: kind}
	l.files = append(l.files, entry)
	return entry, false, nil
}

func (l *Library) Files(kind string) []File {
	l.mu.RLock()
	defer l.mu.RUnlock()
	capacity := 0
	if kind == "" {
		capacity = len(l.files)
	}
	files := make([]File, 0, capacity)
	for _, file := range l.files {
		if kind == "" || file.Kind == kind {
			files = append(files, file)
		}
	}
	slices.SortStableFunc(files, func(first, second File) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return files
}

func (l *Library) Replace(files []File) {
	l.mu.Lock()
	l.files = slices.Clone(files)
	l.mu.Unlock()
}

func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open media %q: %w", path, err)
	}
	defer func() {
		// HashFile is read-only; read errors carry the useful path context, while
		// a close error cannot invalidate bytes already consumed by the hasher.
		_ = file.Close()
	}()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash media %q: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// LocalPath converts file:// paths returned by native pickers into OS paths.
func LocalPath(source string) (string, error) {
	return mediapath.Local(source)
}
