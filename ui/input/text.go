package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Text struct {
	Label string
	Hint  string
	Value string

	editor widget.Editor
	focus  bool

	eventListeners []func(value string)
}

func NewText(label, value string) *Text {
	t := &Text{
		Label: label,
		Hint:  label,
		Value: value,
	}
	t.editor.SingleLine = true
	t.editor.InputHint = key.HintText
	t.editor.SetText(value)
	return t
}

func (t *Text) AddEventListener(listener func(value string)) {
	t.eventListeners = append(t.eventListeners, listener)
}

// Focus selects the field contents and requests keyboard focus.
func (t *Text) Focus() {
	t.editor.SetCaret(0, t.editor.Len())
	t.focus = true
}

func (t *Text) notifyEventListeners() {
	for _, listener := range t.eventListeners {
		listener(t.Value)
	}
}

func (t *Text) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if t.editor.Text() != t.Value && !gtx.Focused(&t.editor) {
		t.editor.SetText(t.Value)
	}

	previous := t.editor.Text()
	editor := material.Editor(th, &t.editor, t.Hint)
	editor.TextSize = unit.Sp(18)
	dims := editorField(th, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
	})
	if t.focus {
		gtx.Execute(key.FocusCmd{Tag: &t.editor})
		t.focus = false
	}

	if value := t.editor.Text(); value != previous {
		t.Value = value
		t.notifyEventListeners()
	}

	return dims
}
