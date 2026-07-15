package show

import (
	"image/color"

	"github.com/google/uuid"
)

// Cue keeps the historical type-and-play JSON shape for archive compatibility.
// Constructors and ShowManager mutations canonicalize that wire representation
// through RepairCueData, so runtime cues always contain exactly one payload arm
// matching Type.
type Cue struct {
	ID          CueID   `json:"id"`
	CueNumber   string  `json:"cueNumber"`
	Description string  `json:"description,omitempty"`
	GroupID     GroupID `json:"groupId,omitempty"`
	GroupTitle  string  `json:"groupTitle,omitempty"`

	Type CueType `json:"type"`

	Timing CueTiming `json:"timing"`
	Play   CuePlay   `json:"play"`
	Link   CueLink   `json:"link"`

	Color color.NRGBA `json:"color,omitempty"`
	Tags  []string    `json:"tags,omitempty"`
	Notes string      `json:"notes,omitempty"`
}

type CueID uuid.UUID

// GroupID identifies a visual and operational cue group. Group membership is
// stored on each cue so older show files remain valid and empty groups cannot
// become orphaned from their cues.
type GroupID uuid.UUID

func NewCueID() CueID {
	return CueID(uuid.Must(uuid.NewV7()))
}

func NewGroupID() GroupID {
	return GroupID(uuid.Must(uuid.NewV7()))
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

// CuePlay is the archive-compatible tagged payload. Exactly one field is
// populated in canonical runtime data; Type and ForType are the central access
// layer used by construction, repair, warnings, cloning, and persistence.
type CuePlay struct {
	Sound         *SoundPlay         `json:"sound,omitempty"`
	Video         *VideoPlay         `json:"video,omitempty"`
	Image         *ImagePlay         `json:"image,omitempty"`
	Remote        *RemotePlay        `json:"remote,omitempty"`
	Wait          *WaitPlay          `json:"wait,omitempty"`
	MediaControl  *MediaControlPlay  `json:"mediaControl,omitempty"`
	OutputControl *OutputControlPlay `json:"outputControl,omitempty"`
}

type CueTiming struct {
	PreWaitMs  int64 `json:"preWaitMs,omitempty"`
	PostWaitMs int64 `json:"postWaitMs,omitempty"`
}

type CueLink struct {
	Mode   CueLinkMode `json:"mode"`
	Target CueTarget   `json:"target"`
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
	Kind  CueTargetKind `json:"kind"`
	CueID CueID         `json:"cueId,omitempty"`
}
