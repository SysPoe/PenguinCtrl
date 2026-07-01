package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/SysPoe/CuSus/show"
	"github.com/SysPoe/CuSus/ui/input"
	"github.com/google/uuid"
)

func (ctx *CueEditUI) DrawBody(th *material.Theme, gtx layout.Context) layout.FlexChild {
	ctx.ensurePageInputs()

	switch ctx.activeTab {
	case TabGeneral:
		return ctx.renderGeneralTab(th, gtx)
	case TabTiming:
		return ctx.renderTimingTab(th, gtx)
	case TabLink:
		return ctx.renderLinkTab(th, gtx)
	case TabMedia:
		return ctx.renderMediaTab(th, gtx)
	case TabTimecode:
		return ctx.renderTimecodeTab(th, gtx)
	case TabRemote:
		return ctx.renderRemoteTab(th, gtx)
	case TabWait:
		return ctx.renderWaitTab(th, gtx)
	case TabMediaCtrl:
		return ctx.renderMediaCtrlTab(th, gtx)
	case TabOutputCtrl:
		return ctx.renderOutputCtrlTab(th, gtx)
	}
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{}
	})
}

type cueEditFormRow struct {
	label  string
	layout func(gtx layout.Context) layout.Dimensions
}

var (
	cueLinkModeLabels = []string{
		"Manual",
		"Start Advance",
		"Start Play",
		"Fade In Advance",
		"Fade In Play",
		"Fade Out Advance",
		"Fade Out Play",
		"End Advance",
		"End Play",
	}
	cueTargetKindLabels = []string{
		"None",
		"Next",
		"Previous",
		"Cue ID",
	}
	remoteProtocolLabels = []string{
		"OSC",
		"ERC",
		"Auto",
	}
	remoteActionLabels = []string{
		"None",
		"Go",
		"Goto",
		"Back",
		"Release",
		"Level",
		"Custom",
	}
	waitKindLabels = []string{
		"Duration",
		"Media Start",
		"Media End",
		"Fade In Complete",
		"Fade Out Complete",
		"Instance Stopped",
		"All Audio Stopped",
		"All Video Stopped",
		"All Media Stopped",
	}
	mediaTargetKindLabels = []string{
		"Cue ID",
		"Instance ID",
		"All Audio",
		"All Video",
		"All Media",
		"Output ID",
	}
	mediaControlActionLabels = []string{
		"Fade To",
		"Fade Out",
		"Stop",
		"Pause",
		"Resume",
		"Seek",
		"Set Volume",
		"Mute",
		"Unmute",
	}
	fadeCurveLabels = []string{
		"Linear",
		"Equal Power",
	}
	outputControlActionLabels = []string{
		"Blackout",
		"Clear",
		"Test Pattern",
		"Identify",
		"Reopen",
		"Fullscreen",
		"Exit Fullscreen",
	}
	soundFileExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".aiff", ".aif", ".m4a"}
	videoFileExtensions = []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}
	imageFileExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
)

func (ctx *CueEditUI) ensurePageInputs() {
	if ctx.page.initialized && ctx.page.cueID == ctx.cue.ID {
		return
	}

	ctx.ensureCuePlay()
	ctx.cType = ctx.cue.Type
	ctx.activeTab = TabGeneral
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

func (ctx *CueEditUI) renderGeneralTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return ctx.renderForm(th, []cueEditFormRow{
		textRow(th, "Cue Number", ctx.page.text["cueNumber"], func(value string) { ctx.cue.CueNumber = value }),
		textRow(th, "Title", ctx.page.text["title"], func(value string) { ctx.cue.Title = value }),
		multilineRow(th, "Description", ctx.page.multiline["description"], func(value string) { ctx.cue.Description = value }),
		checkboxRow(th, "", ctx.page.checkbox["disabled"], func(value bool) { ctx.cue.Disabled = value }),
		colourRow(th, "Color", ctx.page.colour["color"], func(value color.NRGBA) { ctx.cue.HexColor = formatHexColor(value) }),
		textRow(th, "Tags", ctx.page.text["tags"], func(value string) { ctx.cue.Tags = splitTags(value) }),
		multilineRow(th, "Notes", ctx.page.multiline["notes"], func(value string) { ctx.cue.Notes = value }),
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return ctx.renderForm(th, []cueEditFormRow{
		integerRow(th, "Pre Wait MS", ctx.page.integer["preWaitMS"], func(value int) { ctx.cue.Timing.PreWaitMS = int64(value) }),
		integerRow(th, "Post Wait MS", ctx.page.integer["postWaitMS"], func(value int) { ctx.cue.Timing.PostWaitMS = int64(value) }),
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	rows := []cueEditFormRow{
		dropdownRow(th, "Mode", ctx.page.dropdown["linkMode"], func(selected int) {
			ctx.cue.Link.Mode = show.CueLinkMode(selected)
		}),
		dropdownRow(th, "Target", ctx.page.dropdown["linkTargetKind"], func(selected int) {
			ctx.cue.Link.Target.Kind = show.CueTargetKind(selected)
		}),
	}
	if ctx.cue.Link.Target.Kind == show.CueTargetCue {
		rows = append(rows, textRow(th, "Target Cue ID", ctx.page.text["linkTargetCueID"], func(value string) {
			parseCueID(value, &ctx.cue.Link.Target.CueID)
		}))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	rows := []cueEditFormRow{}
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", ctx.page.text["soundFile"], ctx.page.button["soundFileBrowse"], soundFileExtensions, func(value string) { play.File = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["soundClipStartMS"], func(value int) { play.ClipStartMS = int64(value) }),
			integerRow(th, "Clip End MS", ctx.page.integer["soundClipEndMS"], func(value int) { play.ClipEndMS = int64(value) }),
			integerRow(th, "Fade In MS", ctx.page.integer["soundFadeInMS"], func(value int) { play.FadeInMS = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["soundFadeOutMS"], func(value int) { play.FadeOutMS = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["soundLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", ctx.page.text["videoFile"], ctx.page.button["videoFileBrowse"], videoFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["videoOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["videoClipStartMS"], func(value int) { play.ClipStartMS = int64(value) }),
			integerRow(th, "Clip End MS", ctx.page.integer["videoClipEndMS"], func(value int) { play.ClipEndMS = int64(value) }),
			integerRow(th, "Fade In MS", ctx.page.integer["videoFadeInMS"], func(value int) { play.FadeInMS = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["videoFadeOutMS"], func(value int) { play.FadeOutMS = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["videoLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Image; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", ctx.page.text["imageFile"], ctx.page.button["imageFileBrowse"], imageFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["imageOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Fade In MS", ctx.page.integer["imageFadeInMS"], func(value int) { play.FadeInMS = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["imageFadeOutMS"], func(value int) { play.FadeOutMS = int64(value) }),
			integerRow(th, "Duration MS", ctx.page.integer["imageDurationMS"], func(value int) { play.DurationMS = int64(value) }),
		)
	}
	if len(rows) == 0 {
		rows = append(rows, staticRow(th, "Media", "No media settings for this cue type."))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderTimecodeTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, material.Body1(th, "Timecode marker editing is not implemented yet.").Layout)
	})
}

func (ctx *CueEditUI) renderRemoteTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.Remote
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Remote", "No remote settings for this cue type.")})
	}
	return ctx.renderForm(th, []cueEditFormRow{
		dropdownRow(th, "Protocol", ctx.page.dropdown["remoteProtocol"], func(selected int) { play.Protocol = show.RemoteProtocol(selected) }),
		dropdownRow(th, "Action", ctx.page.dropdown["remoteAction"], func(selected int) { play.Action = show.RemoteAction(selected) }),
		textRow(th, "Playback", ctx.page.text["remotePlayback"], func(value string) { play.Playback = value }),
		textRow(th, "Cue Number", ctx.page.text["remoteCueNumber"], func(value string) { play.CueNumber = value }),
		textRow(th, "Level", ctx.page.text["remoteLevel"], func(value string) { play.Level = value }),
	})
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.Wait
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Wait", "No wait settings for this cue type.")})
	}

	rows := []cueEditFormRow{
		dropdownRow(th, "Kind", ctx.page.dropdown["waitKind"], func(selected int) { play.Kind = show.WaitKind(selected) }),
	}
	if play.Kind == show.WaitDuration {
		rows = append(rows, integerRow(th, "Duration MS", ctx.page.integer["waitDurationMS"], func(value int) { play.DurationMS = int64(value) }))
	} else {
		rows = append(rows,
			dropdownRow(th, "Target", ctx.page.dropdown["waitTargetKind"], func(selected int) { play.Target.Kind = show.CueTargetKind(selected) }),
		)
		if play.Target.Kind == show.CueTargetCue {
			rows = append(rows, textRow(th, "Target Cue ID", ctx.page.text["waitTargetCueID"], func(value string) {
				parseCueID(value, &play.Target.CueID)
			}))
		}
		if waitKindUsesMediaTarget(play.Kind) {
			rows = appendMediaTargetRows(rows, th, ctx.page, "waitMedia", &play.Media)
		}
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.MediaControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Media Control", "No media control settings for this cue type.")})
	}

	rows := []cueEditFormRow{
		dropdownRow(th, "Action", ctx.page.dropdown["mediaCtrlAction"], func(selected int) {
			play.Action = show.MediaControlAction(selected)
			syncMediaControlOptionals(play, ctx.page)
		}),
	}
	rows = appendMediaTargetRows(rows, th, ctx.page, "mediaCtrl", &play.Target)
	if mediaControlActionUsesLevel(play.Action) {
		rows = append(rows, floatRow(th, "Level dB", ctx.page.float["mediaCtrlLevelDB"], func(value float64) { play.LevelDB = &value }))
	}
	if play.Action == show.MediaControlSeek {
		rows = append(rows, integerRow(th, "Seek To MS", ctx.page.integer["mediaCtrlSeekToMS"], func(value int) { play.SeekToMS = ptr(int64(value)) }))
	}
	rows = append(rows,
		integerRow(th, "Fade MS", ctx.page.integer["mediaCtrlFadeMS"], func(value int) { play.FadeMS = int64(value) }),
		dropdownRow(th, "Curve", ctx.page.dropdown["mediaCtrlCurve"], func(selected int) { play.Curve = show.FadeCurve(selected) }),
	)
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderOutputCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.OutputControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Output Control", "No output control settings for this cue type.")})
	}
	return ctx.renderForm(th, []cueEditFormRow{
		dropdownRow(th, "Action", ctx.page.dropdown["outputCtrlAction"], func(selected int) { play.Action = show.OutputControlAction(selected) }),
		textRow(th, "Output ID", ctx.page.text["outputCtrlOutputID"], func(value string) { play.OutputID = value }),
		integerRow(th, "Fade Out MS", ctx.page.integer["outputCtrlFadeOutMS"], func(value int) { play.FadeOutMS = int64(value) }),
		integerRow(th, "Fade In MS", ctx.page.integer["outputCtrlFadeInMS"], func(value int) { play.FadeInMS = int64(value) }),
		textRow(th, "Message", ctx.page.text["outputCtrlMessage"], func(value string) { play.Message = value }),
	})
}

func (ctx *CueEditUI) renderForm(th *material.Theme, rows []cueEditFormRow) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ctx.page.list.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				row := rows[index]
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{}
					if row.label != "" {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body2(th, row.label+":")
							label.TextSize = unit.Sp(18)
							gtx.Constraints.Min.X = gtx.Dp(unit.Dp(120))
							gtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
							return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, label.Layout)
						}))
					}
					children = append(children, layout.Rigid(row.layout))
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
				})
			})
		})
	})
}

func textRow(th *material.Theme, label string, field *input.Text, apply func(value string)) cueEditFormRow {
	return cueEditFormRow{label: label, layout: func(gtx layout.Context) layout.Dimensions {
		dims := field.Layout(th, gtx)
		apply(field.Value)
		return dims
	}}
}

func appendMediaTargetRows(rows []cueEditFormRow, th *material.Theme, state cueEditPageState, prefix string, target *show.MediaTarget) []cueEditFormRow {
	rows = append(rows, dropdownRow(th, "Target", state.dropdown[prefix+"TargetKind"], func(selected int) {
		target.Kind = show.MediaTargetKind(selected)
	}))

	switch target.Kind {
	case show.MediaTargetCue:
		rows = append(rows, textRow(th, "Target Cue ID", state.text[prefix+"CueID"], func(value string) {
			parseCueID(value, &target.CueID)
		}))
	case show.MediaTargetInstance:
		rows = append(rows, textRow(th, "Instance ID", state.text[prefix+"InstanceID"], func(value string) {
			target.InstanceID = value
		}))
	case show.MediaTargetOutput:
		rows = append(rows, textRow(th, "Output ID", state.text[prefix+"OutputID"], func(value string) {
			target.OutputID = value
		}))
	}

	return rows
}

func newEnumDropdown(labels []string, selected int) *input.Dropdown {
	items := make([]input.DropdownItem, len(labels))
	for i, label := range labels {
		items[i] = input.DropdownItem{
			Label: label,
			Value: strconv.Itoa(i),
		}
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	return input.NewDropdown(items, selected)
}

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}

	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func cueIDString(id show.CueID) string {
	if id == (show.CueID{}) {
		return ""
	}
	return uuid.UUID(id).String()
}

func parseCueID(value string, target *show.CueID) {
	value = strings.TrimSpace(value)
	if value == "" {
		*target = show.CueID{}
		return
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return
	}
	*target = show.CueID(id)
}

func parseHexColor(value string) color.NRGBA {
	defaultColor := color.NRGBA{R: 0xFF, A: 0xFF}
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 && len(value) != 8 {
		return defaultColor
	}

	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return defaultColor
	}

	if len(value) == 6 {
		return color.NRGBA{
			R: uint8(parsed >> 16),
			G: uint8(parsed >> 8),
			B: uint8(parsed),
			A: 0xFF,
		}
	}

	return color.NRGBA{
		R: uint8(parsed >> 24),
		G: uint8(parsed >> 16),
		B: uint8(parsed >> 8),
		A: uint8(parsed),
	}
}

func formatHexColor(value color.NRGBA) string {
	if value.A == 0xFF {
		return fmt.Sprintf("#%02X%02X%02X", value.R, value.G, value.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", value.R, value.G, value.B, value.A)
}

func waitKindUsesMediaTarget(kind show.WaitKind) bool {
	return kind == show.WaitMediaStart ||
		kind == show.WaitMediaEnd ||
		kind == show.WaitFadeInComplete ||
		kind == show.WaitFadeOutComplete ||
		kind == show.WaitInstanceStopped
}

func mediaControlActionUsesLevel(action show.MediaControlAction) bool {
	return action == show.MediaControlFadeTo ||
		action == show.MediaControlSetVolume
}

func syncMediaControlOptionals(play *show.MediaControlPlay, state cueEditPageState) {
	if mediaControlActionUsesLevel(play.Action) {
		play.LevelDB = &state.float["mediaCtrlLevelDB"].Value
	} else {
		play.LevelDB = nil
	}

	if play.Action == show.MediaControlSeek {
		play.SeekToMS = ptr(int64(state.integer["mediaCtrlSeekToMS"].Value))
	} else {
		play.SeekToMS = nil
	}
}

// Fixes being unable to do e.g. &int64(5) in Go. So you would do ptr(int64(5)) instead.
func ptr[T any](value T) *T {
	return &value
}
