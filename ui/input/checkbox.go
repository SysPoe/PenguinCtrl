package input

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
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
	checkBox.Color = palette.TextSoft
	checkBox.IconColor = selectedInputSurface(th)
	dims := inputField(th, gtx, checkBox.Layout)

	if c.checkbox.Value != previous {
		c.Checked = c.checkbox.Value
		c.notifyEventListeners()
	}

	return dims
}
