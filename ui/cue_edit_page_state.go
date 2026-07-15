package ui

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

// TODO(macro): Replace the parallel string-keyed widget maps with typed
// per-cue editor models or field descriptors. Misspelled/missing keys and
// mismatches between initialization, rendering, and apply logic are currently
// runtime failures that the compiler cannot expose.
// TODO(macro): Page widgets are string-keyed bags rebuilt by magic keys across tabs, marker rows, and timeline resets. Replace with typed per-tab (or per-field-group) structs so field identity is compile-checked and marker editors don't share a global string namespace with the Media tab.
type cueEditPageState struct {
	initialized bool
	cueID       show.CueID
	list        layout.List

	text      map[string]*input.Text
	multiline map[string]*input.Multiline
	integer   map[string]*input.Integer
	float     map[string]*input.Float
	checkbox  map[string]*input.Checkbox
	dropdown  map[string]*input.Dropdown
	colour    map[string]*input.ColourPicker
	button    map[string]*widget.Clickable

	media *mediaPlayInputs
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
		multiline:   map[string]*input.Multiline{},
		integer:     map[string]*input.Integer{},
		float:       map[string]*input.Float{},
		checkbox:    map[string]*input.Checkbox{},
		dropdown:    map[string]*input.Dropdown{},
		colour:      map[string]*input.ColourPicker{},
		button:      map[string]*widget.Clickable{},
	}

	state.text["cueNumber"] = input.NewText("Cue Number", cue.CueNumber)
	state.multiline["description"] = input.NewMultiline("Description", cue.Description)
	state.colour["color"] = input.NewColourPicker("Color", cue.Color)
	state.text["tags"] = input.NewText("Tags", strings.Join(cue.Tags, ", "))
	state.multiline["notes"] = input.NewMultiline("Notes", cue.Notes)

	state.integer["preWaitMs"] = input.NewInteger("Pre Wait MS", int(cue.Timing.PreWaitMs))
	state.integer["postWaitMs"] = input.NewInteger("Post Wait MS", int(cue.Timing.PostWaitMs))
	state.button["timecodeAdd"] = new(widget.Clickable)
	if markers := cueTimecodeMarkers(&cue); markers != nil {
		for index, marker := range *markers {
			initTimecodeMarkerInputs(&state, index, marker)
		}
	}

	// TODO(micro): remaining general/link/action string field keys are duplicated in tab renderers; migrate them to typed field structs
	state.dropdown["linkMode"] = newEnumDropdown(cueLinkModeLabels, int(cue.Link.Mode))
	state.dropdown["linkTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Link.Target.Kind))
	state.media = newCueMediaPlayInputs(cue)
	if cue.Play.Remote != nil {
		state.dropdown["remoteProtocol"] = newEnumDropdown(remoteProtocolLabels, int(cue.Play.Remote.Protocol))
		state.dropdown["remoteAction"] = newEnumDropdown(remoteActionLabels, int(cue.Play.Remote.Action))
		state.text["remotePlayback"] = input.NewText("Playback", cue.Play.Remote.Playback)
		state.text["remoteCueNumber"] = input.NewText("Cue Number", cue.Play.Remote.CueNumber)
		state.text["remoteLevel"] = input.NewText("Level", cue.Play.Remote.Level)
		state.text["remoteCustom"] = input.NewText("Custom Command", cue.Play.Remote.Custom)
	}
	if cue.Play.Wait != nil {
		state.dropdown["waitKind"] = newEnumDropdown(waitKindLabels, int(cue.Play.Wait.Kind))
		state.integer["waitDurationMs"] = input.NewInteger("Duration MS", int(cue.Play.Wait.DurationMs))
		state.dropdown["waitMediaTargetKind"] = newEnumDropdown(mediaTargetKindLabels, int(cue.Play.Wait.Media.Kind))
		state.text["waitMediaInstanceID"] = input.NewText("Instance ID", cue.Play.Wait.Media.InstanceID)
		state.text["waitMediaOutputID"] = input.NewText("Output ID", cue.Play.Wait.Media.OutputID)
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

		state.dropdown["mediaCtrlAction"] = newEnumDropdown(mediaControlActionLabels, int(mediaControl.Action))
		state.dropdown["mediaCtrlTargetKind"] = newEnumDropdown(mediaTargetKindLabels, int(mediaControl.Target.Kind))
		state.text["mediaCtrlInstanceID"] = input.NewText("Instance ID", mediaControl.Target.InstanceID)
		state.text["mediaCtrlOutputID"] = input.NewText("Output ID", mediaControl.Target.OutputID)
		state.float["mediaCtrlLevelDB"] = input.NewFloat("Level dB", levelDB)
		state.integer["mediaCtrlSeekToMs"] = input.NewInteger("Seek To MS", seekToMs)
		state.integer["mediaCtrlFadeMs"] = input.NewInteger("Fade MS", int(mediaControl.FadeMs))
		state.dropdown["mediaCtrlCurve"] = newEnumDropdown(fadeCurveLabels, int(mediaControl.Curve))
	}
	if cue.Play.OutputControl != nil {
		state.dropdown["outputCtrlAction"] = newEnumDropdown(outputControlActionLabels, int(cue.Play.OutputControl.Action))
		state.text["outputCtrlOutputID"] = input.NewText("Output ID", cue.Play.OutputControl.OutputID)
		state.integer["outputCtrlFadeOutMs"] = input.NewInteger("Fade Out MS", int(cue.Play.OutputControl.FadeOutMs))
		state.integer["outputCtrlFadeInMs"] = input.NewInteger("Fade In MS", int(cue.Play.OutputControl.FadeInMs))
		state.text["outputCtrlMessage"] = input.NewText("Message", cue.Play.OutputControl.Message)
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
