package project

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestEditJournalRecoversLatestDirtyGeneration(t *testing.T) {
	journal, err := OpenEditJournal(filepath.Join(t.TempDir(), "recovery.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := show.Show{Title: "first"}
	second := show.Show{Title: "second", Cues: []show.Cue{show.NewSoundCue()}}
	if err := journal.RecordDirty(first, "first.cusus"); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordDirty(second, "second.cusus"); err != nil {
		t.Fatal(err)
	}
	recovered, ok, err := journal.Recover()
	if err != nil || !ok {
		t.Fatalf("Recover() = %#v, %v, %v", recovered, ok, err)
	}
	if recovered.Show.Title != "second" || recovered.DocumentPath != "second.cusus" || len(recovered.Show.Cues) != 1 {
		t.Fatalf("recovered wrong generation: %#v", recovered)
	}
}

func TestEditJournalRejectsUndigestibleShow(t *testing.T) {
	journal, err := OpenEditJournal(filepath.Join(t.TempDir(), "recovery.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	current := show.Show{Extensions: map[string]json.RawMessage{"broken": json.RawMessage(`{`)}}
	if err := journal.RecordDirty(current, "show.cusus"); err == nil {
		t.Fatal("invalid show JSON unexpectedly produced a journal digest")
	}
}

func TestEditJournalSavedGenerationIsNotOffered(t *testing.T) {
	journal, err := OpenEditJournal(filepath.Join(t.TempDir(), "recovery.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	current := show.Show{Title: "saved"}
	if err := journal.RecordDirty(current, "show.cusus"); err != nil {
		t.Fatal(err)
	}
	if err := journal.MarkSaved(current, "show.cusus"); err != nil {
		t.Fatal(err)
	}
	if recovered, ok, err := journal.Recover(); err != nil || ok {
		t.Fatalf("Recover() = %#v, %v, %v; want no dirty recovery", recovered, ok, err)
	}
}

func TestEditJournalIgnoresTornFinalAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.jsonl")
	journal, err := OpenEditJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordDirty(show.Show{Title: "safe"}, "show.cusus"); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"version":1,"show":`)
	_ = file.Close()
	recovered, ok, err := journal.Recover()
	if err != nil || !ok || recovered.Show.Title != "safe" {
		t.Fatalf("Recover() = %#v, %v, %v", recovered, ok, err)
	}
}

func TestEditJournalWritesTypedFullSnapshotRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.jsonl")
	journal, err := OpenEditJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	current := show.Show{Title: "typed snapshot", Cues: []show.Cue{show.NewVideoCue()}}
	if err := journal.RecordDirty(current, "typed.cusus"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &fields); err != nil {
		t.Fatal(err)
	}
	if _, found := fields["dirty"]; found {
		t.Fatalf("version-2 record leaked version-1 dirty field: %s", raw)
	}
	if _, found := fields["show"]; found {
		t.Fatalf("version-2 record leaked version-1 show field: %s", raw)
	}
	var stored storedRecoveryRecord
	if err := json.Unmarshal(bytes.TrimSpace(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Version != journalVersion || stored.Kind != RecoveryRecordFullShowSnapshot || stored.State != RecoveryStateDirty {
		t.Fatalf("typed record identity = version %d, kind %q, state %q", stored.Version, stored.Kind, stored.State)
	}
	if stored.Snapshot.Show.Title != current.Title || stored.DocumentPath != "typed.cusus" {
		t.Fatalf("stored snapshot = %#v", stored)
	}
	digest, err := current.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if want := hex.EncodeToString(digest[:]); stored.Digest != want {
		t.Fatalf("stored digest = %q; want Show.Digest %q", stored.Digest, want)
	}
}

func TestEditJournalMigratesVersionOneFullSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.jsonl")
	current := show.Show{Title: "legacy recovery"}
	digest, err := current.Digest()
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyRecoveryRecord{
		Version: 1, DocumentPath: "legacy.cusus", Digest: hex.EncodeToString(digest[:]),
		Dirty: true, Show: current,
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenEditJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, ok, err := journal.Recover()
	if err != nil || !ok {
		t.Fatalf("Recover(version 1) = %#v, %v, %v", recovered, ok, err)
	}
	if recovered.Version != journalVersion || recovered.Kind != RecoveryRecordFullShowSnapshot || recovered.State != RecoveryStateDirty {
		t.Fatalf("migrated record identity = version %d, kind %q, state %q", recovered.Version, recovered.Kind, recovered.State)
	}
	if !recovered.Dirty || recovered.Show.Title != current.Title || recovered.Snapshot.Show.Title != current.Title {
		t.Fatalf("migrated compatibility/snapshot views = %#v", recovered)
	}
}

func TestEditJournalRejectsDeltaRecordKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.jsonl")
	current := show.Show{Title: "not a delta"}
	digest, err := current.Digest()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(storedRecoveryRecord{
		Version: journalVersion, Kind: RecoveryRecordKind("show-delta"), State: RecoveryStateDirty,
		Digest: hex.EncodeToString(digest[:]), Snapshot: RecoverySnapshot{Show: current},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenEditJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := journal.Recover(); err == nil || !strings.Contains(err.Error(), "unsupported edit journal record kind") {
		t.Fatalf("Recover(delta) error = %v; want unsupported kind", err)
	}
}
