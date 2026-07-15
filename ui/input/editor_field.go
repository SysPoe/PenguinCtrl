package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	editorTextSize = unit.Sp(18)
	editorInset    = unit.Dp(8)
)

// editorFieldModel owns the widget lifecycle shared by typed editor inputs.
// Wrappers remain responsible for formatting and accepting typed values.
type editorFieldModel[T any] struct {
	editor widget.Editor
	text   string
	focus  bool

	changeListener func(value T)
	eventListeners []func(value T)
}

func newEditorFieldModel[T any](text string, singleLine bool, inputHint key.InputHint, filter string) editorFieldModel[T] {
	model := editorFieldModel[T]{text: text}
	model.editor.SingleLine = singleLine
	model.editor.InputHint = inputHint
	model.editor.Filter = filter
	model.editor.SetText(text)
	return model
}

func (m *editorFieldModel[T]) addEventListener(listener func(value T)) {
	m.eventListeners = append(m.eventListeners, listener)
}

func (m *editorFieldModel[T]) setChangeListener(listener func(value T)) {
	m.changeListener = listener
}

func (m *editorFieldModel[T]) notify(value T) {
	if m.changeListener != nil {
		m.changeListener(value)
	}
	for _, listener := range m.eventListeners {
		listener(value)
	}
}

func (m *editorFieldModel[T]) requestFocus() {
	m.editor.SetCaret(0, m.editor.Len())
	m.focus = true
}

func (m *editorFieldModel[T]) layout(th *material.Theme, gtx layout.Context, hint, expected string, accept func(string) bool) layout.Dimensions {
	if m.text != expected && !gtx.Focused(&m.editor) {
		m.setText(expected)
	}

	previous := m.editor.Text()
	editor := material.Editor(th, &m.editor, hint)
	editor.TextSize = editorTextSize
	dims := editorField(th, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(editorInset).Layout(gtx, editor.Layout)
	})
	if m.focus {
		gtx.Execute(key.FocusCmd{Tag: &m.editor})
		m.focus = false
	}

	if text := m.editor.Text(); text != previous {
		if accept(text) {
			m.text = text
		} else {
			m.editor.SetText(previous)
		}
	}
	return dims
}

func (m *editorFieldModel[T]) setText(text string) {
	m.text = text
	m.editor.SetText(text)
}
