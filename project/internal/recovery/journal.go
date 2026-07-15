package recovery

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
	Version         = 2
	maxJournalBytes = 8 << 20
	maxRecordBytes  = 4 << 20
)

type RecordKind string

const FullShowSnapshot RecordKind = "full-show-snapshot"

type State string

const (
	StateDirty State = "dirty"
	StateSaved State = "saved"
)

type Snapshot struct {
	Show show.Show `json:"show"`
}

type Record struct {
	Version      int        `json:"version"`
	Kind         RecordKind `json:"kind"`
	State        State      `json:"state"`
	WrittenAt    time.Time  `json:"writtenAt"`
	DocumentPath string     `json:"documentPath,omitempty"`
	Digest       string     `json:"digest"`
	Snapshot     Snapshot   `json:"snapshot"`

	Dirty bool      `json:"-"`
	Show  show.Show `json:"-"`
}

type Journal struct {
	mu   sync.Mutex
	path string
}

func Open(path string) (*Journal, error) {
	if path == "" {
		return nil, errors.New("edit journal path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create edit journal directory: %w", err)
	}
	if err := atomicfile.Recover(path); err != nil {
		return nil, fmt.Errorf("recover edit journal: %w", err)
	}
	return &Journal{path: path}, nil
}

func (journal *Journal) RecordDirty(current show.Show, documentPath string) error {
	return journal.append(current, documentPath, true)
}

func (journal *Journal) MarkSaved(current show.Show, documentPath string) error {
	return journal.append(current, documentPath, false)
}

func (journal *Journal) Recover() (Record, bool, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	record, found, err := readLastRecord(journal.path)
	if err != nil || !found || record.State != StateDirty {
		return record, false, err
	}
	digest, digestErr := showDigestHex(record.Snapshot.Show)
	if digestErr != nil {
		return Record{}, false, fmt.Errorf("digest recovered show: %w", digestErr)
	}
	if record.Version != Version || record.Kind != FullShowSnapshot || record.State != StateDirty || record.Digest != digest {
		return Record{}, false, errors.New("edit journal recovery record failed validation")
	}
	return record, true, nil
}

func (journal *Journal) append(current show.Show, documentPath string, dirty bool) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	digest, err := showDigestHex(current)
	if err != nil {
		return fmt.Errorf("digest edit journal show: %w", err)
	}
	state := StateSaved
	if dirty {
		state = StateDirty
	}
	record := Record{
		Version: Version, Kind: FullShowSnapshot, State: state,
		WrittenAt: time.Now().UTC(), DocumentPath: documentPath, Digest: digest,
		Snapshot: Snapshot{Show: current}, Dirty: dirty, Show: current,
	}
	raw, err := encodeRecord(record)
	if err != nil {
		return fmt.Errorf("encode edit journal: %w", err)
	}
	if len(raw) > maxRecordBytes {
		return fmt.Errorf("edit journal record is %d bytes; limit is %d", len(raw), maxRecordBytes)
	}
	if info, err := os.Stat(journal.path); err == nil && info.Size()+int64(len(raw)+1) > maxJournalBytes {
		return atomicfile.Write(journal.path, append(raw, '\n'), 0o600)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect edit journal: %w", err)
	}
	file, err := os.OpenFile(journal.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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

func readLastRecord(path string) (Record, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("open edit journal: %w", err)
	}
	defer func() { _ = file.Close() }()
	reader := bufio.NewReaderSize(file, 64*1024)
	var last Record
	found := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxRecordBytes+1 {
			return Record{}, false, errors.New("edit journal contains an oversized record")
		}
		if len(line) > 0 {
			candidate, err := decodeRecord(line)
			if err != nil {
				if readErr == io.EOF {
					break
				}
				return Record{}, false, fmt.Errorf("decode edit journal: %w", err)
			}
			last, found = candidate, true
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Record{}, false, fmt.Errorf("read edit journal: %w", readErr)
		}
	}
	return last, found, nil
}

type storedRecord struct {
	Version      int        `json:"version"`
	Kind         RecordKind `json:"kind"`
	State        State      `json:"state"`
	WrittenAt    time.Time  `json:"writtenAt"`
	DocumentPath string     `json:"documentPath,omitempty"`
	Digest       string     `json:"digest"`
	Snapshot     Snapshot   `json:"snapshot"`
}

type legacyRecord struct {
	Version      int       `json:"version"`
	WrittenAt    time.Time `json:"writtenAt"`
	DocumentPath string    `json:"documentPath,omitempty"`
	Digest       string    `json:"digest"`
	Dirty        bool      `json:"dirty"`
	Show         show.Show `json:"show"`
}

func encodeRecord(record Record) ([]byte, error) {
	return json.Marshal(storedRecord{
		Version: record.Version, Kind: record.Kind, State: record.State,
		WrittenAt: record.WrittenAt, DocumentPath: record.DocumentPath,
		Digest: record.Digest, Snapshot: record.Snapshot,
	})
}

func decodeRecord(raw []byte) (Record, error) {
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Record{}, err
	}
	switch envelope.Version {
	case 1:
		var legacy legacyRecord
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return Record{}, err
		}
		state := StateSaved
		if legacy.Dirty {
			state = StateDirty
		}
		return Record{
			Version: Version, Kind: FullShowSnapshot, State: state,
			WrittenAt: legacy.WrittenAt, DocumentPath: legacy.DocumentPath,
			Digest: legacy.Digest, Snapshot: Snapshot{Show: legacy.Show},
			Dirty: legacy.Dirty, Show: legacy.Show,
		}, nil
	case Version:
		var stored storedRecord
		if err := json.Unmarshal(raw, &stored); err != nil {
			return Record{}, err
		}
		if stored.Kind != FullShowSnapshot {
			return Record{}, fmt.Errorf("unsupported edit journal record kind %q", stored.Kind)
		}
		if stored.State != StateDirty && stored.State != StateSaved {
			return Record{}, fmt.Errorf("unsupported edit journal state %q", stored.State)
		}
		return Record{
			Version: stored.Version, Kind: stored.Kind, State: stored.State,
			WrittenAt: stored.WrittenAt, DocumentPath: stored.DocumentPath,
			Digest: stored.Digest, Snapshot: stored.Snapshot,
			Dirty: stored.State == StateDirty, Show: stored.Snapshot.Show,
		}, nil
	default:
		return Record{}, fmt.Errorf("unsupported edit journal version %d", envelope.Version)
	}
}

func showDigestHex(current show.Show) (string, error) {
	digest, err := current.Digest()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest[:]), nil
}
