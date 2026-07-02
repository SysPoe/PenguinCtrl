package input

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/SysPoe/CuSus/utils"
)

type Dropdown struct {
	Items    []DropdownItem
	Selected int
	expanded bool

	expandedBtn widget.Clickable
	choicesBtns []widget.Clickable

	eventListeners []func(selectedIndex int, selectedValue DropdownItem)
}

type DropdownItem struct {
	Label string
	Value string
}

func (d *Dropdown) getSelectedLabel() string {
	if d.Selected >= 0 && d.Selected < len(d.Items) {
		return d.Items[d.Selected].Label
	}
	return ""
}

func NewDropdown(items []DropdownItem, selected int) *Dropdown {
	return &Dropdown{
		Items:       items,
		Selected:    selected,
		choicesBtns: make([]widget.Clickable, len(items)),
		expandedBtn: widget.Clickable{},
	}
}

func (d *Dropdown) AddEventListener(listener func(selectedIndex int, selectedValue DropdownItem)) {
	d.eventListeners = append(d.eventListeners, listener)
}

func (d *Dropdown) notifyEventListeners() {
	for _, listener := range d.eventListeners {
		listener(d.Selected, d.Items[d.Selected])
	}
}

func (d *Dropdown) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	regBg := inputSurface(th)
	selBg := selectedInputSurface(th)
	nonSelBg := inputSurface(th)

	if d.expandedBtn.Clicked(gtx) {
		d.expanded = !d.expanded
	}

	subs := []layout.FlexChild{
		fixedWidthBtnWithColor(th, &d.expandedBtn, d.getSelectedLabel()+utils.Ter(d.expanded, "▼", " ▶"), inputDefaultWidth, regBg),
	}

	if d.expanded {
		for i, item := range d.Items {
			if d.choicesBtns[i].Clicked(gtx) {
				d.Selected = i
				d.expanded = false
				d.notifyEventListeners()
			}
			if d.Selected == i {
				subs = append(subs, fixedWidthBtnWithColor(th, &d.choicesBtns[i], item.Label, inputDefaultWidth, selBg))
			} else {
				subs = append(subs, fixedWidthBtnWithColor(th, &d.choicesBtns[i], item.Label, inputDefaultWidth, nonSelBg))
			}
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		subs...,
	)
}

func fixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width unit.Dp, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		setFixedWidth(&gtx, width)
		btn := material.ButtonLayout(th, wid)
		btn.Background = bgColor
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				label := material.Body1(th, txt)
				label.Alignment = text.Middle
				label.Color = inputTextColor(th)
				label.TextSize = unit.Sp(18)
				return layoutStableText(gtx, label.Layout)
			})
		})
	})
}

func setFixedWidth(gtx *layout.Context, width unit.Dp) {
	widthPx := gtx.Dp(width)
	widthPx = max(widthPx, gtx.Constraints.Min.X)
	widthPx = min(widthPx, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
}
