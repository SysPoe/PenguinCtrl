package input

import (
	"image/color"

	"gioui.org/layout"
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
	regBg := color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}
	selBg := color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
	nonSelBg := color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}

	if d.expandedBtn.Clicked(gtx) {
		d.expanded = !d.expanded
	}

	subs := []layout.FlexChild{
		fixedWidthBtnWithColor(th, &d.expandedBtn, d.getSelectedLabel()+utils.Ter(d.expanded, "▼", " ▶"), 200, regBg),
	}

	if d.expanded {
		for i, item := range d.Items {
			if d.choicesBtns[i].Clicked(gtx) {
				d.Selected = i
				d.expanded = false
				d.notifyEventListeners()
			}
			if d.Selected == i {
				subs = append(subs, fixedWidthBtnWithColor(th, &d.choicesBtns[i], item.Label, 200, selBg))
			} else {
				subs = append(subs, fixedWidthBtnWithColor(th, &d.choicesBtns[i], item.Label, 200, nonSelBg))
			}
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		subs...,
	)
}

func fixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width int, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		setFixedWidth(&gtx, width)
		return btn.Layout(gtx)
	})
}

func setFixedWidth(gtx *layout.Context, width int) {
	widthPx := gtx.Dp(unit.Dp(width))
	widthPx = max(widthPx, gtx.Constraints.Min.X)
	widthPx = min(widthPx, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
}
