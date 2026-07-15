package ui

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

// Play-payload tabs build rows from the same typed field groups used by
// timecode marker actions. Keeping the schemas here makes payload coverage a
// compile-checked switch instead of duplicating field lists in tab renderers.
func (ctx *CueEditUI) renderMediaTab(th *material.Theme) layout.FlexChild {
	return ctx.renderForm(th, ctx.mediaTabRows(th))
}

func (ctx *CueEditUI) mediaTabRows(th *material.Theme) []cueEditFormRow {
	fields := ctx.page.media
	rows := make([]cueEditFormRow, 0, 7)
	if ctx.cue.Play.Sound != nil {
		play := ctx.cue.Play.Sound
		rows = append(rows, ctx.timedMediaTabRows(th, "audio", soundFileExtensions, &play.File, &play.OutputID, &play.ClipEndMs, &play.LevelDB)...)
	}
	if ctx.cue.Play.Video != nil {
		play := ctx.cue.Play.Video
		rows = append(rows, ctx.timedMediaTabRows(th, "video", videoFileExtensions, &play.File, &play.OutputID, &play.ClipEndMs, &play.LevelDB)...)
	}
	if ctx.cue.Play.Image != nil {
		play := ctx.cue.Play.Image
		rows = append(rows,
			ctx.fileRow(th, "File", "image", fields.file, fields.projectFile, fields.fileBrowse, imageFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", fields.outputID, func(value string) { play.OutputID = value }),
		)
		rows = append(rows, ctx.mediaRangeRows(th, mediaTabRangeLabels, false)...)
	}
	if len(rows) == 0 {
		return []cueEditFormRow{staticRow(th, "Media", "No media settings for this cue type.")}
	}
	return rows
}

func (ctx *CueEditUI) timedMediaTabRows(th *material.Theme, kind string, extensions []string, file, outputID *string, clipEndMs *int64, levelDB *float64) []cueEditFormRow {
	fields := ctx.page.media
	rows := []cueEditFormRow{
		ctx.fileRow(th, "File", kind, fields.file, fields.projectFile, fields.fileBrowse, extensions, func(value string) {
			ctx.setTimecodeMediaSource(file, clipEndMs, fields.clipEndMs, value)
		}),
		textRow(th, "Output ID", fields.outputID, func(value string) { *outputID = value }),
	}
	rows = append(rows, ctx.mediaRangeRows(th, mediaTabRangeLabels, false)...)
	return append(rows, floatRow(th, "Level dB", fields.levelDB, func(value float64) { *levelDB = value }))
}

type mediaRangeLabels struct {
	clipStart string
	clipEnd   string
	fadeIn    string
	fadeOut   string
	duration  string
}

var (
	mediaTabRangeLabels = mediaRangeLabels{
		clipStart: "Clip Start MS",
		clipEnd:   "Clip End MS",
		fadeIn:    "Fade In MS",
		fadeOut:   "Fade Out MS",
		duration:  "Duration MS",
	}
	timecodeRangeLabels = mediaRangeLabels{
		clipStart: "Clip start",
		clipEnd:   "Clip end",
		fadeIn:    "Fade in",
		fadeOut:   "Fade out",
		duration:  "Duration",
	}
)

func (ctx *CueEditUI) mediaRangeRows(th *material.Theme, labels mediaRangeLabels, nonNegative bool) []cueEditFormRow {
	fields := ctx.page.media
	if fields == nil {
		return nil
	}
	switch {
	case ctx.cue.Play.Sound != nil:
		play := ctx.cue.Play.Sound
		return ctx.timedMediaRangeRows(th, labels, fields, &play.FadeInMs, &play.FadeOutMs, nonNegative)
	case ctx.cue.Play.Video != nil:
		play := ctx.cue.Play.Video
		return ctx.timedMediaRangeRows(th, labels, fields, &play.FadeInMs, &play.FadeOutMs, nonNegative)
	case ctx.cue.Play.Image != nil:
		play := ctx.cue.Play.Image
		value := nonNegativeInt64(nonNegative)
		return []cueEditFormRow{
			integerRow(th, labels.duration, fields.durationMs, func(v int) { play.DurationMs = value(v) }),
			integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { play.FadeInMs = value(v) }),
			integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { play.FadeOutMs = value(v) }),
		}
	default:
		return nil
	}
}

func (ctx *CueEditUI) timedMediaRangeRows(th *material.Theme, labels mediaRangeLabels, fields *mediaPlayInputs, fadeInMs, fadeOutMs *int64, nonNegative bool) []cueEditFormRow {
	value := nonNegativeInt64(nonNegative)
	return []cueEditFormRow{
		integerRow(th, labels.clipStart, fields.clipStartMs, func(v int) { ctx.setTimecodeClipStart(int64(v)) }),
		integerRow(th, labels.clipEnd, fields.clipEndMs, func(v int) { ctx.setTimecodeClipEnd(int64(v)) }),
		integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { *fadeInMs = value(v) }),
		integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { *fadeOutMs = value(v) }),
	}
}

func nonNegativeInt64(nonNegative bool) func(int) int64 {
	if nonNegative {
		return func(value int) int64 { return int64(max(0, value)) }
	}
	return func(value int) int64 { return int64(value) }
}

type remoteFormLabels struct {
	protocol  string
	action    string
	playback  string
	cueNumber string
	level     string
	custom    string
}

var (
	cueRemoteFormLabels = remoteFormLabels{
		protocol: "Protocol", action: "Action", playback: "Playback",
		cueNumber: "Cue Number", level: "Level", custom: "Custom Command",
	}
	markerRemoteFormLabels = remoteFormLabels{
		protocol: "Protocol", action: "Remote action", playback: "Playback",
		cueNumber: "Cue number", level: "Level", custom: "Command",
	}
)

func remoteFormRows(th *material.Theme, fields *cueRemoteInputs, play *show.RemotePlay, labels remoteFormLabels) []cueEditFormRow {
	rows := []cueEditFormRow{
		dropdownRow(th, labels.protocol, fields.protocol, func(selected int) { play.Protocol = show.RemoteProtocol(selected) }),
		dropdownRow(th, labels.action, fields.action, func(selected int) { play.Action = show.RemoteAction(selected) }),
		textRow(th, labels.playback, fields.playback, func(value string) { play.Playback = value }),
		textRow(th, labels.cueNumber, fields.cueNumber, func(value string) { play.CueNumber = value }),
		textRow(th, labels.level, fields.level, func(value string) { play.Level = value }),
	}
	if play.Action == show.RemoteActionCustom {
		rows = append(rows, textRow(th, labels.custom, fields.custom, func(value string) { play.Custom = value }))
	}
	return rows
}

func (ctx *CueEditUI) renderRemoteTab(th *material.Theme) layout.FlexChild {
	play := ctx.cue.Play.Remote
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Remote", "No remote settings for this cue type.")})
	}
	return ctx.renderForm(th, remoteFormRows(th, ctx.page.remote, play, cueRemoteFormLabels))
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.Wait
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Wait", "No wait settings for this cue type.")})
	}
	return ctx.renderForm(th, ctx.waitFormRows(th, manager, ctx.page.wait, play))
}

func (ctx *CueEditUI) waitFormRows(th *material.Theme, manager *show.ShowManager, fields *cueWaitInputs, play *show.WaitPlay) []cueEditFormRow {
	rows := []cueEditFormRow{
		dropdownRow(th, "Kind", fields.kind, func(selected int) { play.Kind = show.WaitKind(selected) }),
	}
	if play.Kind == show.WaitDuration {
		rows = append(rows, integerRow(th, "Duration MS", fields.durationMs, func(value int) { play.DurationMs = int64(value) }))
	} else if waitKindUsesMediaTarget(play.Kind) {
		rows = ctx.appendMediaTargetRows(rows, th, manager, &fields.target, &play.Media)
	}
	return rows
}

type outputControlFormLabels struct {
	action, outputID, fadeOut, fadeIn, message string
}

var (
	cueOutputControlFormLabels = outputControlFormLabels{
		action: "Action", outputID: "Output ID", fadeOut: "Fade Out MS", fadeIn: "Fade In MS", message: "Message",
	}
	markerOutputControlFormLabels = outputControlFormLabels{
		action: "Output action", outputID: "Output", fadeOut: "Fade out", fadeIn: "Fade in", message: "Message",
	}
)

func outputControlFormRows(th *material.Theme, fields *cueOutputControlInputs, play *show.OutputControlPlay, labels outputControlFormLabels, nonNegative bool) []cueEditFormRow {
	value := nonNegativeInt64(nonNegative)
	return []cueEditFormRow{
		dropdownRow(th, labels.action, fields.action, func(selected int) { play.Action = show.OutputControlAction(selected) }),
		textRow(th, labels.outputID, fields.outputID, func(value string) { play.OutputID = value }),
		integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { play.FadeOutMs = value(v) }),
		integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { play.FadeInMs = value(v) }),
		textRow(th, labels.message, fields.message, func(value string) { play.Message = value }),
	}
}

func (ctx *CueEditUI) renderOutputCtrlTab(th *material.Theme) layout.FlexChild {
	play := ctx.cue.Play.OutputControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Output Control", "No output control settings for this cue type.")})
	}
	return ctx.renderForm(th, outputControlFormRows(th, ctx.page.outputControl, play, cueOutputControlFormLabels, false))
}

type mediaControlFormLabels struct {
	action, level, seek, fade, curve string
}

var (
	cueMediaControlFormLabels = mediaControlFormLabels{
		action: "Action", level: "Level dB", seek: "Seek To MS", fade: "Fade MS", curve: "Curve",
	}
	markerMediaControlFormLabels = mediaControlFormLabels{
		action: "Track action", level: "Level dB", seek: "Seek to", fade: "Fade time", curve: "Curve",
	}
)

func mediaControlActionRow(th *material.Theme, fields *cueMediaControlInputs, play *show.MediaControlPlay, label string, apply func()) cueEditFormRow {
	return dropdownRow(th, label, fields.action, func(selected int) {
		play.Action = show.MediaControlAction(selected)
		apply()
	})
}

func mediaControlDetailRows(th *material.Theme, fields *cueMediaControlInputs, play *show.MediaControlPlay, labels mediaControlFormLabels, nonNegative bool) []cueEditFormRow {
	value := nonNegativeInt64(nonNegative)
	rows := make([]cueEditFormRow, 0, 4)
	if mediaControlActionUsesLevel(play.Action) {
		rows = append(rows, floatRow(th, labels.level, fields.levelDB, func(v float64) { play.LevelDB = &v }))
	}
	if play.Action == show.MediaControlSeek {
		rows = append(rows, integerRow(th, labels.seek, fields.seekToMs, func(v int) { seek := value(v); play.SeekToMs = &seek }))
	}
	return append(rows,
		integerRow(th, labels.fade, fields.fadeMs, func(v int) { play.FadeMs = value(v) }),
		dropdownRow(th, labels.curve, fields.curve, func(selected int) { play.Curve = show.FadeCurve(selected) }),
	)
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.MediaControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Media Control", "No media control settings for this cue type.")})
	}
	fields := ctx.page.mediaControl
	rows := []cueEditFormRow{mediaControlActionRow(th, fields, play, cueMediaControlFormLabels.action, func() {
		syncMediaControlOptionals(play, fields)
	})}
	rows = ctx.appendMediaTargetRows(rows, th, manager, &fields.target, &play.Target)
	rows = append(rows, mediaControlDetailRows(th, fields, play, cueMediaControlFormLabels, false)...)
	return ctx.renderForm(th, rows)
}
