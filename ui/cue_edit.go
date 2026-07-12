package ui

import (
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/utils"
)

type CueEditUI struct {
	cue   show.Cue
	cType show.CueType
	show  bool
	isNew bool

	pickFile       func(kind string, extensions []string, selected func(path string))
	projectFiles   func(kind string) []ProjectFile
	loadWaveform   func(source string, completed func(samples []float32, sampleRate int, durationMs int64, err error))
	togglePreview  func(cue show.Cue) (bool, error)
	stopPreview    func()
	problemsForCue func(show.Cue) []show.CueProblem

	btnTabGeneral    widget.Clickable
	btnTabTiming     widget.Clickable
	btnTabLink       widget.Clickable
	btnTabMedia      widget.Clickable
	btnTabTimecode   widget.Clickable
	btnTabRemote     widget.Clickable
	btnTabWait       widget.Clickable
	btnTabMediaCtrl  widget.Clickable
	btnTabOutputCtrl widget.Clickable

	btnCancel widget.Clickable
	btnSave   widget.Clickable

	activeTab       int
	focusFirstInput bool

	modalTag struct{}
	page     cueEditPageState
	timeline timecodeTimelineState
}

type ProjectFile struct {
	Name string
	Path string
}

const (
	tabGeneral = iota
	tabTiming
	tabLink
	tabMedia
	tabTimecode
	tabRemote
	tabWait
	tabMediaCtrl
	tabOutputCtrl
)

func (ctx *CueEditUI) drawTopBar(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		barHeight := gtx.Dp(unit.Dp(topBarHeight))

		colorActive := palette.Primary
		colorInactive := palette.SurfaceRaised
		colorBg := palette.Surface

		gtx.Constraints.Min.Y = barHeight
		gtx.Constraints.Max.Y = barHeight

		// Make bg
		paint.FillShape(
			gtx.Ops, colorBg,
			clip.Rect{Max: image.Point{
				X: gtx.Constraints.Max.X,
				Y: barHeight,
			}}.Op(),
		)

		sub := []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				titleText := "Edit Cue"
				if ctx.isNew {
					titleText = "Add Cue"
				}
				title := stableBody1(th, titleText)
				title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
				return layoutStableText(gtx, title.Layout)
			}),
			makeBtnWithColor(th, &ctx.btnTabGeneral, "General", utils.Ter(ctx.activeTab == tabGeneral, colorActive, colorInactive)),
			makeBtnWithColor(th, &ctx.btnTabTiming, "Timing", utils.Ter(ctx.activeTab == tabTiming, colorActive, colorInactive)),
			makeBtnWithColor(th, &ctx.btnTabLink, "Link", utils.Ter(ctx.activeTab == tabLink, colorActive, colorInactive)),
		}

		if ctx.cType == show.CueTypeImage || ctx.cType == show.CueTypeVideo || ctx.cType == show.CueTypeSound {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabMedia, "Media", utils.Ter(ctx.activeTab == tabMedia, colorActive, colorInactive)))
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabTimecode, "Timecode", utils.Ter(ctx.activeTab == tabTimecode, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeRemote {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabRemote, "Remote", utils.Ter(ctx.activeTab == tabRemote, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeWait {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabWait, "Wait", utils.Ter(ctx.activeTab == tabWait, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeMediaControl {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabMediaCtrl, "Media Ctrl", utils.Ter(ctx.activeTab == tabMediaCtrl, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeOutputControl {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabOutputCtrl, "Output Ctrl", utils.Ter(ctx.activeTab == tabOutputCtrl, colorActive, colorInactive)))
		}

		if ctx.btnTabGeneral.Clicked(gtx) {
			ctx.activeTab = tabGeneral
		}
		if ctx.btnTabTiming.Clicked(gtx) {
			ctx.activeTab = tabTiming
		}
		if ctx.btnTabLink.Clicked(gtx) {
			ctx.activeTab = tabLink
		}
		if ctx.btnTabMedia.Clicked(gtx) {
			ctx.activeTab = tabMedia
		}
		if ctx.btnTabTimecode.Clicked(gtx) {
			ctx.activeTab = tabTimecode
		}
		if ctx.btnTabRemote.Clicked(gtx) {
			ctx.activeTab = tabRemote
		}
		if ctx.btnTabWait.Clicked(gtx) {
			ctx.activeTab = tabWait
		}
		if ctx.btnTabMediaCtrl.Clicked(gtx) {
			ctx.activeTab = tabMediaCtrl
		}
		if ctx.btnTabOutputCtrl.Clicked(gtx) {
			ctx.activeTab = tabOutputCtrl
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			sub...,
		)
	})
}

func (ctx *CueEditUI) drawBottomBar(th *material.Theme, gtx layout.Context, manager *show.ShowManager, saveShortcut bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ctx.btnCancel.Clicked(gtx) {
			ctx.stopTimecodePreview()
			ctx.show = false
			gtx.Execute(key.FocusCmd{})
		}

		if ctx.btnSave.Clicked(gtx) || saveShortcut {
			ctx.stopTimecodePreview()
			show.RepairCueData(&ctx.cue)
			if markers := cueTimecodeMarkers(&ctx.cue); markers != nil {
				sortTimecodeMarkers(markers)
			}
			if ctx.isNew {
				manager.AddCueAndSelect(ctx.cue)
			} else {
				manager.ReplaceCue(ctx.cue)
			}
			ctx.isNew = false
			ctx.show = false
			gtx.Execute(key.FocusCmd{})
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			makeFlexedBtnWithColor(th, &ctx.btnCancel, "Cancel", palette.Danger, 1),
			makeFlexedBtnWithColor(th, &ctx.btnSave, "Save", palette.Success, 1),
		)
	})
}

func (ctx *CueEditUI) drawProblemBar(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	problems := show.CueProblems(ctx.cue, manager.Snapshot())
	if ctx.problemsForCue != nil {
		problems = ctx.problemsForCue(ctx.cue)
	}
	actionable := make([]show.CueProblem, 0, len(problems))
	for _, problem := range problems {
		if problem.Severity != show.ProblemState {
			actionable = append(actionable, problem)
		}
	}
	if len(actionable) == 0 {
		return layout.Rigid(func(layout.Context) layout.Dimensions { return layout.Dimensions{} })
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		accent := palette.Primary
		for _, problem := range actionable {
			if problem.Severity == show.ProblemBlocker {
				accent = palette.Danger
				break
			}
			if problem.Severity == show.ProblemCaution {
				accent = palette.Warning
			}
		}
		lines := make([]string, 0, len(actionable))
		for _, problem := range actionable {
			line := problem.Severity.Label() + " · " + problem.Message
			if problem.Fix != "" {
				line += " — " + problem.Fix
			}
			lines = append(lines, line)
		}
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, palette.SurfaceSunken, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(th, strings.Join(lines, "\n"))
					label.Color = accent
					return label.Layout(gtx)
				})
			},
		)
	})
}

func (ctx *CueEditUI) toggleTimecodePreview() {
	if ctx.togglePreview == nil || ctx.cue.Play.Sound == nil {
		return
	}
	playing, err := ctx.togglePreview(ctx.cue)
	if err == nil {
		ctx.timeline.previewing = playing
	}
}

func (ctx *CueEditUI) stopTimecodePreview() {
	if ctx.stopPreview != nil {
		ctx.stopPreview()
	}
	ctx.timeline.previewing = false
}

func cueEditorShortcuts(gtx layout.Context) (save, preview bool, tabOffset int) {
	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameLeftArrow, Required: key.ModShortcut},
			key.Filter{Name: key.NameRightArrow, Required: key.ModShortcut},
		)
		if !ok {
			return save, preview, tabOffset
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			switch event.Name {
			case key.NameSpace:
				preview = true
			case key.NameLeftArrow:
				tabOffset--
			case key.NameRightArrow:
				tabOffset++
			default:
				save = true
			}
		}
	}
}

func (ctx *CueEditUI) moveTab(offset int) {
	if offset == 0 {
		return
	}
	tabs := []int{tabGeneral, tabTiming, tabLink}
	switch ctx.cType {
	case show.CueTypeImage, show.CueTypeVideo, show.CueTypeSound:
		tabs = append(tabs, tabMedia, tabTimecode)
	case show.CueTypeRemote:
		tabs = append(tabs, tabRemote)
	case show.CueTypeWait:
		tabs = append(tabs, tabWait)
	case show.CueTypeMediaControl:
		tabs = append(tabs, tabMediaCtrl)
	case show.CueTypeOutputControl:
		tabs = append(tabs, tabOutputCtrl)
	}
	for i, tab := range tabs {
		if tab == ctx.activeTab {
			next := (i + offset) % len(tabs)
			if next < 0 {
				next += len(tabs)
			}
			ctx.activeTab = tabs[next]
			ctx.focusFirstInput = true
			return
		}
	}
	ctx.activeTab = tabs[0]
}

func (ctx *CueEditUI) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.show {
		return layout.Dimensions{}
	}
	saveShortcut, previewShortcut, tabOffset := cueEditorShortcuts(gtx)
	ctx.moveTab(tabOffset)
	if previewShortcut && ctx.activeTab == tabTimecode && ctx.cue.Play.Sound != nil {
		ctx.toggleTimecodePreview()
	}

	margin := image.Pt(0, 0)
	widthHeight := image.Pt(gtx.Constraints.Max.X-margin.X*2, gtx.Constraints.Max.Y-margin.Y*2)
	borderWidth := gtx.Dp(unit.Dp(2))
	borderRadius := gtx.Dp(unit.Dp(0))
	padding := 0

	// Draw border and background
	defer op.Offset(image.Pt(
		margin.X-borderWidth, margin.Y-borderWidth,
	)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, th.ContrastBg, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(widthHeight.X+borderWidth*2, widthHeight.Y+borderWidth*2)},
		SE:   borderRadius + borderWidth,
		SW:   borderRadius + borderWidth,
		NW:   borderRadius + borderWidth,
		NE:   borderRadius + borderWidth,
	}.Op(gtx.Ops))

	// Prevent clicks from going through to the underlying UI
	hitArea := clip.Rect(image.Rectangle{Max: image.Pt(widthHeight.X+borderWidth*2, widthHeight.Y+borderWidth*2)}).Push(gtx.Ops)
	event.Op(gtx.Ops, &ctx.modalTag)
	hitArea.Pop()
	for {
		_, ok := gtx.Event(pointer.Filter{
			Target:  &ctx.modalTag,
			Kinds:   pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Enter | pointer.Leave | pointer.Cancel,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
	}

	defer op.Offset(image.Pt(borderWidth, borderWidth)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, th.Bg, clip.RRect{
		Rect: image.Rectangle{Max: widthHeight},
		SE:   borderRadius,
		SW:   borderRadius,
		NW:   borderRadius,
		NE:   borderRadius,
	}.Op(gtx.Ops))

	defer op.Offset(image.Pt(padding, padding)).Push(gtx.Ops).Pop()
	gtx.Constraints.Min.X = widthHeight.X - padding*2
	gtx.Constraints.Max.X = widthHeight.X - padding*2
	gtx.Constraints.Min.Y = widthHeight.Y - padding*2
	gtx.Constraints.Max.Y = widthHeight.Y - padding*2

	// Return acutal layout
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		ctx.drawTopBar(th, gtx),
		ctx.drawProblemBar(th, gtx, manager),
		ctx.drawBody(th, gtx, manager),
		ctx.drawBottomBar(th, gtx, manager, saveShortcut),
	)
}
