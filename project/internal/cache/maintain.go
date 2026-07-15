package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Object struct {
	Path string
	Size uint64
	Used time.Time
}

func Maintain(quotaBytes, reserveBytes uint64, protected []string) error {
	layout, err := CurrentLayout()
	if err != nil {
		return err
	}
	return MaintainRoot(layout.Root, quotaBytes, reserveBytes, protected, AvailableBytes)
}

func MaintainRoot(root string, quotaBytes, reserveBytes uint64, protected []string, available func(string) (uint64, error)) error {
	objects, err := Objects(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	protectedAbs := make([]string, 0, len(protected))
	for _, path := range protected {
		if absolute, e := filepath.Abs(path); e == nil {
			protectedAbs = append(protectedAbs, PathKey(absolute))
		}
	}
	var total uint64
	for _, object := range objects {
		total += object.Size
	}
	free, freeErr := available(root)
	if freeErr != nil {
		free = reserveBytes
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Used.Before(objects[j].Used) })
	for _, object := range objects {
		if total <= quotaBytes && free >= reserveBytes {
			break
		}
		if ObjectProtected(object.Path, protectedAbs) {
			continue
		}
		if err := os.RemoveAll(object.Path); err != nil {
			continue
		}
		total -= min(total, object.Size)
		free += object.Size
	}
	if total > quotaBytes || free < reserveBytes {
		return errors.New("cache cannot satisfy quota/free-space reserve while active show assets are protected")
	}
	return nil
}

func Objects(root string) ([]Object, error) {
	var result []Object
	for _, directory := range LayoutFromRoot(root).ObjectRoots() {
		entries, err := os.ReadDir(directory)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".previous") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			object, err := Inspect(path)
			if err != nil {
				return nil, err
			}
			result = append(result, object)
		}
	}
	return result, nil
}

func Inspect(path string) (Object, error) {
	object := Object{Path: path}
	if err := filepath.WalkDir(path, func(child string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(object.Used) {
			object.Used = info.ModTime()
		}
		if !item.IsDir() {
			object.Size += uint64(max(int64(0), info.Size()))
		}
		return nil
	}); err != nil {
		return Object{}, fmt.Errorf("inspect cache object %q: %w", path, err)
	}
	return object, nil
}

func ObjectProtected(object string, protected []string) bool {
	object = PathKey(object)
	for _, path := range protected {
		path = PathKey(path)
		if path == object || strings.HasPrefix(path, object+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func PathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func Touch(path string) error {
	now := time.Now()
	return os.Chtimes(path, now, now)
}
