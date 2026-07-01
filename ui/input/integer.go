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

	editor widget.Editor
	text   string

	eventListeners []func(value int)
}

func NewInteger(label string, value int) *Integer {
	i := &Integer{
		Label: label,
		Hint:  label,
		Value: value,
		text:  strconv.Itoa(value),
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
	if expected := strconv.Itoa(i.Value); i.text != expected && !gtx.Focused(&i.editor) {
		i.text = expected
		i.editor.SetText(i.text)
	}

	previous := i.editor.Text()
	editor := material.Editor(th, &i.editor, i.Hint)
	editor.TextSize = unit.Sp(14)
	dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)

	if text := i.editor.Text(); text != previous {
		i.text = text
		if value, err := strconv.Atoi(text); err == nil && value != i.Value {
			i.Value = value
			i.notifyEventListeners()
		}
	}

	return dims
}
