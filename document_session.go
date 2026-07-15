package main

import (
	"crypto/sha256"
	"sync"

	"github.com/syspoe/cusus/show"
)

// documentSession owns the mutable identity and dirty state of the open show.
// The Gio loop may read it from health, save, and frame goroutines, so all
// state transitions are kept behind this small synchronized boundary.
type documentSession struct {
	mu              sync.RWMutex
	saveMu          sync.Mutex
	path            string
	lastSavedDigest [sha256.Size]byte
	suppressJournal bool
}

func newDocumentSession(path string, current show.Show, recovered bool) *documentSession {
	session := &documentSession{
		path:            path,
		lastSavedDigest: documentDigest(current),
	}
	if recovered {
		session.lastSavedDigest = [sha256.Size]byte{}
	}
	return session
}

func documentDigest(current show.Show) [sha256.Size]byte {
	digest, err := current.Digest()
	if err == nil {
		return digest
	}
	return sha256.Sum256([]byte("invalid-show:" + err.Error()))
}

func (s *documentSession) status(current show.Show) (path string, dirty, journalSuppressed bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path, documentDigest(current) != s.lastSavedDigest, s.suppressJournal
}

func (s *documentSession) pathSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

func (s *documentSession) beginReplace() {
	s.mu.Lock()
	s.suppressJournal = true
	s.mu.Unlock()
}

func (s *documentSession) finishReplace(path string, current show.Show) {
	s.mu.Lock()
	s.path = path
	s.lastSavedDigest = documentDigest(current)
	s.suppressJournal = false
	s.mu.Unlock()
}

func (s *documentSession) markSaved(path string, current show.Show) {
	s.mu.Lock()
	if path != "" {
		s.path = path
	}
	s.lastSavedDigest = documentDigest(current)
	s.mu.Unlock()
}

func (s *documentSession) reset(current show.Show) {
	s.beginReplace()
	s.finishReplace("", current)
}

func (s *documentSession) serializeSave(save func()) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	save()
}
