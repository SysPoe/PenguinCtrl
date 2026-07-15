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

type CacheMaintainer struct {
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	errMu     sync.RWMutex
	err       error
	active    func() bool
	protected func() []string
	limits    func() (quotaBytes, reserveBytes uint64)

	lifecycleMu sync.Mutex
	sessions    map[*ProjectSession][]string
}

// StartCacheMaintainer starts cache maintenance with a compatibility callback
// for paths owned outside ProjectSession. Sessions opened through OpenSession
// are protected by the maintainer directly and do not need that callback.
func StartCacheMaintainer(active func() bool, protected func() []string, limits func() (quotaBytes, reserveBytes uint64)) *CacheMaintainer {
	ctx, cancel := context.WithCancel(context.Background())
	m := &CacheMaintainer{
		cancel:    cancel,
		active:    active,
		protected: protected,
		limits:    limits,
		sessions:  make(map[*ProjectSession][]string),
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(cacheMaintenanceInterval)
		defer ticker.Stop()
		for {
			_ = m.MaintainNow()
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

// OpenSession opens an archive and registers its extracted cache object before
// maintenance can run. Closing the returned session releases that protection.
func (m *CacheMaintainer) OpenSession(path string) (*ProjectSession, error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	session, err := OpenSession(path)
	if err != nil {
		return nil, err
	}
	if m.sessions == nil {
		m.sessions = make(map[*ProjectSession][]string)
	}
	m.sessions[session] = session.ProtectedPaths()
	session.bindCacheProtection(func() {
		m.lifecycleMu.Lock()
		delete(m.sessions, session)
		m.lifecycleMu.Unlock()
	})
	return session, nil
}

// MaintainNow performs one maintenance pass. It is also useful for explicit
// maintenance at lifecycle boundaries and for deterministic diagnostics.
func (m *CacheMaintainer) MaintainNow() error {
	if m.active != nil && m.active() {
		return nil
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	var protected []string
	if m.protected != nil {
		protected = append(protected, m.protected()...)
	}
	for _, paths := range m.sessions {
		protected = append(protected, paths...)
	}
	var quota, reserve uint64
	if m.limits != nil {
		quota, reserve = m.limits()
	}
	err := MaintainCache(quota, reserve, protected)
	m.errMu.Lock()
	m.err = err
	m.errMu.Unlock()
	return err
}

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
