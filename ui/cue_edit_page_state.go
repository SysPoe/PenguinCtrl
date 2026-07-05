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
	state.text["title"] = input.NewText("Title", cue.Title)
	state.multiline["description"] = input.NewMultiline("Description", cue.Description)
	state.checkbox["disabled"] = input.NewCheckbox("Disabled", cue.Disabled)
	state.colour["color"] = input.NewColourPicker("Color", cue.Color)
	state.text["tags"] = input.NewText("Tags", strings.Join(cue.Tags, ", "))
	state.multiline["notes"] = input.NewMultiline("Notes", cue.Notes)

	state.integer["preWaitMs"] = input.NewInteger("Pre Wait MS", int(cue.Timing.PreWaitMs))
	state.integer["postWaitMs"] = input.NewInteger("Post Wait MS", int(cue.Timing.PostWaitMs))

	state.dropdown["linkMode"] = newEnumDropdown(cueLinkModeLabels, int(cue.Link.Mode))
	state.dropdown["linkTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Link.Target.Kind))

	if cue.Play.Sound != nil {
		state.text["soundFile"] = input.NewText("File", cue.Play.Sound.File)
		state.button["soundFileBrowse"] = new(widget.Clickable)
		state.integer["soundClipStartMs"] = input.NewInteger("Clip Start MS", int(cue.Play.Sound.ClipStartMs))
		state.integer["soundClipEndMs"] = input.NewInteger("Clip End MS", int(cue.Play.Sound.ClipEndMs))
		state.integer["soundFadeInMs"] = input.NewInteger("Fade In MS", int(cue.Play.Sound.FadeInMs))
		state.integer["soundFadeOutMs"] = input.NewInteger("Fade Out MS", int(cue.Play.Sound.FadeOutMs))
		state.float["soundLevelDB"] = input.NewFloat("Level dB", cue.Play.Sound.LevelDB)
	}
	if cue.Play.Video != nil {
		state.text["videoFile"] = input.NewText("File", cue.Play.Video.File)
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
	}
	if cue.Play.Wait != nil {
		state.dropdown["waitKind"] = newEnumDropdown(waitKindLabels, int(cue.Play.Wait.Kind))
		state.integer["waitDurationMs"] = input.NewInteger("Duration MS", int(cue.Play.Wait.DurationMs))
		state.dropdown["waitTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Play.Wait.Target.Kind))
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
