package input

import (
	"image"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestEditorFieldModelFansOutBindingThenObservers(t *testing.T) {
	model := newEditorFieldModel[int]("1", true, key.HintNumeric, "0123456789")
	var calls []string
	model.addEventListener(func(value int) { calls = append(calls, "observer") })
	model.setChangeListener(func(value int) { calls = append(calls, "first binding") })

	model.notify(2)
	model.setChangeListener(func(value int) { calls = append(calls, "second binding") })
	model.notify(3)

	want := []string{"first binding", "observer", "second binding", "observer"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("listener order = %v, want %v", calls, want)
	}
}

func TestEditorFieldModelSynchronizesAndConsumesFocusRequest(t *testing.T) {
	model := newEditorFieldModel[string]("initial", true, key.HintText, "")
	model.requestFocus()
	if !model.focus || model.editor.SelectedText() != "initial" {
		t.Fatalf("focus request = pending %t selection %q", model.focus, model.editor.SelectedText())
	}

	accepted := 0
	model.layout(material.NewTheme(), editorFieldTestContext(), "Value", "external", func(string) bool {
		accepted++
		return true
	})
	if model.focus {
		t.Fatal("focus request was not consumed during layout")
	}
	if model.text != "external" || model.editor.Text() != "external" {
		t.Fatalf("external sync = model %q editor %q", model.text, model.editor.Text())
	}
	if accepted != 0 {
		t.Fatalf("external sync invoked edit acceptor %d times", accepted)
	}
}

func TestTypedEditorsSynchronizeExternalValuesWithoutNotification(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "text", run: func(t *testing.T) {
			field := NewText("Text", "old")
			notifications := 0
			field.AddEventListener(func(string) { notifications++ })
			field.Value = "new"
			field.Layout(material.NewTheme(), editorFieldTestContext())
			if field.field.editor.Text() != "new" || notifications != 0 {
				t.Fatalf("editor = %q, notifications = %d", field.field.editor.Text(), notifications)
			}
		}},
		{name: "integer", run: func(t *testing.T) {
			field := NewInteger("Integer", 1)
			notifications := 0
			field.AddEventListener(func(int) { notifications++ })
			field.Value = 42
			field.Layout(material.NewTheme(), editorFieldTestContext())
			if field.field.editor.Text() != "42" || notifications != 0 {
				t.Fatalf("editor = %q, notifications = %d", field.field.editor.Text(), notifications)
			}
		}},
		{name: "float", run: func(t *testing.T) {
			field := NewFloat("Float", 1.5)
			notifications := 0
			field.AddEventListener(func(float64) { notifications++ })
			field.Value = -3.25
			field.Layout(material.NewTheme(), editorFieldTestContext())
			if field.field.editor.Text() != "-3.25" || notifications != 0 {
				t.Fatalf("editor = %q, notifications = %d", field.field.editor.Text(), notifications)
			}
		}},
		{name: "multiline", run: func(t *testing.T) {
			field := NewMultiline("Multiline", "old")
			notifications := 0
			field.AddEventListener(func(string) { notifications++ })
			field.Value = "new\nvalue"
			field.Layout(material.NewTheme(), editorFieldTestContext())
			if field.field.editor.Text() != "new\nvalue" || notifications != 0 {
				t.Fatalf("editor = %q, notifications = %d", field.field.editor.Text(), notifications)
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func editorFieldTestContext() layout.Context {
	return layout.Context{
		Constraints: layout.Exact(image.Pt(640, 240)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops:         new(op.Ops),
	}
}
