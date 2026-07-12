package operatorlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
)

type Severity int

const (
	Warning Severity = iota
	Recoverable
	ShowStopping
)

func (s Severity) Label() string {
	switch s {
	case Warning:
		return "WARNING"
	case Recoverable:
		return "RECOVERABLE FAILURE"
	case ShowStopping:
		return "SHOW-STOPPING FAILURE"
	default:
		return "EVENT"
	}
}

type Event struct {
	ID             string
	Timestamp      time.Time
	Severity       Severity
	Source         string
	Message        string
	CueID          show.CueID
	CueNumber      string
	AcknowledgedAt time.Time
}

type PreflightCheck struct {
	Severity     Severity
	Code         string
	Source       string
	Message      string
	Consequence  string
	Fix          string
	Field        string
	CueID        show.CueID
	CueNumber    string
	Fingerprint  string
	Acknowledged bool
}

func (e Event) Acknowledged() bool { return !e.AcknowledgedAt.IsZero() }

type Store struct {
	mu       sync.RWMutex
	logMu    sync.Mutex
	events   []Event
	onChange func()
	logPath  string
}

func NewStore() *Store { return &Store{} }

func (s *Store) SetLogPath(path string) {
	s.mu.Lock()
	s.logPath = strings.TrimSpace(path)
	s.mu.Unlock()
}

func (s *Store) SetOnChange(callback func()) {
	s.mu.Lock()
	s.onChange = callback
	s.mu.Unlock()
}

func (s *Store) Add(severity Severity, source, message string, cueID show.CueID, cueNumber string) Event {
	event := Event{
		ID: uuid.NewString(), Timestamp: time.Now(), Severity: severity,
		Source: strings.TrimSpace(source), Message: strings.TrimSpace(message),
		CueID: cueID, CueNumber: strings.TrimSpace(cueNumber),
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	if len(s.events) > 1000 {
		s.events = append([]Event(nil), s.events[len(s.events)-1000:]...)
	}
	callback := s.onChange
	logPath := s.logPath
	s.mu.Unlock()
	if logPath != "" {
		s.logMu.Lock()
		_ = appendEventLog(logPath, event)
		s.logMu.Unlock()
	}
	if callback != nil {
		callback()
	}
	return event
}

func appendEventLog(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= 5*1024*1024 {
		_ = os.Remove(path + ".1")
		if err := os.Rename(path, path+".1"); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(raw, '\n'))
	return err
}

func (s *Store) Snapshot() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

func (s *Store) LatestUnacknowledged() (Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result Event
	found := false
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		if event.Acknowledged() {
			continue
		}
		if !found || event.Severity > result.Severity {
			result, found = event, true
		}
	}
	return result, found
}

func (s *Store) CueFailure(cueID show.CueID) (Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		if event.CueID == cueID && event.Severity >= Recoverable {
			return event, true
		}
	}
	return Event{}, false
}

func (s *Store) Event(id string) (Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		if s.events[i].ID == id {
			return s.events[i], true
		}
	}
	return Event{}, false
}

func (s *Store) Acknowledge(id string) bool {
	s.mu.Lock()
	changed := false
	for i := range s.events {
		if s.events[i].ID == id && !s.events[i].Acknowledged() {
			s.events[i].AcknowledgedAt = time.Now()
			changed = true
			break
		}
	}
	callback := s.onChange
	s.mu.Unlock()
	if changed && callback != nil {
		callback()
	}
	return changed
}

func (s *Store) AcknowledgeAll() int {
	s.mu.Lock()
	now, count := time.Now(), 0
	for i := range s.events {
		if !s.events[i].Acknowledged() {
			s.events[i].AcknowledgedAt = now
			count++
		}
	}
	callback := s.onChange
	s.mu.Unlock()
	if count > 0 && callback != nil {
		callback()
	}
	return count
}

func (s *Store) ClearAcknowledged() int {
	s.mu.Lock()
	kept := s.events[:0]
	removed := 0
	for _, event := range s.events {
		if event.Acknowledged() {
			removed++
			continue
		}
		kept = append(kept, event)
	}
	s.events = kept
	callback := s.onChange
	s.mu.Unlock()
	if removed > 0 && callback != nil {
		callback()
	}
	return removed
}
