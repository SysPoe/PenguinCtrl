package input

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
)

// Checkbox is a themed boolean field with model-binding and event callbacks.
type Checkbox struct {
	Label   string
	Checked bool

	checkbox    widget.Bool
	lastChecked bool

	changeListener func(checked bool)
	eventListeners []func(checked bool)
}

// NewCheckbox constructs a checkbox with its public and widget values aligned.
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

// AddEventListener appends a callback invoked for user-originated changes.
func (c *Checkbox) AddEventListener(listener func(checked bool)) {
	c.eventListeners = append(c.eventListeners, listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (c *Checkbox) SetChangeListener(listener func(checked bool)) {
	c.changeListener = listener
}

func (c *Checkbox) notifyEventListeners() {
	if c.changeListener != nil {
		c.changeListener(c.Checked)
	}
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

// Layout synchronizes the public value and renders the checkbox field.
func (c *Checkbox) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	// Only synchronize a value that changed through the public model. Keeping the
	// last synchronized value prevents a queued widget toggle from being replaced
	// merely because the internal and public values temporarily diverge.
	c.synchronize()

	previous := c.checkbox.Value
	checkBox := material.CheckBox(th, &c.checkbox, c.Label)
	accent := palette.Text
	checkBox.Color = accent
	checkBox.IconColor = accent
	dims := inputField(gtx, checkBox.Layout)

	if c.checkbox.Value != previous {
		c.Checked = c.checkbox.Value
		c.lastChecked = c.Checked
		c.notifyEventListeners()
	}

	return dims
}
