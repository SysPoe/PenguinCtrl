package ui

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/syspoe/cusus/palette"
)

const (
	modalDimmerAlpha = 0xB8
	dialogPanelWidth = unit.Dp(440)
)

type modalPanelStyle struct {
	width      unit.Dp
	height     unit.Dp
	minWidth   unit.Dp
	maxWidth   unit.Dp
	background color.NRGBA
	radius     unit.Dp
}

type modalLayer struct {
	tag struct{}
}

func (m *modalLayer) layout(gtx layout.Context, panel modalPanelStyle, content layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Black, modalDimmerAlpha), clip.Rect{Max: size}.Op())
	m.absorbInput(gtx, size)

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx = constrainModalPanel(gtx, panel)
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, panel.background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(panel.radius)).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			content,
		)
	})
}

func constrainModalPanel(gtx layout.Context, panel modalPanelStyle) layout.Context {
	if panel.width > 0 {
		width := min(gtx.Constraints.Max.X, gtx.Dp(panel.width))
		gtx.Constraints.Min.X = width
		gtx.Constraints.Max.X = width
	} else {
		maxWidth := gtx.Constraints.Max.X
		if panel.maxWidth > 0 {
			maxWidth = min(maxWidth, gtx.Dp(panel.maxWidth))
		}
		minWidth := gtx.Constraints.Min.X
		if panel.minWidth > 0 {
			minWidth = max(minWidth, min(maxWidth, gtx.Dp(panel.minWidth)))
		}
		gtx.Constraints.Min.X = minWidth
		gtx.Constraints.Max.X = maxWidth
	}
	if panel.height > 0 {
		height := min(gtx.Constraints.Max.Y, gtx.Dp(panel.height))
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
	}
	return gtx
}

func (m *modalLayer) absorbInput(gtx layout.Context, size image.Point) {
	hitArea := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &m.tag)
	hitArea.Pop()
	for {
		_, ok := gtx.Event(pointer.Filter{
			Target:  &m.tag,
			Kinds:   pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Enter | pointer.Leave | pointer.Cancel,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			return
		}
	}
}

type confirmationKeyAction uint8

const (
	confirmationKeyNone confirmationKeyAction = iota
	confirmationKeyCancel
	confirmationKeyAccept
)

func confirmationAction(name key.Name) confirmationKeyAction {
	switch name {
	case key.NameEscape:
		return confirmationKeyCancel
	case key.NameReturn, key.NameEnter:
		return confirmationKeyAccept
	default:
		return confirmationKeyNone
	}
}

func handleConfirmationKeys(gtx layout.Context, cancel, accept func()) {
	for {
		inputEvent, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			return
		}
		keyEvent, ok := inputEvent.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		switch confirmationAction(keyEvent.Name) {
		case confirmationKeyCancel:
			cancel()
		case confirmationKeyAccept:
			accept()
		}
		return
	}
}
