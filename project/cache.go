package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const cacheMaintenanceInterval = 5 * time.Minute

// TODO(macro): CacheMaintainer is a background reaper of published media cache
// but its protected-path policy is closed over from window_loop via callbacks into
// show/settings. Push protection policy into the project session (or archive
// package) so cache lifetime is not orchestrated from the Gio event loop.
type CacheMaintainer struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	errMu  sync.RWMutex
	err    error
}

func StartCacheMaintainer(active func() bool, protected func() []string, limits func() (quotaBytes, reserveBytes uint64)) *CacheMaintainer {
	ctx, cancel := context.WithCancel(context.Background())
	m := &CacheMaintainer{cancel: cancel}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(cacheMaintenanceInterval)
		defer ticker.Stop()
		for {
			if !active() {
				quota, reserve := limits()
				m.errMu.Lock()
				m.err = MaintainCache(quota, reserve, protected())
				m.errMu.Unlock()
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return m
}

func (m *CacheMaintainer) Close() { m.cancel(); m.wg.Wait() }

// Err returns the most recent cache-maintenance failure, if any.
func (m *CacheMaintainer) Err() error {
	m.errMu.RLock()
	defer m.errMu.RUnlock()
	return m.err
}

type cacheObject struct {
	path string
	size uint64
	used time.Time
}

func MaintainCache(quotaBytes, reserveBytes uint64, protected []string) error {
	cache, err := currentCacheLayout()
	if err != nil {
		return err
	}
	return maintainCacheRoot(cache.Root, quotaBytes, reserveBytes, protected, cacheAvailableBytes)
}

func maintainCacheRoot(root string, quotaBytes, reserveBytes uint64, protected []string, available func(string) (uint64, error)) error {
	objects, err := cacheObjects(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	protectedAbs := make([]string, 0, len(protected))
	for _, path := range protected {
		if absolute, e := filepath.Abs(path); e == nil {
			protectedAbs = append(protectedAbs, cachePathKey(absolute))
		}
	}
	var total uint64
	for _, object := range objects {
		total += object.size
	}
	free, freeErr := available(root)
	if freeErr != nil {
		free = reserveBytes
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].used.Before(objects[j].used) })
	for _, object := range objects {
		if total <= quotaBytes && free >= reserveBytes {
			break
		}
		if cacheObjectProtected(object.path, protectedAbs) {
			continue
		}
		if err := os.RemoveAll(object.path); err != nil {
			continue
		}
		total -= min(total, object.size)
		free += object.size
	}
	if total > quotaBytes || free < reserveBytes {
		return errors.New("cache cannot satisfy quota/free-space reserve while active show assets are protected")
	}
	return nil
}

func cacheObjects(root string) ([]cacheObject, error) {
	var result []cacheObject
	for _, directory := range cacheLayoutFromRoot(root).objectRoots() {
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
			object, err := inspectCacheObject(path)
			if err != nil {
				return nil, err
			}
			result = append(result, object)
		}
	}
	return result, nil
}

func inspectCacheObject(path string) (cacheObject, error) {
	object := cacheObject{path: path}
	if err := filepath.WalkDir(path, func(child string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(object.used) {
			object.used = info.ModTime()
		}
		if !item.IsDir() {
			object.size += uint64(max(int64(0), info.Size()))
		}
		return nil
	}); err != nil {
		return cacheObject{}, fmt.Errorf("inspect cache object %q: %w", path, err)
	}
	return object, nil
}

func cacheObjectProtected(object string, protected []string) bool {
	object = cachePathKey(object)
	for _, path := range protected {
		path = cachePathKey(path)
		if path == object || strings.HasPrefix(path, object+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func cachePathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func touchCachePath(path string) error {
	now := time.Now()
	return os.Chtimes(path, now, now)
}
