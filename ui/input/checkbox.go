// TODO(micro): Add Go-style documentation for Checkbox and its exported constructor, listener, and layout API.
package input

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Checkbox struct {
	Label   string
	Checked bool

	checkbox    widget.Bool
	lastChecked bool

	eventListeners []func(checked bool)
}

func NewCheckbox(label string, checked bool) *Checkbox {
	return &Checkbox{
		Label:   label,
		Checked: checked,
		checkbox: widget.Bool{
			Value: checked,
		},
		lastChecked: checked,
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

func (c *Checkbox) synchronize() {
	if c.Checked != c.lastChecked {
		c.checkbox.Value = c.Checked
		c.lastChecked = c.Checked
		return
	}
	if c.checkbox.Value != c.lastChecked {
		c.Checked = c.checkbox.Value
		c.lastChecked = c.Checked
		c.notifyEventListeners()
	}
}

func (c *Checkbox) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	// Only synchronize a value that changed through the public model. Keeping the
	// last synchronized value prevents a queued widget toggle from being replaced
	// merely because the internal and public values temporarily diverge.
	c.synchronize()

	previous := c.checkbox.Value
	checkBox := material.CheckBox(th, &c.checkbox, c.Label)
	accent := inputTextColor(th)
	checkBox.Color = accent
	checkBox.IconColor = accent
	dims := inputField(th, gtx, checkBox.Layout)

	if c.checkbox.Value != previous {
		c.Checked = c.checkbox.Value
		c.lastChecked = c.Checked
		c.notifyEventListeners()
	}

	return dims
}
