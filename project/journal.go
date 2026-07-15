package project

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syspoe/cusus/internal/atomicfile"
	"github.com/syspoe/cusus/show"
)

const (
	journalVersion  = 1
	maxJournalBytes = 8 << 20
	maxRecordBytes  = 4 << 20
)

// TODO(macro): Decide whether recovery owns full show documents or deltas —
// EditJournal appends entire show.Show blobs (including media absolute paths and
// acknowledgements) for every dirty mark, mixing crash recovery with document
// persistence and exploding size as shows grow.
// RecoveryRecord is a flushed show-state generation. Dirty records are
// recoverable; clean records prove that the same state reached its document.
type RecoveryRecord struct {
	Version      int       `json:"version"`
	WrittenAt    time.Time `json:"writtenAt"`
	DocumentPath string    `json:"documentPath,omitempty"`
	Digest       string    `json:"digest"`
	Dirty        bool      `json:"dirty"`
	Show         show.Show `json:"show"`
}

// TODO(macro): EditJournal digests/shows independently of package-main showDigest
// and document dirty tracking in window_loop. Own dirty identity and recovery in
// one document-session type so journal, save path, and UI dirty chrome cannot
// disagree on what "saved" means.
type EditJournal struct {
	mu   sync.Mutex
	path string
}

func OpenEditJournal(path string) (*EditJournal, error) {
	if path == "" {
		return nil, errors.New("edit journal path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create edit journal directory: %w", err)
	}
	if err := atomicfile.Recover(path); err != nil {
		return nil, fmt.Errorf("recover edit journal: %w", err)
	}
	return &EditJournal{path: path}, nil
}

func (j *EditJournal) RecordDirty(current show.Show, documentPath string) error {
	return j.append(current, documentPath, true)
}

func (j *EditJournal) MarkSaved(current show.Show, documentPath string) error {
	return j.append(current, documentPath, false)
}

func (j *EditJournal) Recover() (RecoveryRecord, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readLastJournalRecord(j.path)
	if err != nil || !found || !record.Dirty {
		return record, false, err
	}
	if record.Version != journalVersion || record.Digest != showStateDigest(record.Show) {
		return RecoveryRecord{}, false, errors.New("edit journal recovery record failed validation")
	}
	return record, true, nil
}

func (j *EditJournal) append(current show.Show, documentPath string, dirty bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	record := RecoveryRecord{
		Version: journalVersion, WrittenAt: time.Now().UTC(), DocumentPath: documentPath,
		Digest: showStateDigest(current), Dirty: dirty, Show: current,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode edit journal: %w", err)
	}
	if len(raw) > maxRecordBytes {
		return fmt.Errorf("edit journal record is %d bytes; limit is %d", len(raw), maxRecordBytes)
	}
	if info, err := os.Stat(j.path); err == nil && info.Size()+int64(len(raw)+1) > maxJournalBytes {
		return atomicfile.Write(j.path, append(raw, '\n'), 0o600)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect edit journal: %w", err)
	}
	file, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open edit journal: %w", err)
	}
	if _, err = file.Write(append(raw, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("flush edit journal: %w", err)
	}
	return nil
}

func readLastJournalRecord(path string) (RecoveryRecord, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return RecoveryRecord{}, false, nil
	}
	if err != nil {
		return RecoveryRecord{}, false, fmt.Errorf("open edit journal: %w", err)
	}
	// TODO(micro): Explicitly discard or return this read-only Close error so the cleanup policy is clear.
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	var last RecoveryRecord
	found := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxRecordBytes+1 {
			return RecoveryRecord{}, false, errors.New("edit journal contains an oversized record")
		}
		if len(line) > 0 {
			var candidate RecoveryRecord
			if err := json.Unmarshal(line, &candidate); err != nil {
				if readErr == io.EOF { // Ignore only a torn final append.
					break
				}
				return RecoveryRecord{}, false, fmt.Errorf("decode edit journal: %w", err)
			}
			last, found = candidate, true
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return RecoveryRecord{}, false, fmt.Errorf("read edit journal: %w", readErr)
		}
	}
	return last, found, nil
}

func showStateDigest(current show.Show) string {
	// TODO(micro): handle json.Marshal error in showStateDigest instead of discarding with _
	raw, _ := json.Marshal(current)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
