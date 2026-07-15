package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TODO(macro): Text/Integer/Float/Multiline each reimplement Value↔editor sync, focus request, and listener fan-out with only the parse step differing. Introduce a shared editor-field base (or generic) so the input kit's contract is one pattern, then keep typed wrappers thin.
type Text struct {
	Label string
	Hint  string
	Value string

	editor widget.Editor
	focus  bool

	changeListener func(value string)
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

// SetChangeListener replaces the field's single model-binding callback.
func (t *Text) SetChangeListener(listener func(value string)) {
	t.changeListener = listener
}

// Focus selects the field contents and requests keyboard focus.
func (t *Text) Focus() {
	t.editor.SetCaret(0, t.editor.Len())
	t.focus = true
}

func (t *Text) notifyEventListeners() {
	if t.changeListener != nil {
		t.changeListener(t.Value)
	}
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
	// TODO(micro): TextSize Sp(18) and inset Dp(8) are duplicated across Text/Integer/Float/Multiline; name shared editorTextSize/editorInset consts
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
