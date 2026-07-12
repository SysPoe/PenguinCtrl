package show

import (
	"image/color"

	"github.com/google/uuid"
)

type Cue struct {
	ID          CueID
	CueNumber   string
	Description string
	GroupID     GroupID
	GroupTitle  string

	Type     CueType
	Disabled bool

	Timing CueTiming
	Play   CuePlay
	Link   CueLink

	Color color.NRGBA
	Tags  []string
	Notes string
}

type CueID uuid.UUID

// GroupID identifies a visual and operational cue group. Group membership is
// stored on each cue so older show files remain valid and empty groups cannot
// become orphaned from their cues.
type GroupID uuid.UUID

func NewCueID() CueID {
	id, _ := uuid.NewV7()
	return CueID(id)
}

func NewGroupID() GroupID {
	id, _ := uuid.NewV7()
	return GroupID(id)
}

type CueType int

const (
	CueTypeSound CueType = iota
	CueTypeVideo
	CueTypeImage
	CueTypeRemote
	CueTypeWait
	CueTypeMediaControl
	CueTypeOutputControl
)

// Exactly one of these should be non-empty
type CuePlay struct {
	Sound         *SoundPlay
	Video         *VideoPlay
	Image         *ImagePlay
	Remote        *RemotePlay
	Wait          *WaitPlay
	MediaControl  *MediaControlPlay
	OutputControl *OutputControlPlay
}

type CueTiming struct {
	PreWaitMs  int64
	PostWaitMs int64
}

type CueLink struct {
	Mode   CueLinkMode
	Target CueTarget
}

type CueLinkMode int

const (
	CueLinkManual CueLinkMode = iota // Do nothing

	// On cue trigger
	CueLinkStartAdvance
	CueLinkStartPlay

	// On end of fade in (media only)
	CueLinkFadeInAdvance
	CueLinkFadeInPlay

	// On start of fade out (media only)
	CueLinkFadeOutAdvance
	CueLinkFadeOutPlay

	// On end of track (media only)
	CueLinkEndAdvance
	CueLinkEndPlay
)

type CueTargetKind int

const (
	CueTargetNone CueTargetKind = iota
	CueTargetNext
	CueTargetPrevious
	CueTargetCue
)

type CueTarget struct {
	Kind  CueTargetKind
	CueID CueID
}
