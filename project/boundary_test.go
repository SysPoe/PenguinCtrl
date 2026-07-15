package project

import (
	"path/filepath"
	"testing"

	projectcache "github.com/syspoe/cusus/project/internal/cache"
	"github.com/syspoe/cusus/project/internal/recovery"
	"github.com/syspoe/cusus/show"
)

func TestCacheFacadeMatchesInternalOwner(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-cache")
	facade := cacheLayoutAt(userRoot)
	owned := projectcache.LayoutAt(userRoot)
	if facade.Root != owned.Root || facade.Shows != owned.Shows || facade.Transcoded != owned.Transcoded {
		t.Fatalf("cache facade = %#v; internal owner = %#v", facade, owned)
	}
	protected := filepath.Join(owned.Shows, "active", "media", "cue.opus")
	if cacheObjectProtected(owned.Shows, []string{protected}) != projectcache.ObjectProtected(owned.Shows, []string{protected}) {
		t.Fatal("cache protection facade diverged from internal owner")
	}
}

func TestRecoveryFacadeAliasesInternalOwner(t *testing.T) {
	journal, err := OpenEditJournal(filepath.Join(t.TempDir(), "recovery.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var owned *recovery.Journal = journal
	current := show.Show{Title: "facade boundary"}
	if err := owned.RecordDirty(current, "show.cusus"); err != nil {
		t.Fatal(err)
	}
	record, ok, err := journal.Recover()
	if err != nil || !ok {
		t.Fatalf("Recover() = %#v, %v, %v", record, ok, err)
	}
	var ownedRecord recovery.Record = record
	if ownedRecord.Kind != recovery.FullShowSnapshot || record.Kind != RecoveryRecordFullShowSnapshot || record.State != RecoveryStateDirty {
		t.Fatalf("recovery facade identity = %#v", record)
	}
}
