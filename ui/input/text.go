package input

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget/material"
)

type Text struct {
	Label string
	Hint  string
	Value string

	field editorFieldModel[string]
}

func NewText(label, value string) *Text {
	t := &Text{
		Label: label,
		Hint:  label,
		Value: value,
		field: newEditorFieldModel[string](value, true, key.HintText, ""),
	}
	return t
}

func (t *Text) AddEventListener(listener func(value string)) {
	t.field.addEventListener(listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (t *Text) SetChangeListener(listener func(value string)) {
	t.field.setChangeListener(listener)
}

// Focus selects the field contents and requests keyboard focus.
func (t *Text) Focus() {
	t.field.requestFocus()
}

func (t *Text) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return t.field.layout(th, gtx, t.Hint, t.Value, func(value string) bool {
		t.Value = value
		t.field.notify(t.Value)
		return true
	})
}
