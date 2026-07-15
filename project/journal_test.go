package project

import (
	"encoding/json"
	"os"
	"path/filepath"
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
