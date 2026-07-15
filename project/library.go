package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
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
	// TODO(micro): pre-size files with len(l.files) when kind=="" to avoid growth from zero
	var files []File
	for _, file := range l.files {
		if kind == "" || file.Kind == kind {
			files = append(files, file)
		}
	}
	// TODO(micro): prefer slices.SortFunc with strings.Compare on lowercased names
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files
}

func (l *Library) Replace(files []File) {
	l.mu.Lock()
	// TODO(micro): prefer slices.Clone(files)
	l.files = append([]File(nil), files...)
	l.mu.Unlock()
}

func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open media %q: %w", path, err)
	}
	// TODO(micro): Explicitly discard or return this read-only Close error so the cleanup policy is clear.
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash media %q: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// LocalPath converts file:// paths returned by native pickers into OS paths.
func LocalPath(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("media path is empty")
	}
	parsed, err := url.Parse(source)
	if err == nil && strings.EqualFold(parsed.Scheme, "file") {
		path, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return "", fmt.Errorf("decode media URI: %w", err)
		}
		if parsed.Host != "" {
			path = "//" + parsed.Host + path
		}
		if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		return filepath.FromSlash(path), nil
	}
	if err == nil && parsed.Scheme != "" && len(parsed.Scheme) != 1 {
		return "", fmt.Errorf("unsupported media URI %q", source)
	}
	return filepath.Clean(source), nil
}
