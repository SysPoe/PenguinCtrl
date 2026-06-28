package show

import "github.com/google/uuid"

type Cue struct {
	ID          CueID
	CueNumber   string
	Title       string
	Description string

	Type     CueType
	Disabled bool

	Timing CueTiming
	Play   CuePlay
	Link   CueLink

	HexColor string
	Tags     []string
	Notes    string
}

type CueID uuid.UUID

func NewCueID() CueID {
	id, _ := uuid.NewV7()
	return CueID(id)
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
	PreWaitMS  int64
	PostWaitMS int64
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
