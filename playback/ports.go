package playback

import (
	"context"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type ShowAccess interface {
	Snapshot() []show.Cue
	SelectedCueCopy() (show.Cue, int, bool)
	CueByIDCopy(show.CueID) (show.Cue, int, bool)
	SelectCue(int)
	DeselectCue()
}

type SettingsAccess interface {
	Snapshot() config.Settings
}

type RemoteCommands interface {
	DispatchWithResult(context.Context, show.RemotePlay, show.Cue) (remote.DispatchResult, error)
}

type RemoteHealthSource interface {
	Health() []remote.TargetHealth
}

type RemotePort interface {
	RemoteCommands
	RemoteHealthSource
}

type MediaOutput interface {
	Subscribe(string) (<-chan Event, func())
	OutputSnapshot(string) ([]Event, uint64)
	HandleOutputReport(string, string)
	HandleOutputDuration(string, int64)
	HandleOutputError(string, error)
}

type RuntimeQuery interface {
	ActiveInstances() []Instance
	ActiveExecutions() []CueExecution
	KnownDurations() map[show.CueID]int64
	OutputIDs() []string
	CueProblems(show.Cue) []show.CueProblem
	CueActive(show.CueID) bool
	LastError() string
	SafetyLatchReason() string
}

type OperatorControls interface {
	PlaySelected() error
	PlaySelectedOverride() error
	PlayIndex(int) error
	PlayCueID(show.CueID) error
	StopAll()
	BlackoutAll()
	ControlMedia(show.MediaTarget, show.MediaControlAction, *float64, *int64, int64) error
	FadeInstance(string) error
	FadeAll()
	EndInstance(string)
}

type RemoteHealthQuery interface {
	RemoteHealth() []remote.TargetHealth
}

var (
	_ ShowAccess        = (*show.ShowManager)(nil)
	_ SettingsAccess    = (*config.Store)(nil)
	_ RemotePort        = (*remote.Dispatcher)(nil)
	_ MediaOutput       = (*Engine)(nil)
	_ RuntimeQuery      = (*Engine)(nil)
	_ OperatorControls  = (*Engine)(nil)
	_ RemoteHealthQuery = (*Engine)(nil)
)
