package project

import "github.com/syspoe/cusus/project/internal/recovery"

type RecoveryRecordKind = recovery.RecordKind

const RecoveryRecordFullShowSnapshot = recovery.FullShowSnapshot

type RecoveryState = recovery.State

const (
	RecoveryStateDirty = recovery.StateDirty
	RecoveryStateSaved = recovery.StateSaved
)

type RecoverySnapshot = recovery.Snapshot

type RecoveryRecord = recovery.Record

type EditJournal = recovery.Journal

func OpenEditJournal(path string) (*EditJournal, error) {
	return recovery.Open(path)
}
