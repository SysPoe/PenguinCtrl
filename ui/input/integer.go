package input

import (
	"strconv"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Integer struct {
	Label string
	Hint  string
	Value int

	editor   widget.Editor
	text     string
	optional bool
	empty    bool
	focus    bool

	eventListeners []func(value int)
}

func (i *Integer) Focus() {
	i.editor.SetCaret(0, i.editor.Len())
	i.focus = true
}

func NewInteger(label string, value int) *Integer {
	return newInteger(label, value, false)
}

// NewOptionalInteger creates an integer input where an empty field represents 0.
func NewOptionalInteger(label string, value int) *Integer {
	return newInteger(label, value, true)
}

func newInteger(label string, value int, optional bool) *Integer {
	text := strconv.Itoa(value)
	if optional && value == 0 {
		text = ""
	}
	i := &Integer{
		Label:    label,
		Hint:     label,
		Value:    value,
		text:     text,
		optional: optional,
		empty:    optional && value == 0,
	}
	i.editor.SingleLine = true
	i.editor.InputHint = key.HintNumeric
	i.editor.Filter = "-0123456789"
	i.editor.SetText(i.text)
	return i
}

func (i *Integer) AddEventListener(listener func(value int)) {
	i.eventListeners = append(i.eventListeners, listener)
}

func (i *Integer) notifyEventListeners() {
	for _, listener := range i.eventListeners {
		listener(i.Value)
	}
}

func (i *Integer) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	expected := strconv.Itoa(i.Value)
	if i.optional && i.empty {
		expected = ""
	}
	if i.text != expected && !gtx.Focused(&i.editor) {
		i.text = expected
		i.editor.SetText(i.text)
	}

	previous := i.editor.Text()
	editor := material.Editor(th, &i.editor, i.Hint)
	editor.TextSize = unit.Sp(18)
	dims := editorField(th, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
	})
	if i.focus {
		gtx.Execute(key.FocusCmd{Tag: &i.editor})
		i.focus = false
	}

	if text := i.editor.Text(); text != previous {
		i.applyText(text)
	}

	return dims
}

func (i *Integer) applyText(text string) {
	i.text = text
	if i.optional && text == "" {
		changed := !i.empty || i.Value != 0
		i.empty = true
		i.Value = 0
		if changed {
			i.notifyEventListeners()
		}
		return
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return
	}
	changed := i.empty || value != i.Value
	i.empty = false
	i.Value = value
	if changed {
		i.notifyEventListeners()
	}
}
