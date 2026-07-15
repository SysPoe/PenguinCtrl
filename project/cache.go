package project

import (
	"context"
	"sync"
	"time"

	projectcache "github.com/syspoe/cusus/project/internal/cache"
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
	return projectcache.Maintain(quotaBytes, reserveBytes, protected)
}

func maintainCacheRoot(root string, quotaBytes, reserveBytes uint64, protected []string, available func(string) (uint64, error)) error {
	return projectcache.MaintainRoot(root, quotaBytes, reserveBytes, protected, available)
}

func inspectCacheObject(path string) (cacheObject, error) {
	object, err := projectcache.Inspect(path)
	return cacheObject{path: object.Path, size: object.Size, used: object.Used}, err
}

func cacheObjectProtected(object string, protected []string) bool {
	return projectcache.ObjectProtected(object, protected)
}

func cachePathKey(path string) string {
	return projectcache.PathKey(path)
}

func touchCachePath(path string) error {
	return projectcache.Touch(path)
}
