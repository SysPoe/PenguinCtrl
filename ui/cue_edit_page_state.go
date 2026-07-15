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

	// TODO(micro): magic string field keys ("cueNumber","soundFile",…) are duplicated in tab renderers; share const keys or typed field structs
	state.dropdown["linkMode"] = newEnumDropdown(cueLinkModeLabels, int(cue.Link.Mode))
	state.dropdown["linkTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Link.Target.Kind))

	if cue.Play.Sound != nil {
		// TODO(micro): Sound/Video/Image field init is nearly identical; extract mediaPlayInputs(kind, play) helper
		state.text["soundFile"] = input.NewText("File", cue.Play.Sound.File)
		state.dropdown["soundProjectFile"] = input.NewDropdown(nil, -1)
		state.button["soundFileBrowse"] = new(widget.Clickable)
		state.text["soundOutputID"] = input.NewText("Output ID", cue.Play.Sound.OutputID)
		state.integer["soundClipStartMs"] = input.NewInteger("Clip Start MS", int(cue.Play.Sound.ClipStartMs))
		state.integer["soundClipEndMs"] = input.NewInteger("Clip End MS", int(cue.Play.Sound.ClipEndMs))
		state.integer["soundFadeInMs"] = input.NewInteger("Fade In MS", int(cue.Play.Sound.FadeInMs))
		state.integer["soundFadeOutMs"] = input.NewInteger("Fade Out MS", int(cue.Play.Sound.FadeOutMs))
		state.float["soundLevelDB"] = input.NewFloat("Level dB", cue.Play.Sound.LevelDB)
	}
	if cue.Play.Video != nil {
		state.text["videoFile"] = input.NewText("File", cue.Play.Video.File)
		state.dropdown["videoProjectFile"] = input.NewDropdown(nil, -1)
		state.button["videoFileBrowse"] = new(widget.Clickable)
		state.text["videoOutputID"] = input.NewText("Output ID", cue.Play.Video.OutputID)
		state.integer["videoClipStartMs"] = input.NewInteger("Clip Start MS", int(cue.Play.Video.ClipStartMs))
		state.integer["videoClipEndMs"] = input.NewInteger("Clip End MS", int(cue.Play.Video.ClipEndMs))
		state.integer["videoFadeInMs"] = input.NewInteger("Fade In MS", int(cue.Play.Video.FadeInMs))
		state.integer["videoFadeOutMs"] = input.NewInteger("Fade Out MS", int(cue.Play.Video.FadeOutMs))
		state.float["videoLevelDB"] = input.NewFloat("Level dB", cue.Play.Video.LevelDB)
	}
	if cue.Play.Image != nil {
		state.text["imageFile"] = input.NewText("File", cue.Play.Image.File)
		state.dropdown["imageProjectFile"] = input.NewDropdown(nil, -1)
		state.button["imageFileBrowse"] = new(widget.Clickable)
		state.text["imageOutputID"] = input.NewText("Output ID", cue.Play.Image.OutputID)
		state.integer["imageFadeInMs"] = input.NewInteger("Fade In MS", int(cue.Play.Image.FadeInMs))
		state.integer["imageFadeOutMs"] = input.NewInteger("Fade Out MS", int(cue.Play.Image.FadeOutMs))
		state.integer["imageDurationMs"] = input.NewInteger("Duration MS", int(cue.Play.Image.DurationMs))
	}
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
