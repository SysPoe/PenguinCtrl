package input

import (
	"strconv"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget/material"
)

// Integer is a single-line integer input.
type Integer struct {
	Label string
	Hint  string
	Value int

	field    editorFieldModel[int]
	optional bool
	empty    bool
}

// Focus selects the field contents and requests keyboard focus.
func (i *Integer) Focus() {
	i.field.requestFocus()
}

// NewInteger creates a single-line integer input.
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
		field:    newEditorFieldModel[int](text, true, key.HintNumeric, "-0123456789"),
		optional: optional,
		empty:    optional && value == 0,
	}
	return i
}

func (i *Integer) AddEventListener(listener func(value int)) {
	i.field.addEventListener(listener)
}

// SetChangeListener replaces the field's single model-binding callback.
func (i *Integer) SetChangeListener(listener func(value int)) {
	i.field.setChangeListener(listener)
}

func (i *Integer) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	expected := strconv.Itoa(i.Value)
	if i.optional && i.empty {
		expected = ""
	}
	return i.field.layout(th, gtx, i.Hint, expected, func(text string) bool {
		i.applyText(text)
		return true
	})
}

func (i *Integer) applyText(text string) {
	i.field.text = text
	if i.optional && text == "" {
		changed := !i.empty || i.Value != 0
		i.empty = true
		i.Value = 0
		if changed {
			i.field.notify(i.Value)
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
		i.field.notify(i.Value)
	}
}
