package input

import (
	"strconv"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget/material"
)

type Float struct {
	Label string
	Hint  string
	Value float64

	field editorFieldModel[float64]
}

func NewFloat(label string, value float64) *Float {
	text := strconv.FormatFloat(value, 'f', -1, 64)
	return &Float{
		Label: label,
		Hint:  label,
		Value: value,
		field: newEditorFieldModel[float64](text, true, key.HintNumeric, "-0123456789."),
	}
}

func (f *Float) AddEventListener(listener func(value float64)) {
	f.field.addEventListener(listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (f *Float) SetChangeListener(listener func(value float64)) {
	f.field.setChangeListener(listener)
}

// Focus selects the field contents and requests keyboard focus.
func (f *Float) Focus() {
	f.field.requestFocus()
}

func (f *Float) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	expected := strconv.FormatFloat(f.Value, 'f', -1, 64)
	return f.field.layout(th, gtx, f.Hint, expected, f.applyText)
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
	f.field.text = text
	if value, err := strconv.ParseFloat(text, 64); err == nil && value != f.Value {
		f.Value = value
		f.field.notify(f.Value)
	}
	return true
}
