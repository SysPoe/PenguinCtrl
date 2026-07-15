package input

import (
	"image/color"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/primitives"
	"github.com/syspoe/cusus/utils"
)

type Dropdown struct {
	Items    []DropdownItem
	Selected int
	expanded bool

	expandedBtn widget.Clickable
	focus       bool
	choicesBtns []widget.Clickable

	changeListener func(selectedIndex int, selectedValue DropdownItem)
	eventListeners []func(selectedIndex int, selectedValue DropdownItem)
}

func (d *Dropdown) Focus() {
	d.focus = true
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
	}
}

func (d *Dropdown) SetItems(items []DropdownItem, selected int) {
	d.Items = items
	if len(d.choicesBtns) != len(items) {
		d.choicesBtns = make([]widget.Clickable, len(items))
	}
	if len(items) == 0 {
		d.Selected = -1
		d.expanded = false
		return
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	d.Selected = selected
}

func (d *Dropdown) AddEventListener(listener func(selectedIndex int, selectedValue DropdownItem)) {
	d.eventListeners = append(d.eventListeners, listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (d *Dropdown) SetChangeListener(listener func(selectedIndex int, selectedValue DropdownItem)) {
	d.changeListener = listener
}

func (d *Dropdown) notifyEventListeners() {
	if d.Selected < 0 || d.Selected >= len(d.Items) {
		return
	}
	if d.changeListener != nil {
		d.changeListener(d.Selected, d.Items[d.Selected])
	}
	for _, listener := range d.eventListeners {
		listener(d.Selected, d.Items[d.Selected])
	}
}

func (d *Dropdown) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	regularBackground := palette.Surface
	selBg := palette.SurfaceRaised

	if d.expandedBtn.Clicked(gtx) {
		d.expanded = !d.expanded
	}

	subs := []layout.FlexChild{
		fixedWidthBtnWithColor(th, &d.expandedBtn, d.getSelectedLabel()+utils.Ter(d.expanded, " ▼", " ▶"), inputDefaultWidth, regularBackground),
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
				subs = append(subs, fixedWidthBtnWithColor(th, &d.choicesBtns[i], item.Label, inputDefaultWidth, regularBackground))
			}
		}
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		subs...,
	)
	if d.focus {
		gtx.Execute(key.FocusCmd{Tag: &d.expandedBtn})
		d.focus = false
	}
	return dims
}

func fixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width unit.Dp, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		primitives.SetFixedWidth(&gtx, width)
		btn := material.ButtonLayout(th, wid)
		btn.Background = bgColor
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				label := material.Body1(th, txt)
				label.Alignment = text.Middle
				label.Color = palette.Text
				label.TextSize = unit.Sp(18)
				return primitives.StableText(gtx, label.Layout)
			})
		})
	})
}
