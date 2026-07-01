package ui

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/SysPoe/CuSus/show"
	"github.com/SysPoe/CuSus/ui/input"
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
	state.colour["color"] = input.NewColourPicker("Color", parseHexColor(cue.HexColor))
	state.text["tags"] = input.NewText("Tags", strings.Join(cue.Tags, ", "))
	state.multiline["notes"] = input.NewMultiline("Notes", cue.Notes)

	state.integer["preWaitMS"] = input.NewInteger("Pre Wait MS", int(cue.Timing.PreWaitMS))
	state.integer["postWaitMS"] = input.NewInteger("Post Wait MS", int(cue.Timing.PostWaitMS))

	state.dropdown["linkMode"] = newEnumDropdown(cueLinkModeLabels, int(cue.Link.Mode))
	state.dropdown["linkTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Link.Target.Kind))
	state.text["linkTargetCueID"] = input.NewText("Target Cue ID", cueIDString(cue.Link.Target.CueID))

	if cue.Play.Sound != nil {
		state.text["soundFile"] = input.NewText("File", cue.Play.Sound.File)
		state.button["soundFileBrowse"] = new(widget.Clickable)
		state.integer["soundClipStartMS"] = input.NewInteger("Clip Start MS", int(cue.Play.Sound.ClipStartMS))
		state.integer["soundClipEndMS"] = input.NewInteger("Clip End MS", int(cue.Play.Sound.ClipEndMS))
		state.integer["soundFadeInMS"] = input.NewInteger("Fade In MS", int(cue.Play.Sound.FadeInMS))
		state.integer["soundFadeOutMS"] = input.NewInteger("Fade Out MS", int(cue.Play.Sound.FadeOutMS))
		state.float["soundLevelDB"] = input.NewFloat("Level dB", cue.Play.Sound.LevelDB)
	}
	if cue.Play.Video != nil {
		state.text["videoFile"] = input.NewText("File", cue.Play.Video.File)
		state.button["videoFileBrowse"] = new(widget.Clickable)
		state.text["videoOutputID"] = input.NewText("Output ID", cue.Play.Video.OutputID)
		state.integer["videoClipStartMS"] = input.NewInteger("Clip Start MS", int(cue.Play.Video.ClipStartMS))
		state.integer["videoClipEndMS"] = input.NewInteger("Clip End MS", int(cue.Play.Video.ClipEndMS))
		state.integer["videoFadeInMS"] = input.NewInteger("Fade In MS", int(cue.Play.Video.FadeInMS))
		state.integer["videoFadeOutMS"] = input.NewInteger("Fade Out MS", int(cue.Play.Video.FadeOutMS))
		state.float["videoLevelDB"] = input.NewFloat("Level dB", cue.Play.Video.LevelDB)
	}
	if cue.Play.Image != nil {
		state.text["imageFile"] = input.NewText("File", cue.Play.Image.File)
		state.button["imageFileBrowse"] = new(widget.Clickable)
		state.text["imageOutputID"] = input.NewText("Output ID", cue.Play.Image.OutputID)
		state.integer["imageFadeInMS"] = input.NewInteger("Fade In MS", int(cue.Play.Image.FadeInMS))
		state.integer["imageFadeOutMS"] = input.NewInteger("Fade Out MS", int(cue.Play.Image.FadeOutMS))
		state.integer["imageDurationMS"] = input.NewInteger("Duration MS", int(cue.Play.Image.DurationMS))
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
		state.integer["waitDurationMS"] = input.NewInteger("Duration MS", int(cue.Play.Wait.DurationMS))
		state.dropdown["waitTargetKind"] = newEnumDropdown(cueTargetKindLabels, int(cue.Play.Wait.Target.Kind))
		state.text["waitTargetCueID"] = input.NewText("Target Cue ID", cueIDString(cue.Play.Wait.Target.CueID))
		state.dropdown["waitMediaTargetKind"] = newEnumDropdown(mediaTargetKindLabels, int(cue.Play.Wait.Media.Kind))
		state.text["waitMediaCueID"] = input.NewText("Media Cue ID", cueIDString(cue.Play.Wait.Media.CueID))
		state.text["waitMediaInstanceID"] = input.NewText("Instance ID", cue.Play.Wait.Media.InstanceID)
		state.text["waitMediaOutputID"] = input.NewText("Output ID", cue.Play.Wait.Media.OutputID)
	}
	if cue.Play.MediaControl != nil {
		mediaControl := cue.Play.MediaControl
		levelDB := 0.0
		if mediaControl.LevelDB != nil {
			levelDB = *mediaControl.LevelDB
		}
		seekToMS := 0
		if mediaControl.SeekToMS != nil {
			seekToMS = int(*mediaControl.SeekToMS)
		}

		state.dropdown["mediaCtrlAction"] = newEnumDropdown(mediaControlActionLabels, int(mediaControl.Action))
		state.dropdown["mediaCtrlTargetKind"] = newEnumDropdown(mediaTargetKindLabels, int(mediaControl.Target.Kind))
		state.text["mediaCtrlCueID"] = input.NewText("Target Cue ID", cueIDString(mediaControl.Target.CueID))
		state.text["mediaCtrlInstanceID"] = input.NewText("Instance ID", mediaControl.Target.InstanceID)
		state.text["mediaCtrlOutputID"] = input.NewText("Output ID", mediaControl.Target.OutputID)
		state.float["mediaCtrlLevelDB"] = input.NewFloat("Level dB", levelDB)
		state.integer["mediaCtrlSeekToMS"] = input.NewInteger("Seek To MS", seekToMS)
		state.integer["mediaCtrlFadeMS"] = input.NewInteger("Fade MS", int(mediaControl.FadeMS))
		state.dropdown["mediaCtrlCurve"] = newEnumDropdown(fadeCurveLabels, int(mediaControl.Curve))
	}
	if cue.Play.OutputControl != nil {
		state.dropdown["outputCtrlAction"] = newEnumDropdown(outputControlActionLabels, int(cue.Play.OutputControl.Action))
		state.text["outputCtrlOutputID"] = input.NewText("Output ID", cue.Play.OutputControl.OutputID)
		state.integer["outputCtrlFadeOutMS"] = input.NewInteger("Fade Out MS", int(cue.Play.OutputControl.FadeOutMS))
		state.integer["outputCtrlFadeInMS"] = input.NewInteger("Fade In MS", int(cue.Play.OutputControl.FadeInMS))
		state.text["outputCtrlMessage"] = input.NewText("Message", cue.Play.OutputControl.Message)
	}

	return state
}
