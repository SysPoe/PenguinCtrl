package input

import (
	"strconv"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Float struct {
	Label string
	Hint  string
	Value float64

	editor widget.Editor
	text   string

	eventListeners []func(value float64)
}

func NewFloat(label string, value float64) *Float {
	f := &Float{
		Label: label,
		Hint:  label,
		Value: value,
		text:  strconv.FormatFloat(value, 'f', -1, 64),
	}
	f.editor.SingleLine = true
	f.editor.InputHint = key.HintNumeric
	f.editor.Filter = "-0123456789."
	f.editor.SetText(f.text)
	return f
}

func (f *Float) AddEventListener(listener func(value float64)) {
	f.eventListeners = append(f.eventListeners, listener)
}

func (f *Float) notifyEventListeners() {
	for _, listener := range f.eventListeners {
		listener(f.Value)
	}
}

func (f *Float) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if expected := strconv.FormatFloat(f.Value, 'f', -1, 64); f.text != expected && !gtx.Focused(&f.editor) {
		f.text = expected
		f.editor.SetText(f.text)
	}

	previous := f.editor.Text()
	editor := material.Editor(th, &f.editor, f.Hint)
	editor.TextSize = unit.Sp(18)
	dims := editorField(th, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
	})

	if text := f.editor.Text(); text != previous {
		if !f.applyText(text) {
			f.editor.SetText(previous)
		}
	}

	return dims
}

func validFloatInput(text string) bool {
	switch text {
	case "", "-", ".", "-.":
		return true
	}
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

func (f *Float) applyText(text string) bool {
	if !validFloatInput(text) {
		return false
	}
	f.text = text
	if value, err := strconv.ParseFloat(text, 64); err == nil && value != f.Value {
		f.Value = value
		f.notifyEventListeners()
	}
	return true
}
