package ui

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

type cueEditPageState struct {
	initialized bool
	cueID       show.CueID
	list        layout.List

	general       cueGeneralInputs
	timing        cueTimingInputs
	link          cueLinkInputs
	media         *mediaPlayInputs
	remote        *cueRemoteInputs
	wait          *cueWaitInputs
	mediaControl  *cueMediaControlInputs
	outputControl *cueOutputControlInputs
	markers       []timecodeMarkerInputs
}

type cueGeneralInputs struct {
	cueNumber   *input.Text
	description *input.Multiline
	color       *input.ColourPicker
	tags        *input.Text
	notes       *input.Multiline
}

type cueTimingInputs struct {
	preWaitMs  *input.Integer
	postWaitMs *input.Integer
}

type cueLinkInputs struct {
	mode       *input.Dropdown
	targetKind *input.Dropdown
	targetCue  *input.Dropdown
}

type cueRemoteInputs struct {
	protocol  *input.Dropdown
	action    *input.Dropdown
	playback  *input.Text
	cueNumber *input.Text
	level     *input.Text
	custom    *input.Text
}

func newCueRemoteInputs(play *show.RemotePlay) *cueRemoteInputs {
	return &cueRemoteInputs{
		protocol:  newEnumDropdown(remoteProtocolLabels, int(play.Protocol)),
		action:    newEnumDropdown(remoteActionLabels, int(play.Action)),
		playback:  input.NewText("Playback", play.Playback),
		cueNumber: input.NewText("Cue Number", play.CueNumber),
		level:     input.NewText("Level", play.Level),
		custom:    input.NewText("Custom Command", play.Custom),
	}
}

type cueMediaTargetInputs struct {
	kind       *input.Dropdown
	instanceID *input.Text
	outputID   *input.Text
	cue        *input.Dropdown
	group      *input.Dropdown
}

func newCueMediaTargetInputs(target show.MediaTarget) cueMediaTargetInputs {
	return cueMediaTargetInputs{
		kind:       newEnumDropdown(mediaTargetKindLabels, int(target.Kind)),
		instanceID: input.NewText("Instance ID", target.InstanceID),
		outputID:   input.NewText("Output ID", target.OutputID),
	}
}

type cueWaitInputs struct {
	kind       *input.Dropdown
	durationMs *input.Integer
	target     cueMediaTargetInputs
}

type cueMediaControlInputs struct {
	action   *input.Dropdown
	target   cueMediaTargetInputs
	levelDB  *input.Float
	seekToMs *input.Integer
	fadeMs   *input.Integer
	curve    *input.Dropdown
}

func newCueMediaControlInputs(play *show.MediaControlPlay) *cueMediaControlInputs {
	levelDB := 0.0
	if play.LevelDB != nil {
		levelDB = *play.LevelDB
	}
	seekToMs := int64(0)
	if play.SeekToMs != nil {
		seekToMs = *play.SeekToMs
	}
	return &cueMediaControlInputs{
		action:   newEnumDropdown(mediaControlActionLabels, int(play.Action)),
		target:   newCueMediaTargetInputs(play.Target),
		levelDB:  input.NewFloat("Level dB", levelDB),
		seekToMs: input.NewInteger("Seek To MS", int(seekToMs)),
		fadeMs:   input.NewInteger("Fade MS", int(play.FadeMs)),
		curve:    newEnumDropdown(fadeCurveLabels, int(play.Curve)),
	}
}

type cueOutputControlInputs struct {
	action    *input.Dropdown
	outputID  *input.Text
	fadeOutMs *input.Integer
	fadeInMs  *input.Integer
	message   *input.Text
}

func newCueOutputControlInputs(play *show.OutputControlPlay) *cueOutputControlInputs {
	return &cueOutputControlInputs{
		action:    newEnumDropdown(outputControlActionLabels, int(play.Action)),
		outputID:  input.NewText("Output ID", play.OutputID),
		fadeOutMs: input.NewInteger("Fade Out MS", int(play.FadeOutMs)),
		fadeInMs:  input.NewInteger("Fade In MS", int(play.FadeInMs)),
		message:   input.NewText("Message", play.Message),
	}
}

type timecodeMarkerInputs struct {
	time          *input.Integer
	disabled      *input.Checkbox
	actionType    *input.Dropdown
	delete        *widget.Clickable
	mediaControl  *cueMediaControlInputs
	outputControl *cueOutputControlInputs
	remote        *cueRemoteInputs
}

func newTimecodeMarkerInputs(marker show.TimecodeMarker) timecodeMarkerInputs {
	media := marker.Action.MediaControl
	if media == nil {
		media = defaultTimecodeMediaControl()
	}
	output := marker.Action.OutputControl
	if output == nil {
		output = show.NewOutputControlCue().Play.OutputControl
	}
	remote := marker.Action.Remote
	if remote == nil {
		remote = show.NewRemoteCue().Play.Remote
	}
	return timecodeMarkerInputs{
		time:          input.NewInteger("Time MS", int(marker.TimeMs)),
		disabled:      input.NewCheckbox("Disabled", marker.Disabled),
		actionType:    newEnumDropdown(timecodeActionLabels, timecodeActionIndex(marker.Type)),
		delete:        new(widget.Clickable),
		mediaControl:  newCueMediaControlInputs(media),
		outputControl: newCueOutputControlInputs(output),
		remote:        newCueRemoteInputs(remote),
	}
}

// mediaPlayInputs is the compile-checked widget model shared by the Media and
// Timecode tabs. A cue has at most one media payload, so these fields do not
// need cue-type-prefixed string keys.
type mediaPlayInputs struct {
	file        *input.Text
	projectFile *input.Dropdown
	fileBrowse  *widget.Clickable
	outputID    *input.Text
	clipStartMs *input.Integer
	clipEndMs   *input.Integer
	fadeInMs    *input.Integer
	fadeOutMs   *input.Integer
	durationMs  *input.Integer
	levelDB     *input.Float
}

func newTimedMediaPlayInputs(file, outputID string, clipStartMs, clipEndMs, fadeInMs, fadeOutMs int64, levelDB float64) *mediaPlayInputs {
	return &mediaPlayInputs{
		file:        input.NewText("File", file),
		projectFile: input.NewDropdown(nil, -1),
		fileBrowse:  new(widget.Clickable),
		outputID:    input.NewText("Output ID", outputID),
		clipStartMs: input.NewInteger("Clip Start MS", int(clipStartMs)),
		clipEndMs:   input.NewInteger("Clip End MS", int(clipEndMs)),
		fadeInMs:    input.NewInteger("Fade In MS", int(fadeInMs)),
		fadeOutMs:   input.NewInteger("Fade Out MS", int(fadeOutMs)),
		levelDB:     input.NewFloat("Level dB", levelDB),
	}
}

func newImageMediaPlayInputs(play *show.ImagePlay) *mediaPlayInputs {
	return &mediaPlayInputs{
		file:        input.NewText("File", play.File),
		projectFile: input.NewDropdown(nil, -1),
		fileBrowse:  new(widget.Clickable),
		outputID:    input.NewText("Output ID", play.OutputID),
		fadeInMs:    input.NewInteger("Fade In MS", int(play.FadeInMs)),
		fadeOutMs:   input.NewInteger("Fade Out MS", int(play.FadeOutMs)),
		durationMs:  input.NewInteger("Duration MS", int(play.DurationMs)),
	}
}

func newCueMediaPlayInputs(cue show.Cue) *mediaPlayInputs {
	switch {
	case cue.Play.Sound != nil:
		play := cue.Play.Sound
		return newTimedMediaPlayInputs(play.File, play.OutputID, play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs, play.LevelDB)
	case cue.Play.Video != nil:
		play := cue.Play.Video
		return newTimedMediaPlayInputs(play.File, play.OutputID, play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs, play.LevelDB)
	case cue.Play.Image != nil:
		return newImageMediaPlayInputs(cue.Play.Image)
	default:
		return nil
	}
}

func newCueEditPageState(cue show.Cue) cueEditPageState {
	state := cueEditPageState{
		initialized: true,
		cueID:       cue.ID,
		list:        layout.List{Axis: layout.Vertical},
	}

	state.general = cueGeneralInputs{
		cueNumber:   input.NewText("Cue Number", cue.CueNumber),
		description: input.NewMultiline("Description", cue.Description),
		color:       input.NewColourPicker("Color", cue.Color),
		tags:        input.NewText("Tags", strings.Join(cue.Tags, ", ")),
		notes:       input.NewMultiline("Notes", cue.Notes),
	}
	state.timing = cueTimingInputs{
		preWaitMs:  input.NewInteger("Pre Wait MS", int(cue.Timing.PreWaitMs)),
		postWaitMs: input.NewInteger("Post Wait MS", int(cue.Timing.PostWaitMs)),
	}
	state.link = cueLinkInputs{
		mode:       newEnumDropdown(cueLinkModeLabels, int(cue.Link.Mode)),
		targetKind: newEnumDropdown(cueTargetKindLabels, int(cue.Link.Target.Kind)),
	}
	if markers := cueTimecodeMarkers(&cue); markers != nil {
		state.markers = make([]timecodeMarkerInputs, len(*markers))
		for index, marker := range *markers {
			state.markers[index] = newTimecodeMarkerInputs(marker)
		}
	}

	state.media = newCueMediaPlayInputs(cue)
	if cue.Play.Remote != nil {
		state.remote = newCueRemoteInputs(cue.Play.Remote)
	}
	if cue.Play.Wait != nil {
		play := cue.Play.Wait
		state.wait = &cueWaitInputs{
			kind:       newEnumDropdown(waitKindLabels, int(play.Kind)),
			durationMs: input.NewInteger("Duration MS", int(play.DurationMs)),
			target:     newCueMediaTargetInputs(play.Media),
		}
	}
	if cue.Play.MediaControl != nil {
		state.mediaControl = newCueMediaControlInputs(cue.Play.MediaControl)
	}
	if cue.Play.OutputControl != nil {
		state.outputControl = newCueOutputControlInputs(cue.Play.OutputControl)
	}

	return state
}

func (ctx *CueEditUI) ensurePageInputs() {
	if ctx.page.initialized && ctx.page.cueID == ctx.cue.ID {
		return
	}

	ctx.ensureCuePlay()
	normalizeCueEditModel(&ctx.cue)
	ctx.tabs.active = tabGeneral
	ctx.page = newCueEditPageState(ctx.cue)
}

func (ctx *CueEditUI) ensureCuePlay() {
	switch ctx.cue.Type {
	case show.CueTypeSound:
		if ctx.cue.Play.Sound == nil {
			ctx.cue.Play.Sound = show.NewSoundCue().Play.Sound
		}
	case show.CueTypeVideo:
		if ctx.cue.Play.Video == nil {
			ctx.cue.Play.Video = show.NewVideoCue().Play.Video
		}
	case show.CueTypeImage:
		if ctx.cue.Play.Image == nil {
			ctx.cue.Play.Image = show.NewImageCue().Play.Image
		}
	case show.CueTypeRemote:
		if ctx.cue.Play.Remote == nil {
			ctx.cue.Play.Remote = show.NewRemoteCue().Play.Remote
		}
	case show.CueTypeWait:
		if ctx.cue.Play.Wait == nil {
			ctx.cue.Play.Wait = show.NewWaitCue().Play.Wait
		}
	case show.CueTypeMediaControl:
		if ctx.cue.Play.MediaControl == nil {
			ctx.cue.Play.MediaControl = show.NewMediaControlCue().Play.MediaControl
		}
	case show.CueTypeOutputControl:
		if ctx.cue.Play.OutputControl == nil {
			ctx.cue.Play.OutputControl = show.NewOutputControlCue().Play.OutputControl
		}
	}
}
