package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

func (ctx *CueEditUI) drawBody(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	ctx.ensurePageInputs()
	if ctx.focusFirstInput {
		ctx.focusActiveTab()
		ctx.focusFirstInput = false
	}

	switch ctx.activeTab {
	case tabGeneral:
		return ctx.renderGeneralTab(th, gtx)
	case tabTiming:
		return ctx.renderTimingTab(th, gtx)
	case tabLink:
		return ctx.renderLinkTab(th, gtx, manager)
	case tabMedia:
		return ctx.renderMediaTab(th, gtx)
	case tabTimecode:
		return ctx.renderTimecodeTab(th, gtx, manager)
	case tabRemote:
		return ctx.renderRemoteTab(th, gtx)
	case tabWait:
		return ctx.renderWaitTab(th, gtx, manager)
	case tabMediaCtrl:
		return ctx.renderMediaCtrlTab(th, gtx, manager)
	case tabOutputCtrl:
		return ctx.renderOutputCtrlTab(th, gtx)
	}
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{}
	})
}

func (ctx *CueEditUI) focusActiveTab() {
	switch ctx.activeTab {
	case tabGeneral:
		ctx.page.text["cueNumber"].Focus()
	case tabTiming:
		ctx.page.integer["preWaitMs"].Focus()
	case tabLink:
		ctx.page.dropdown["linkMode"].Focus()
	case tabMedia:
		switch ctx.cType {
		case show.CueTypeSound:
			ctx.page.text["soundFile"].Focus()
		case show.CueTypeVideo:
			ctx.page.text["videoFile"].Focus()
		case show.CueTypeImage:
			ctx.page.text["imageFile"].Focus()
		}
	case tabRemote:
		ctx.page.dropdown["remoteProtocol"].Focus()
	case tabWait:
		ctx.page.dropdown["waitKind"].Focus()
	case tabMediaCtrl:
		ctx.page.dropdown["mediaCtrlAction"].Focus()
	case tabOutputCtrl:
		ctx.page.dropdown["outputCtrlAction"].Focus()
	}
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
		"Auto",
		"OSC",
		"ERC",
	}
	remoteActionLabels = []string{
		"None",
		"Go",
		"Go to",
		"Back",
		"Release",
		"Level",
		"Activate",
		"Flash",
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
		"Current Track",
		"Cue Group",
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
	timecodeActionLabels = []string{
		"Current track",
		"Output control",
		"Remote",
	}
	videoFileExtensions = []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}
	soundFileExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".aiff", ".aif", ".m4a", ".opus"}
	imageFileExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
)

func (ctx *CueEditUI) ensurePageInputs() {
	if ctx.page.initialized && ctx.page.cueID == ctx.cue.ID {
		return
	}

	ctx.ensureCuePlay()
	ctx.cType = ctx.cue.Type
	ctx.activeTab = tabGeneral
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
		multilineRow(th, "Description", ctx.page.multiline["description"], func(value string) { ctx.cue.Description = value }),
		colourRow(th, "Color", ctx.page.colour["color"], func(value color.NRGBA) { ctx.cue.Color = value }),
		textRow(th, "Tags", ctx.page.text["tags"], func(value string) { ctx.cue.Tags = splitTags(value) }),
		multilineRow(th, "Notes", ctx.page.multiline["notes"], func(value string) { ctx.cue.Notes = value }),
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return ctx.renderForm(th, []cueEditFormRow{
		integerRow(th, "Pre Wait MS", ctx.page.integer["preWaitMs"], func(value int) { ctx.cue.Timing.PreWaitMs = int64(value) }),
		integerRow(th, "Post Wait MS", ctx.page.integer["postWaitMs"], func(value int) { ctx.cue.Timing.PostWaitMs = int64(value) }),
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	rows := []cueEditFormRow{
		dropdownRow(th, "Mode", ctx.page.dropdown["linkMode"], func(selected int) {
			ctx.cue.Link.Mode = show.CueLinkMode(selected)
			if ctx.cue.Link.Mode != show.CueLinkManual && ctx.cue.Link.Target.Kind == show.CueTargetNone {
				ctx.cue.Link.Target.Kind = show.CueTargetNext
				ctx.page.dropdown["linkTargetKind"].Selected = int(show.CueTargetNext)
			}
		}),
		dropdownRow(th, "Target", ctx.page.dropdown["linkTargetKind"], func(selected int) {
			ctx.cue.Link.Target.Kind = show.CueTargetKind(selected)
		}),
	}
	if ctx.cue.Link.Target.Kind == show.CueTargetCue {
		rows = append(rows, ctx.cueTargetDropdownRow(th, "Target Cue", "linkTargetCue", manager, &ctx.cue.Link.Target.CueID))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	rows := []cueEditFormRow{}
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "audio", ctx.page.text["soundFile"], ctx.page.dropdown["soundProjectFile"], ctx.page.button["soundFileBrowse"], soundFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["soundOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["soundClipStartMs"], func(value int) { play.ClipStartMs = int64(value) }),
			integerRow(th, "Clip End MS", ctx.page.integer["soundClipEndMs"], func(value int) { play.ClipEndMs = int64(value) }),
			integerRow(th, "Fade In MS", ctx.page.integer["soundFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["soundFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["soundLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "video", ctx.page.text["videoFile"], ctx.page.dropdown["videoProjectFile"], ctx.page.button["videoFileBrowse"], videoFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["videoOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["videoClipStartMs"], func(value int) { play.ClipStartMs = int64(value) }),
			integerRow(th, "Clip End MS", ctx.page.integer["videoClipEndMs"], func(value int) { play.ClipEndMs = int64(value) }),
			integerRow(th, "Fade In MS", ctx.page.integer["videoFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["videoFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["videoLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Image; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "image", ctx.page.text["imageFile"], ctx.page.dropdown["imageProjectFile"], ctx.page.button["imageFileBrowse"], imageFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["imageOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Fade In MS", ctx.page.integer["imageFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["imageFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			integerRow(th, "Duration MS", ctx.page.integer["imageDurationMs"], func(value int) { play.DurationMs = int64(value) }),
		)
	}
	if len(rows) == 0 {
		rows = append(rows, staticRow(th, "Media", "No media settings for this cue type."))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderTimecodeTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	markers := cueTimecodeMarkers(&ctx.cue)
	if markers == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Timecode", "This cue type does not support timecode markers.")})
	}

	if sortTimecodeMarkers(markers) {
		ctx.timeline.selected = map[int]bool{}
		ctx.resetTimecodeInputs()
	}

	rows := ctx.timecodeEditorRows(th, markers)
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ctx.renderNativeTimecodeEditor(th, gtx, manager, markers)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ctx.layoutFormRows(th, gtx, rows)
				}),
			)
		})
	})
}

func cueTimecodeMarkers(cue *show.Cue) *[]show.TimecodeMarker {
	if cue == nil {
		return nil
	}
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return &cue.Play.Sound.Timecode
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return &cue.Play.Video.Timecode
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return &cue.Play.Image.Timecode
		}
	}
	return nil
}

func (ctx *CueEditUI) renderRemoteTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.Remote
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Remote", "No remote settings for this cue type.")})
	}
	rows := []cueEditFormRow{
		dropdownRow(th, "Protocol", ctx.page.dropdown["remoteProtocol"], func(selected int) { play.Protocol = show.RemoteProtocol(selected) }),
		dropdownRow(th, "Action", ctx.page.dropdown["remoteAction"], func(selected int) { play.Action = show.RemoteAction(selected) }),
		textRow(th, "Playback", ctx.page.text["remotePlayback"], func(value string) { play.Playback = value }),
		textRow(th, "Cue Number", ctx.page.text["remoteCueNumber"], func(value string) { play.CueNumber = value }),
		textRow(th, "Level", ctx.page.text["remoteLevel"], func(value string) { play.Level = value }),
	}
	if play.Action == show.RemoteActionCustom {
		rows = append(rows, textRow(th, "Custom Command", ctx.page.text["remoteCustom"], func(value string) { play.Custom = value }))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.Wait
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Wait", "No wait settings for this cue type.")})
	}

	rows := []cueEditFormRow{
		dropdownRow(th, "Kind", ctx.page.dropdown["waitKind"], func(selected int) { play.Kind = show.WaitKind(selected) }),
	}
	if play.Kind == show.WaitDuration {
		rows = append(rows, integerRow(th, "Duration MS", ctx.page.integer["waitDurationMs"], func(value int) { play.DurationMs = int64(value) }))
	} else {
		if waitKindUsesMediaTarget(play.Kind) {
			rows = ctx.appendMediaTargetRows(rows, th, manager, "waitMedia", &play.Media)
		}
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
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
	rows = ctx.appendMediaTargetRows(rows, th, manager, "mediaCtrl", &play.Target)
	if mediaControlActionUsesLevel(play.Action) {
		rows = append(rows, floatRow(th, "Level dB", ctx.page.float["mediaCtrlLevelDB"], func(value float64) { play.LevelDB = &value }))
	}
	if play.Action == show.MediaControlSeek {
		rows = append(rows, integerRow(th, "Seek To MS", ctx.page.integer["mediaCtrlSeekToMs"], func(value int) { play.SeekToMs = ptr(int64(value)) }))
	}
	rows = append(rows,
		integerRow(th, "Fade MS", ctx.page.integer["mediaCtrlFadeMs"], func(value int) { play.FadeMs = int64(value) }),
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
		integerRow(th, "Fade Out MS", ctx.page.integer["outputCtrlFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
		integerRow(th, "Fade In MS", ctx.page.integer["outputCtrlFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
		textRow(th, "Message", ctx.page.text["outputCtrlMessage"], func(value string) { play.Message = value }),
	})
}

func (ctx *CueEditUI) renderForm(th *material.Theme, rows []cueEditFormRow) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ctx.layoutFormRows(th, gtx, rows)
		})
	})
}

func (ctx *CueEditUI) layoutFormRows(th *material.Theme, gtx layout.Context, rows []cueEditFormRow) layout.Dimensions {
	return ctx.page.list.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		row := rows[index]
		return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{}
			if row.label != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := stableBody2(th, row.label+":")
					label.TextSize = unit.Sp(18)
					labelWidth := gtx.Dp(unit.Dp(120))
					maxLabelWidth := gtx.Constraints.Max.X / 3
					if maxLabelWidth > 0 && labelWidth > maxLabelWidth {
						labelWidth = maxLabelWidth
					}
					if labelWidth < 0 {
						labelWidth = 0
					}
					gtx.Constraints.Min.X = labelWidth
					gtx.Constraints.Max.X = labelWidth
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutStableText(gtx, label.Layout)
					})
				}))
			}
			children = append(children, layout.Flexed(1, row.layout))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, children...)
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

func (ctx *CueEditUI) appendMediaTargetRows(rows []cueEditFormRow, th *material.Theme, manager *show.ShowManager, prefix string, target *show.MediaTarget) []cueEditFormRow {
	rows = append(rows, dropdownRow(th, "Target", ctx.page.dropdown[prefix+"TargetKind"], func(selected int) {
		target.Kind = show.MediaTargetKind(selected)
	}))

	switch target.Kind {
	case show.MediaTargetCue:
		rows = append(rows, ctx.cueTargetDropdownRow(th, "Target Cue", prefix+"Cue", manager, &target.CueID))
	case show.MediaTargetInstance:
		rows = append(rows, textRow(th, "Instance ID", ctx.page.text[prefix+"InstanceID"], func(value string) {
			target.InstanceID = value
		}))
	case show.MediaTargetOutput:
		rows = append(rows, textRow(th, "Output ID", ctx.page.text[prefix+"OutputID"], func(value string) {
			target.OutputID = value
		}))
	case show.MediaTargetGroup:
		rows = append(rows, ctx.groupTargetDropdownRow(th, "Target Group", prefix+"Group", manager, &target.GroupID))
	}

	return rows
}

func (ctx *CueEditUI) groupTargetDropdownRow(th *material.Theme, label, key string, manager *show.ShowManager, target *show.GroupID) cueEditFormRow {
	items := groupDropdownItems(manager)
	selected := 0
	for index, item := range items {
		if item.Value == uuid.UUID(*target).String() {
			selected = index
			break
		}
	}
	dropdown := ctx.page.dropdown[key]
	if dropdown == nil {
		dropdown = input.NewDropdown(items, selected)
		ctx.page.dropdown[key] = dropdown
	} else {
		dropdown.SetItems(items, selected)
	}
	return dropdownRow(th, label, dropdown, func(selected int) {
		if selected < 0 || selected >= len(dropdown.Items) {
			return
		}
		id, err := uuid.Parse(strings.TrimSpace(dropdown.Items[selected].Value))
		if err != nil {
			*target = show.GroupID{}
			return
		}
		*target = show.GroupID(id)
	})
}

func groupDropdownItems(manager *show.ShowManager) []input.DropdownItem {
	if manager == nil {
		return []input.DropdownItem{{Label: "No cue groups available", Value: ""}}
	}
	groups := manager.Groups()
	if len(groups) == 0 {
		return []input.DropdownItem{{Label: "No cue groups available", Value: ""}}
	}
	items := make([]input.DropdownItem, 0, len(groups))
	for _, group := range groups {
		title := strings.TrimSpace(group.Title)
		if title == "" {
			title = "Untitled Group"
		}
		items = append(items, input.DropdownItem{
			Label: fmt.Sprintf("%s (%d cues)", title, group.Count),
			Value: uuid.UUID(group.ID).String(),
		})
	}
	return items
}

func (ctx *CueEditUI) cueTargetDropdownRow(th *material.Theme, label, key string, manager *show.ShowManager, target *show.CueID) cueEditFormRow {
	dropdown := ctx.ensureCueTargetDropdown(key, manager, *target)
	return dropdownRow(th, label, dropdown, func(selected int) {
		if selected < 0 || selected >= len(dropdown.Items) {
			return
		}

		value := strings.TrimSpace(dropdown.Items[selected].Value)
		if value == "" {
			*target = show.CueID{}
			return
		}

		id, err := uuid.Parse(value)
		if err != nil {
			return
		}
		*target = show.CueID(id)
	})
}

func (ctx *CueEditUI) ensureCueTargetDropdown(key string, manager *show.ShowManager, selectedCueID show.CueID) *input.Dropdown {
	items := cueDropdownItems(manager, ctx.cue.ID)
	selected := cueDropdownSelectedIndex(items, selectedCueID)

	dropdown := ctx.page.dropdown[key]
	if dropdown == nil {
		dropdown = input.NewDropdown(items, selected)
		ctx.page.dropdown[key] = dropdown
		return dropdown
	}

	dropdown.SetItems(items, selected)
	return dropdown
}

func cueDropdownItems(manager *show.ShowManager, excludeCueID show.CueID) []input.DropdownItem {
	if manager == nil {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}

	cues := manager.Snapshot()
	if len(cues) == 0 {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}
	items := make([]input.DropdownItem, 0, len(cues))
	for _, cue := range cues {
		if cue.ID == excludeCueID {
			continue
		}
		items = append(items, input.DropdownItem{
			Label: cueDropdownLabel(cue),
			Value: uuid.UUID(cue.ID).String(),
		})
	}

	if len(items) == 0 {
		return []input.DropdownItem{{Label: "No other cues available", Value: ""}}
	}
	return items
}

func cueDropdownLabel(cue show.Cue) string {
	number := strings.TrimSpace(cue.CueNumber)
	description := strings.TrimSpace(cue.Description)

	switch {
	case number != "" && description != "":
		return number + " - " + description
	case number != "":
		return number
	case description != "":
		return description
	default:
		return "Untitled cue"
	}
}

func cueDropdownSelectedIndex(items []input.DropdownItem, selectedCueID show.CueID) int {
	if selectedCueID != (show.CueID{}) {
		selectedValue := uuid.UUID(selectedCueID).String()
		for i, item := range items {
			if item.Value == selectedValue {
				return i
			}
		}
	}
	return 0
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
		play.SeekToMs = ptr(int64(state.integer["mediaCtrlSeekToMs"].Value))
	} else {
		play.SeekToMs = nil
	}
}

// Fixes being unable to do e.g. &int64(5) in Go. So you would do ptr(int64(5)) instead.
func ptr[T any](value T) *T {
	return &value
}
