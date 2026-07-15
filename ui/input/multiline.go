package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget/material"
)

type Multiline struct {
	Label string
	Hint  string
	Value string

	field editorFieldModel[string]
}

func NewMultiline(label, value string) *Multiline {
	m := &Multiline{
		Label: label,
		Hint:  label,
		Value: value,
		field: newEditorFieldModel[string](value, false, key.HintText, ""),
	}
	return m
}

func (m *Multiline) AddEventListener(listener func(value string)) {
	m.field.addEventListener(listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (m *Multiline) SetChangeListener(listener func(value string)) {
	m.field.setChangeListener(listener)
}

// Focus selects the field contents and requests keyboard focus.
func (m *Multiline) Focus() {
	m.field.requestFocus()
}

func (m *Multiline) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return m.field.layout(th, gtx, m.Hint, m.Value, func(value string) bool {
		m.Value = value
		m.field.notify(m.Value)
		return true
	})
}
