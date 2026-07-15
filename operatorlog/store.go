package operatorlog

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
)

const (
	eventMemoryCapacity = 1000
	eventLogMaxBytes    = 5 * 1024 * 1024
	eventLogGenerations = 4
)

type Severity int

const (
	Info Severity = iota
	Warning
	Recoverable
	CueFailure
	ShowStopping
)

func (s Severity) Label() string {
	switch s {
	case Info:
		return "INFORMATION"
	case Warning:
		return "WARNING"
	case Recoverable:
		return "RECOVERABLE FAILURE"
	case CueFailure:
		return "CUE FAILURE"
	case ShowStopping:
		return "SHOW-STOPPING FAILURE"
	default:
		return "EVENT"
	}
}

type Event struct {
	ID             string         `json:"id"`
	Sequence       uint64         `json:"sequence"`
	Timestamp      time.Time      `json:"timestamp"`
	SessionID      string         `json:"sessionId"`
	BuildID        string         `json:"buildId"`
	ShowID         string         `json:"showId,omitempty"`
	Severity       Severity       `json:"severity"`
	Source         string         `json:"source"`
	Message        string         `json:"message"`
	CueID          show.CueID     `json:"cueId,omitempty"`
	CueNumber      string         `json:"cueNumber,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
	AcknowledgedAt time.Time      `json:"acknowledgedAt,omitempty"`
}

func (e Event) Acknowledged() bool { return !e.AcknowledgedAt.IsZero() }

// TODO(macro): Store fuses three sinks—UI ring buffer, diagnostic-only JSONL
// (appendDiagnostic never enters events), and rotated durable logs—behind one
// type with dual locks. Split operator-facing Event history from a diagnostic
// log writer so acknowledgement/snapshot APIs stop sharing lifecycle with
// console capture and disk rotation policy.
type Store struct {
	mu        sync.RWMutex
	logMu     sync.Mutex
	events    []Event
	onChange  func()
	logPath   string
	sessionID string
	buildID   string
	showID    func() string
	sequence  uint64
	logError  error
}

func NewStore() *Store { return &Store{sessionID: uuid.NewString()} }

func (s *Store) SetContext(buildID string, showID func() string) {
	s.mu.Lock()
	s.buildID, s.showID = strings.TrimSpace(buildID), showID
	s.mu.Unlock()
}

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
	return s.AddDetails(severity, source, message, cueID, cueNumber, nil)
}

func (s *Store) AddDetails(severity Severity, source, message string, cueID show.CueID, cueNumber string, details map[string]any) Event {
	s.mu.Lock()
	event := s.newEventLocked(severity, source, message, cueID, cueNumber, details)
	s.events = append(s.events, event)
	if len(s.events) > eventMemoryCapacity {
		s.events = append([]Event(nil), s.events[len(s.events)-eventMemoryCapacity:]...)
	}
	callback := s.onChange
	logPath := s.logPath
	s.mu.Unlock()
	if logPath != "" {
		s.persist(logPath, event)
	}
	if callback != nil {
		callback()
	}
	return event
}

// Writer captures otherwise console-only diagnostics in the structured log
// without turning informational runtime messages into operator alarms.
func (s *Store) Writer(source string) io.Writer {
	return &eventWriter{store: s, source: strings.TrimSpace(source)}
}

func (s *Store) Diagnostic(source, message string, details map[string]any) {
	s.appendDiagnostic(source, message, details)
}

type eventWriter struct {
	store  *Store
	source string
}

func (w *eventWriter) Write(p []byte) (int, error) {
	for _, line := range bytes.Split(p, []byte{'\n'}) {
		message := strings.TrimSpace(string(line))
		if message != "" {
			w.store.appendDiagnostic(w.source, message, map[string]any{"console": true})
		}
	}
	return len(p), nil
}

func (s *Store) appendDiagnostic(source, message string, details map[string]any) {
	s.mu.Lock()
	event := s.newEventLocked(Info, source, message, show.CueID{}, "", details)
	logPath := s.logPath
	s.mu.Unlock()
	if logPath != "" {
		s.persist(logPath, event)
	}
}

func (s *Store) newEventLocked(severity Severity, source, message string, cueID show.CueID, cueNumber string, details map[string]any) Event {
	s.sequence++
	showID := ""
	if s.showID != nil {
		showID = s.showID()
	}
	return Event{
		ID: uuid.NewString(), Sequence: s.sequence, Timestamp: time.Now(), SessionID: s.sessionID, BuildID: s.buildID, ShowID: showID, Severity: severity,
		Source: strings.TrimSpace(source), Message: strings.TrimSpace(message), CueID: cueID, CueNumber: strings.TrimSpace(cueNumber), Details: details,
	}
}

func (s *Store) persist(path string, event Event) {
	s.logMu.Lock()
	err := appendEventLog(path, event)
	s.logMu.Unlock()
	if err == nil {
		return
	}
	s.mu.Lock()
	s.logError = err
	callback := s.onChange
	s.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// LogError reports the most recent failure to append the durable operator log.
func (s *Store) LogError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logError
}

func appendEventLog(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= eventLogMaxBytes {
		if err := rotateEventLogs(path, eventLogGenerations); err != nil {
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

func rotateEventLogs(path string, generations int) error {
	_ = os.Remove(path + "." + strconv.Itoa(generations))
	for generation := generations - 1; generation >= 1; generation-- {
		from, to := path+"."+strconv.Itoa(generation), path+"."+strconv.Itoa(generation+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(path, path+".1")
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
		if event.Acknowledged() || event.Severity == Info {
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
