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

// TODO(macro): CueEditUI is a multi-file god object (shell, typed page state, and
// the full timecode timeline). Carve the timeline into its own component
// with an explicit cue/media adapter, keep tab forms as pure field binders, and stop
// hanging waveform/preview/history methods on the editor shell.
type CueEditUI struct {
	cue   show.Cue
	show  bool
	isNew bool

	pickFile       func(kind string, extensions []string, selected func(path string))
	projectFiles   func(kind string) []ProjectFile
	loadWaveform   func(source string, completed func(samples []float32, sampleRate int, durationMs int64, err error))
	togglePreview  func(cue show.Cue) (bool, error)
	stopPreview    func()
	problemsForCue func(show.Cue) []show.CueProblem
	previewError   string

	btnCancel widget.Clickable
	btnSave   widget.Clickable

	modalTag struct{}
	page     cueEditPageState
	tabs     cueEditTabState
	timeline timecodeTimelineState
}

type ProjectFile struct {
	Name string
	Path string
}

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

		sub := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			titleText := "Edit Cue"
			if ctx.isNew {
				titleText = "Add Cue"
			}
			title := stableBody1(th, titleText)
			title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
			return layoutStableText(gtx, title.Layout)
		})}

		tabs := cueEditTabsForCueType(ctx.cue.Type)
		for _, tab := range tabs {
			button := ctx.tabs.button(tab.id)
			sub = append(sub, makeBtnWithColor(th, button, tab.label, utils.Ter(ctx.tabs.active == tab.id, colorActive, colorInactive)))
		}
		// Process every owned button to preserve queued clicks across a cue-type
		// switch, matching the previous fixed-button implementation.
		for tab := tabGeneral; tab < cueEditTabCount; tab++ {
			if ctx.tabs.button(tab).Clicked(gtx) {
				ctx.tabs.active = tab
			}
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			sub...,
		)
	})
}

func (ctx *CueEditUI) drawBottomBar(th *material.Theme, gtx layout.Context, manager *show.ShowManager, saveShortcut, cancelShortcut bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ctx.btnCancel.Clicked(gtx) || cancelShortcut {
			ctx.stopTimecodePreview()
			ctx.show = false
			gtx.Execute(key.FocusCmd{})
		} else if ctx.btnSave.Clicked(gtx) || saveShortcut {
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
	if len(actionable) == 0 && ctx.previewError == "" {
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
		lines := make([]string, 0, len(actionable)+1)
		if ctx.previewError != "" {
			accent = palette.Danger
			lines = append(lines, "PREVIEW · "+ctx.previewError)
		}
		for _, problem := range actionable {
			line := problem.Severity.Label() + " · " + problem.Message
			if problem.Fix != "" {
				line += " - " + problem.Fix
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
	if err != nil {
		ctx.timeline.previewing = false
		ctx.previewError = err.Error()
		return
	}
	ctx.previewError = ""
	ctx.timeline.previewing = playing
}

func (ctx *CueEditUI) stopTimecodePreview() {
	if ctx.stopPreview != nil {
		ctx.stopPreview()
	}
	ctx.timeline.previewing = false
	ctx.previewError = ""
}

func cueEditorShortcut(name key.Name) (save, cancel, preview bool, tabOffset int) {
	switch name {
	case key.NameEscape:
		cancel = true
	case "S":
		save = true
	case key.NameSpace:
		preview = true
	case key.NameLeftArrow:
		tabOffset = -1
	case key.NameRightArrow:
		tabOffset = 1
	}
	return
}

func cueEditorShortcuts(gtx layout.Context) (save, cancel, preview bool, tabOffset int) {
	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameLeftArrow, Required: key.ModShortcut},
			key.Filter{Name: key.NameRightArrow, Required: key.ModShortcut},
		)
		if !ok {
			return save, cancel, preview, tabOffset
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			eventSave, eventCancel, eventPreview, eventOffset := cueEditorShortcut(event.Name)
			save = save || eventSave
			cancel = cancel || eventCancel
			preview = preview || eventPreview
			tabOffset += eventOffset
		}
	}
}

func (ctx *CueEditUI) moveTab(offset int) {
	ctx.tabs.move(ctx.cue.Type, offset)
}

func (ctx *CueEditUI) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.show {
		return layout.Dimensions{}
	}
	saveShortcut, cancelShortcut, previewShortcut, tabOffset := cueEditorShortcuts(gtx)
	ctx.moveTab(tabOffset)
	if previewShortcut && ctx.tabs.active == tabTimecode && ctx.cue.Play.Sound != nil {
		ctx.toggleTimecodePreview()
	}

	// TODO(micro): margin/padding/borderRadius are all zero — dead math; drop vars or document why reserved.
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

	// TODO(micro): typo "acutal" → "actual"; comment is noise, can delete.
	// Return acutal layout
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		ctx.drawTopBar(th, gtx),
		ctx.drawProblemBar(th, gtx, manager),
		ctx.drawBody(th, manager),
		ctx.drawBottomBar(th, gtx, manager, saveShortcut, cancelShortcut),
	)
}
