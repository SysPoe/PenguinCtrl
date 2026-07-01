package input

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Checkbox struct {
	Label   string
	Checked bool

	checkbox widget.Bool

	eventListeners []func(checked bool)
}

func NewCheckbox(label string, checked bool) *Checkbox {
	return &Checkbox{
		Label:   label,
		Checked: checked,
		checkbox: widget.Bool{
			Value: checked,
		},
	}
}

func (c *Checkbox) AddEventListener(listener func(checked bool)) {
	c.eventListeners = append(c.eventListeners, listener)
}

func (c *Checkbox) notifyEventListeners() {
	for _, listener := range c.eventListeners {
		listener(c.Checked)
	}
}

func (c *Checkbox) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if c.checkbox.Value != c.Checked {
		c.checkbox.Value = c.Checked
	}

	previous := c.checkbox.Value
	checkBox := material.CheckBox(th, &c.checkbox, c.Label)
	checkBox.Color = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xDE}
	checkBox.IconColor = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
	dims := checkBox.Layout(gtx)

	if c.checkbox.Value != previous {
		c.Checked = c.checkbox.Value
		c.notifyEventListeners()
	}

	return dims
}
