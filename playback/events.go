package playback

import (
	"time"

	"github.com/syspoe/cusus/show"
)

// Media type values are shared by playback snapshots and media backends.
const (
	MediaTypeAudio = "audio"
	MediaTypeVideo = "video"
	MediaTypeImage = "image"
)

// Instance is a detached public snapshot of one live media item. Engine
// lifecycle bookkeeping lives separately in liveInstance.
type Instance struct {
	ID             string       `json:"id"`
	CueID          show.CueID   `json:"cueId"`
	GroupID        show.GroupID `json:"groupId,omitempty"`
	CueNumber      string       `json:"cueNumber"`
	LayerOrder     uint64       `json:"layerOrder,omitempty"`
	OutputID       string       `json:"outputId"`
	Preview        bool         `json:"preview,omitempty"`
	MediaType      string       `json:"mediaType"`
	Source         string       `json:"source"`
	ClipStartMs    int64        `json:"clipStartMs"`
	ClipEndMs      int64        `json:"clipEndMs"`
	FadeInMs       int64        `json:"fadeInMs"`
	FadeOutMs      int64        `json:"fadeOutMs"`
	DurationMs     int64        `json:"durationMs"`
	LevelDB        float64      `json:"levelDb"`
	Paused         bool         `json:"paused"`
	Muted          bool         `json:"muted"`
	PositionMs     int64        `json:"positionMs"`
	FadeInComplete bool         `json:"-"`
	StartedAt      time.Time    `json:"-"`
	BackendStarted bool         `json:"-"`
	Presented      bool         `json:"-"`
	LoadState      string       `json:"loadState,omitempty"`
	StartLatencyMs int64        `json:"startLatencyMs,omitempty"`
}

// liveInstance is the engine-owned mutable record. Embedding the public state
// keeps state transitions compact while snapshots copy only Instance.
type liveInstance struct {
	Instance
	fadeOutStarted bool
	endScheduled   bool
	cueIndex       int
	link           show.CueLink
	postWaitMs     int64
	run            cueRunToken
	positionAt     time.Time
	fadeStartedAt  time.Time
	fadeStartDB    float64
	fadeTargetDB   float64
	fadeDurationMs int64
	requestedAt    time.Time
	// ReplacementScheduled prevents rapid successive visual starts from
	// restarting an outgoing layer's configured fade.
	replacementScheduled bool
	// LifecycleGeneration invalidates stale fade/end timers after pause, seek,
	// duration correction, or resume. Timers must never act on another
	// generation of the same logical playback instance.
	lifecycleGeneration uint64
	cue                 show.Cue
}

// MediaSnapshot is the immutable wire state required by output consumers.
// It deliberately excludes cue-run ownership, timer generations, and other
// mutable engine lifecycle fields.
type MediaSnapshot struct {
	ID             string       `json:"id"`
	CueID          show.CueID   `json:"cueId"`
	GroupID        show.GroupID `json:"groupId,omitempty"`
	CueNumber      string       `json:"cueNumber"`
	LayerOrder     uint64       `json:"layerOrder,omitempty"`
	OutputID       string       `json:"outputId"`
	Preview        bool         `json:"preview,omitempty"`
	MediaType      string       `json:"mediaType"`
	Source         string       `json:"source"`
	ClipStartMs    int64        `json:"clipStartMs"`
	ClipEndMs      int64        `json:"clipEndMs"`
	FadeInMs       int64        `json:"fadeInMs"`
	FadeOutMs      int64        `json:"fadeOutMs"`
	DurationMs     int64        `json:"durationMs"`
	LevelDB        float64      `json:"levelDb"`
	Paused         bool         `json:"paused"`
	Muted          bool         `json:"muted"`
	PositionMs     int64        `json:"positionMs"`
	LoadState      string       `json:"loadState,omitempty"`
	StartLatencyMs int64        `json:"startLatencyMs,omitempty"`
}

func snapshotMedia(instance Instance) MediaSnapshot {
	return MediaSnapshot{
		ID: instance.ID, CueID: instance.CueID, GroupID: instance.GroupID, CueNumber: instance.CueNumber,
		LayerOrder: instance.LayerOrder, OutputID: instance.OutputID, Preview: instance.Preview,
		MediaType: instance.MediaType, Source: instance.Source, ClipStartMs: instance.ClipStartMs,
		ClipEndMs: instance.ClipEndMs, FadeInMs: instance.FadeInMs, FadeOutMs: instance.FadeOutMs,
		DurationMs: instance.DurationMs, LevelDB: instance.LevelDB, Paused: instance.Paused,
		Muted: instance.Muted, PositionMs: instance.PositionMs, LoadState: instance.LoadState,
		StartLatencyMs: instance.StartLatencyMs,
	}
}

func (snapshot MediaSnapshot) compatibilityInstance() Instance {
	return Instance{
		ID: snapshot.ID, CueID: snapshot.CueID, GroupID: snapshot.GroupID, CueNumber: snapshot.CueNumber,
		LayerOrder: snapshot.LayerOrder, OutputID: snapshot.OutputID, Preview: snapshot.Preview,
		MediaType: snapshot.MediaType, Source: snapshot.Source, ClipStartMs: snapshot.ClipStartMs,
		ClipEndMs: snapshot.ClipEndMs, FadeInMs: snapshot.FadeInMs, FadeOutMs: snapshot.FadeOutMs,
		DurationMs: snapshot.DurationMs, LevelDB: snapshot.LevelDB, Paused: snapshot.Paused,
		Muted: snapshot.Muted, PositionMs: snapshot.PositionMs, LoadState: snapshot.LoadState,
		StartLatencyMs: snapshot.StartLatencyMs,
	}
}

// CueExecution describes a cue that is currently doing synchronous work.
// Media cues continue to be represented by Instance after their start action
// completes; this type makes pre/post waits, wait cues, and other blocking
// actions observable to the cue-list UI as well.
type CueExecution struct {
	ID          string
	CueID       show.CueID
	GroupID     show.GroupID
	CueIndex    int
	CueType     show.CueType
	Phase       string
	StartedAt   time.Time
	PhaseAt     time.Time
	DurationMs  int64
	ElapsedMs   int64
	RemainingMs int64
}

// CommandRecord is the durable-in-memory audit trail for accepted playback
// work. Sequence order is the order in which commands are allowed to begin
// their cue action, regardless of goroutine scheduling.
type CommandRecord struct {
	Sequence     uint64
	CueID        show.CueID
	CueNumber    string
	Origin       string
	Preview      bool
	AcceptedAt   time.Time
	DispatchedAt time.Time
	CompletedAt  time.Time
}

type OutputEventKind string

const (
	OutputEventPlay    OutputEventKind = "play"
	OutputEventControl OutputEventKind = "control"
	OutputEventRemove  OutputEventKind = "remove"
	OutputEventSync    OutputEventKind = "sync"
	OutputEventResync  OutputEventKind = "resync"
	OutputEventError   OutputEventKind = "error"
	OutputEventOutput  OutputEventKind = "output"
)

// Event is the flattened compatibility DTO exposed by Subscribe. Playback
// produces it only from the closed outputEvent variant set below; Action and
// the optional legacy fields remain for existing media/output consumers.
type Event struct {
	Kind        OutputEventKind `json:"-"`
	Sequence    uint64          `json:"sequence,omitempty"`
	Action      string          `json:"action"`
	OutputID    string          `json:"outputId,omitempty"`
	Instance    *Instance       `json:"instance,omitempty"`
	Instances   []Instance      `json:"instances,omitempty"`
	InstanceIDs []string        `json:"instanceIds,omitempty"`
	Control     string          `json:"control,omitempty"`
	FadeMs      int64           `json:"fadeMs,omitempty"`
	FadeOutMs   int64           `json:"fadeOutMs,omitempty"`
	FadeInMs    int64           `json:"fadeInMs,omitempty"`
	LevelDB     *float64        `json:"levelDb,omitempty"`
	PositionMs  *int64          `json:"positionMs,omitempty"`
	Message     string          `json:"message,omitempty"`
	Curve       show.FadeCurve  `json:"curve,omitempty"`
	Error       string          `json:"error,omitempty"`
}

func (event Event) OutputKind() OutputEventKind {
	if event.Kind != "" {
		return event.Kind
	}
	return OutputEventKind(event.Action)
}

type mediaCommand string

const (
	mediaCommandFadeOut   mediaCommand = "fade-out"
	mediaCommandStop      mediaCommand = "stop"
	mediaCommandStopAll   mediaCommand = "stop-all"
	mediaCommandPause     mediaCommand = "pause"
	mediaCommandResume    mediaCommand = "resume"
	mediaCommandSeek      mediaCommand = "seek"
	mediaCommandFadeTo    mediaCommand = "fade-to"
	mediaCommandSetVolume mediaCommand = "set-volume"
	mediaCommandMute      mediaCommand = "mute"
	mediaCommandUnmute    mediaCommand = "unmute"
)

type outputCommand string

const (
	outputCommandBlackout       outputCommand = "blackout"
	outputCommandClear          outputCommand = "clear"
	outputCommandTestPattern    outputCommand = "test-pattern"
	outputCommandIdentify       outputCommand = "identify"
	outputCommandReopen         outputCommand = "reopen"
	outputCommandFullscreen     outputCommand = "fullscreen"
	outputCommandExitFullscreen outputCommand = "exit-fullscreen"
)

type outputEvent interface {
	compatibilityEvent() Event
}

type playOutputEvent struct {
	outputID string
	instance MediaSnapshot
}

func (event playOutputEvent) compatibilityEvent() Event {
	instance := event.instance.compatibilityInstance()
	return Event{Kind: OutputEventPlay, Action: string(OutputEventPlay), OutputID: event.outputID, Instance: &instance}
}

type mediaControlOutputEvent struct {
	outputID    string
	instanceIDs []string
	command     mediaCommand
	fadeMs      int64
	levelDB     *float64
	positionMs  *int64
	curve       show.FadeCurve
}

func (event mediaControlOutputEvent) compatibilityEvent() Event {
	return Event{
		Kind: OutputEventControl, Action: string(OutputEventControl), OutputID: event.outputID,
		InstanceIDs: append([]string(nil), event.instanceIDs...), Control: string(event.command),
		FadeMs: event.fadeMs, LevelDB: event.levelDB, PositionMs: event.positionMs, Curve: event.curve,
	}
}

type removeOutputEvent struct {
	outputID    string
	instanceIDs []string
}

func (event removeOutputEvent) compatibilityEvent() Event {
	return Event{
		Kind: OutputEventRemove, Action: string(OutputEventRemove), OutputID: event.outputID,
		InstanceIDs: append([]string(nil), event.instanceIDs...),
	}
}

type syncOutputEvent struct {
	outputID  string
	instances []MediaSnapshot
}

func (event syncOutputEvent) compatibilityEvent() Event {
	instances := make([]Instance, 0, len(event.instances))
	for _, snapshot := range event.instances {
		instances = append(instances, snapshot.compatibilityInstance())
	}
	return Event{Kind: OutputEventSync, Action: string(OutputEventSync), OutputID: event.outputID, Instances: instances}
}

type resyncOutputEvent struct {
	outputID string
}

func (event resyncOutputEvent) compatibilityEvent() Event {
	return Event{Kind: OutputEventResync, Action: string(OutputEventResync), OutputID: event.outputID}
}

type errorOutputEvent struct {
	outputID string
	err      string
}

func (event errorOutputEvent) compatibilityEvent() Event {
	return Event{Kind: OutputEventError, Action: string(OutputEventError), OutputID: event.outputID, Error: event.err}
}

type outputControlOutputEvent struct {
	outputID  string
	command   outputCommand
	fadeOutMs int64
	fadeInMs  int64
	message   string
}

func (event outputControlOutputEvent) compatibilityEvent() Event {
	return Event{
		Kind: OutputEventOutput, Action: string(OutputEventOutput), OutputID: event.outputID,
		Control: string(event.command), FadeOutMs: event.fadeOutMs, FadeInMs: event.fadeInMs, Message: event.message,
	}
}
