package project

import (
	"bufio"
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
	journalVersion  = 2
	maxJournalBytes = 8 << 20
	maxRecordBytes  = 4 << 20
)

type RecoveryRecordKind string

const RecoveryRecordFullShowSnapshot RecoveryRecordKind = "full-show-snapshot"

type RecoveryState string

const (
	RecoveryStateDirty RecoveryState = "dirty"
	RecoveryStateSaved RecoveryState = "saved"
)

// RecoverySnapshot is a complete show generation. Recovery deliberately uses
// bounded full snapshots rather than deltas so every valid dirty record can be
// restored independently after a torn append or journal compaction.
type RecoverySnapshot struct {
	Show show.Show `json:"show"`
}

// RecoveryRecord is a flushed full-snapshot generation. Kind, State, and
// Snapshot are the durable version-2 contract. Show and Dirty are populated as
// compatibility views for callers of the version-1 API and are not serialized.
type RecoveryRecord struct {
	Version      int                `json:"version"`
	Kind         RecoveryRecordKind `json:"kind"`
	State        RecoveryState      `json:"state"`
	WrittenAt    time.Time          `json:"writtenAt"`
	DocumentPath string             `json:"documentPath,omitempty"`
	Digest       string             `json:"digest"`
	Snapshot     RecoverySnapshot   `json:"snapshot"`

	Dirty bool      `json:"-"`
	Show  show.Show `json:"-"`
}

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
	if err != nil || !found || record.State != RecoveryStateDirty {
		return record, false, err
	}
	digest, digestErr := showDigestHex(record.Snapshot.Show)
	if digestErr != nil {
		return RecoveryRecord{}, false, fmt.Errorf("digest recovered show: %w", digestErr)
	}
	if record.Version != journalVersion || record.Kind != RecoveryRecordFullShowSnapshot || record.State != RecoveryStateDirty || record.Digest != digest {
		return RecoveryRecord{}, false, errors.New("edit journal recovery record failed validation")
	}
	return record, true, nil
}

func (j *EditJournal) append(current show.Show, documentPath string, dirty bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	digest, err := showDigestHex(current)
	if err != nil {
		return fmt.Errorf("digest edit journal show: %w", err)
	}
	state := RecoveryStateSaved
	if dirty {
		state = RecoveryStateDirty
	}
	record := RecoveryRecord{
		Version: journalVersion, Kind: RecoveryRecordFullShowSnapshot, State: state,
		WrittenAt: time.Now().UTC(), DocumentPath: documentPath, Digest: digest,
		Snapshot: RecoverySnapshot{Show: current}, Dirty: dirty, Show: current,
	}
	raw, err := encodeRecoveryRecord(record)
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
			candidate, err := decodeRecoveryRecord(line)
			if err != nil {
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

type storedRecoveryRecord struct {
	Version      int                `json:"version"`
	Kind         RecoveryRecordKind `json:"kind"`
	State        RecoveryState      `json:"state"`
	WrittenAt    time.Time          `json:"writtenAt"`
	DocumentPath string             `json:"documentPath,omitempty"`
	Digest       string             `json:"digest"`
	Snapshot     RecoverySnapshot   `json:"snapshot"`
}

type legacyRecoveryRecord struct {
	Version      int       `json:"version"`
	WrittenAt    time.Time `json:"writtenAt"`
	DocumentPath string    `json:"documentPath,omitempty"`
	Digest       string    `json:"digest"`
	Dirty        bool      `json:"dirty"`
	Show         show.Show `json:"show"`
}

func encodeRecoveryRecord(record RecoveryRecord) ([]byte, error) {
	return json.Marshal(storedRecoveryRecord{
		Version: record.Version, Kind: record.Kind, State: record.State,
		WrittenAt: record.WrittenAt, DocumentPath: record.DocumentPath,
		Digest: record.Digest, Snapshot: record.Snapshot,
	})
}

func decodeRecoveryRecord(raw []byte) (RecoveryRecord, error) {
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return RecoveryRecord{}, err
	}
	switch envelope.Version {
	case 1:
		var legacy legacyRecoveryRecord
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return RecoveryRecord{}, err
		}
		state := RecoveryStateSaved
		if legacy.Dirty {
			state = RecoveryStateDirty
		}
		return RecoveryRecord{
			Version: journalVersion, Kind: RecoveryRecordFullShowSnapshot, State: state,
			WrittenAt: legacy.WrittenAt, DocumentPath: legacy.DocumentPath,
			Digest: legacy.Digest, Snapshot: RecoverySnapshot{Show: legacy.Show},
			Dirty: legacy.Dirty, Show: legacy.Show,
		}, nil
	case journalVersion:
		var stored storedRecoveryRecord
		if err := json.Unmarshal(raw, &stored); err != nil {
			return RecoveryRecord{}, err
		}
		if stored.Kind != RecoveryRecordFullShowSnapshot {
			return RecoveryRecord{}, fmt.Errorf("unsupported edit journal record kind %q", stored.Kind)
		}
		if stored.State != RecoveryStateDirty && stored.State != RecoveryStateSaved {
			return RecoveryRecord{}, fmt.Errorf("unsupported edit journal state %q", stored.State)
		}
		return RecoveryRecord{
			Version: stored.Version, Kind: stored.Kind, State: stored.State,
			WrittenAt: stored.WrittenAt, DocumentPath: stored.DocumentPath,
			Digest: stored.Digest, Snapshot: stored.Snapshot,
			Dirty: stored.State == RecoveryStateDirty, Show: stored.Snapshot.Show,
		}, nil
	default:
		return RecoveryRecord{}, fmt.Errorf("unsupported edit journal version %d", envelope.Version)
	}
}

func showDigestHex(current show.Show) (string, error) {
	digest, err := current.Digest()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest[:]), nil
}
