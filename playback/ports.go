package playback

import (
	"context"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

// ShowAccess is the playback-facing view of the cue document and operator
// selection. Implementations retain ownership of both the document and its
// selection state.
type ShowAccess interface {
	Snapshot() []show.Cue
	SelectedCueCopy() (show.Cue, int, bool)
	CueByIDCopy(show.CueID) (show.Cue, int, bool)
	SelectCue(int)
	DeselectCue()
}

// SettingsAccess supplies immutable settings snapshots to playback policy.
type SettingsAccess interface {
	Snapshot() config.Settings
}

// RemoteCommands is the write-side port used by remote cue execution.
type RemoteCommands interface {
	DispatchWithResult(context.Context, show.RemotePlay, show.Cue) (remote.DispatchResult, error)
}

// RemoteHealthSource is the read-side port used by health and preflight views.
type RemoteHealthSource interface {
	Health() []remote.TargetHealth
}

// RemotePort is the usual combined implementation supplied by a composition
// root. Its lifecycle remains owned by the caller.
type RemotePort interface {
	RemoteCommands
	RemoteHealthSource
}

// MediaOutput is the narrow callback/event surface needed by media backends.
type MediaOutput interface {
	Subscribe(string) (<-chan Event, func())
	OutputSnapshot(string) ([]Event, uint64)
	HandleOutputReport(string, string)
	HandleOutputDuration(string, int64)
	HandleOutputError(string, error)
}

// RuntimeQuery exposes immutable playback state without operator mutations.
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

// OperatorControls is the command surface used by operator controls.
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

// RemoteHealthQuery lets health collectors depend on only remote status.
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
