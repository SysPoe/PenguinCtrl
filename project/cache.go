package project

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type CacheMaintainer struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func StartCacheMaintainer(active func() bool, protected func() []string, limits func() (quotaBytes, reserveBytes uint64)) *CacheMaintainer {
	ctx, cancel := context.WithCancel(context.Background())
	m := &CacheMaintainer{cancel: cancel}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			if !active() {
				quota, reserve := limits()
				_ = MaintainCache(quota, reserve, protected())
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

type cacheObject struct {
	path string
	size uint64
	used time.Time
}

func MaintainCache(quotaBytes, reserveBytes uint64, protected []string) error {
	root, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	return maintainCacheRoot(filepath.Join(root, "CuSus"), quotaBytes, reserveBytes, protected, cacheAvailableBytes)
}

func maintainCacheRoot(root string, quotaBytes, reserveBytes uint64, protected []string, available func(string) (uint64, error)) error {
	objects, err := cacheObjects(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	protectedAbs := make([]string, 0, len(protected))
	for _, path := range protected {
		if absolute, e := filepath.Abs(path); e == nil {
			protectedAbs = append(protectedAbs, strings.ToLower(filepath.Clean(absolute)))
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
	for _, directory := range []string{"shows", "transcoded"} {
		entries, err := os.ReadDir(filepath.Join(root, directory))
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
			path := filepath.Join(root, directory, entry.Name())
			var size uint64
			used := time.Time{}
			_ = filepath.WalkDir(path, func(child string, item fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				info, err := item.Info()
				if err != nil {
					return nil
				}
				if info.ModTime().After(used) {
					used = info.ModTime()
				}
				if !item.IsDir() {
					size += uint64(max(int64(0), info.Size()))
				}
				return nil
			})
			result = append(result, cacheObject{path: path, size: size, used: used})
		}
	}
	return result, nil
}

func cacheObjectProtected(object string, protected []string) bool {
	object = strings.ToLower(filepath.Clean(object))
	for _, path := range protected {
		if path == object || strings.HasPrefix(path, object+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func touchCachePath(path string) { now := time.Now(); _ = os.Chtimes(path, now, now) }
