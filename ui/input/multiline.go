package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Multiline struct {
	Label string
	Hint  string
	Value string

	editor widget.Editor

	eventListeners []func(value string)
}

func NewMultiline(label, value string) *Multiline {
	m := &Multiline{
		Label: label,
		Hint:  label,
		Value: value,
	}
	m.editor.InputHint = key.HintText
	m.editor.SetText(value)
	return m
}

func (m *Multiline) AddEventListener(listener func(value string)) {
	m.eventListeners = append(m.eventListeners, listener)
}

func (m *Multiline) notifyEventListeners() {
	for _, listener := range m.eventListeners {
		listener(m.Value)
	}
}

func (m *Multiline) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if m.editor.Text() != m.Value && !gtx.Focused(&m.editor) {
		m.editor.SetText(m.Value)
	}

	previous := m.editor.Text()
	editor := material.Editor(th, &m.editor, m.Hint)
	// TODO(micro): TextSize Sp(18) and inset Dp(8) are duplicated across Text/Integer/Float/Multiline; name shared editorTextSize/editorInset consts
	editor.TextSize = unit.Sp(18)
	dims := editorField(th, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
	})

	if value := m.editor.Text(); value != previous {
		m.Value = value
		m.notifyEventListeners()
	}

	return dims
}
