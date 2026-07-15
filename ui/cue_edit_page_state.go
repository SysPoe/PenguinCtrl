package ui

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

// TODO(macro): Timecode marker widgets remain string-keyed because marker rows
// are rebuilt after sorting, deletion, undo, and action-type changes. Replace
// these maps with a typed marker-input slice owned by the timeline editor.
type cueEditPageState struct {
	initialized bool
	cueID       show.CueID
	list        layout.List

	text     map[string]*input.Text
	integer  map[string]*input.Integer
	float    map[string]*input.Float
	checkbox map[string]*input.Checkbox
	dropdown map[string]*input.Dropdown
	button   map[string]*widget.Clickable

	general       cueGeneralInputs
	timing        cueTimingInputs
	link          cueLinkInputs
	media         *mediaPlayInputs
	remote        *cueRemoteInputs
	wait          *cueWaitInputs
	mediaControl  *cueMediaControlInputs
	outputControl *cueOutputControlInputs
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

type cueOutputControlInputs struct {
	action    *input.Dropdown
	outputID  *input.Text
	fadeOutMs *input.Integer
	fadeInMs  *input.Integer
	message   *input.Text
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
		text:        map[string]*input.Text{},
		integer:     map[string]*input.Integer{},
		float:       map[string]*input.Float{},
		checkbox:    map[string]*input.Checkbox{},
		dropdown:    map[string]*input.Dropdown{},
		button:      map[string]*widget.Clickable{},
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
		for index, marker := range *markers {
			initTimecodeMarkerInputs(&state, index, marker)
		}
	}

	state.media = newCueMediaPlayInputs(cue)
	if cue.Play.Remote != nil {
		play := cue.Play.Remote
		state.remote = &cueRemoteInputs{
			protocol:  newEnumDropdown(remoteProtocolLabels, int(play.Protocol)),
			action:    newEnumDropdown(remoteActionLabels, int(play.Action)),
			playback:  input.NewText("Playback", play.Playback),
			cueNumber: input.NewText("Cue Number", play.CueNumber),
			level:     input.NewText("Level", play.Level),
			custom:    input.NewText("Custom Command", play.Custom),
		}
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
		mediaControl := cue.Play.MediaControl
		levelDB := 0.0
		if mediaControl.LevelDB != nil {
			levelDB = *mediaControl.LevelDB
		}
		seekToMs := 0
		if mediaControl.SeekToMs != nil {
			seekToMs = int(*mediaControl.SeekToMs)
		}

		state.mediaControl = &cueMediaControlInputs{
			action:   newEnumDropdown(mediaControlActionLabels, int(mediaControl.Action)),
			target:   newCueMediaTargetInputs(mediaControl.Target),
			levelDB:  input.NewFloat("Level dB", levelDB),
			seekToMs: input.NewInteger("Seek To MS", seekToMs),
			fadeMs:   input.NewInteger("Fade MS", int(mediaControl.FadeMs)),
			curve:    newEnumDropdown(fadeCurveLabels, int(mediaControl.Curve)),
		}
	}
	if cue.Play.OutputControl != nil {
		play := cue.Play.OutputControl
		state.outputControl = &cueOutputControlInputs{
			action:    newEnumDropdown(outputControlActionLabels, int(play.Action)),
			outputID:  input.NewText("Output ID", play.OutputID),
			fadeOutMs: input.NewInteger("Fade Out MS", int(play.FadeOutMs)),
			fadeInMs:  input.NewInteger("Fade In MS", int(play.FadeInMs)),
			message:   input.NewText("Message", play.Message),
		}
	}

	return state
}

func initTimecodeMarkerInputs(state *cueEditPageState, index int, marker show.TimecodeMarker) {
	key := fmt.Sprintf("timecode.%d", index)
	state.integer[key+".time"] = input.NewInteger("Time MS", int(marker.TimeMs))
	state.checkbox[key+".disabled"] = input.NewCheckbox("Disabled", marker.Disabled)
	state.dropdown[key+".type"] = newEnumDropdown(timecodeActionLabels, timecodeActionIndex(marker.Type))
	state.button["timecodeDelete"] = new(widget.Clickable)

	media := marker.Action.MediaControl
	if media == nil {
		media = defaultTimecodeMediaControl()
	}
	level := 0.0
	if media.LevelDB != nil {
		level = *media.LevelDB
	}
	seek := int64(0)
	if media.SeekToMs != nil {
		seek = *media.SeekToMs
	}
	state.dropdown[key+".mediaAction"] = newEnumDropdown(mediaControlActionLabels, int(media.Action))
	state.float[key+".level"] = input.NewFloat("Level dB", level)
	state.integer[key+".fade"] = input.NewInteger("Fade MS", int(media.FadeMs))
	state.integer[key+".seek"] = input.NewInteger("Seek MS", int(seek))
	state.dropdown[key+".curve"] = newEnumDropdown(fadeCurveLabels, int(media.Curve))

	output := marker.Action.OutputControl
	if output == nil {
		output = show.NewOutputControlCue().Play.OutputControl
	}
	state.dropdown[key+".outputAction"] = newEnumDropdown(outputControlActionLabels, int(output.Action))
	state.text[key+".outputID"] = input.NewText("Output ID", output.OutputID)
	state.integer[key+".fadeOut"] = input.NewInteger("Fade Out MS", int(output.FadeOutMs))
	state.integer[key+".fadeIn"] = input.NewInteger("Fade In MS", int(output.FadeInMs))
	state.text[key+".message"] = input.NewText("Message", output.Message)

	remote := marker.Action.Remote
	if remote == nil {
		remote = show.NewRemoteCue().Play.Remote
	}
	state.dropdown[key+".protocol"] = newEnumDropdown(remoteProtocolLabels, int(remote.Protocol))
	state.dropdown[key+".remoteAction"] = newEnumDropdown(remoteActionLabels, int(remote.Action))
	state.text[key+".playback"] = input.NewText("Playback", remote.Playback)
	state.text[key+".cueNumber"] = input.NewText("Cue Number", remote.CueNumber)
	state.text[key+".remoteLevel"] = input.NewText("Level", remote.Level)
	state.text[key+".custom"] = input.NewText("Custom Command", remote.Custom)
}
