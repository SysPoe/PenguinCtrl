package input

import (
	"image/color"
	"testing"
)

func TestCheckboxReconcilesUserAndExternalChanges(t *testing.T) {
	field := NewCheckbox("Enabled", false)
	changes := 0
	field.AddEventListener(func(bool) { changes++ })

	field.checkbox.Value = true
	field.synchronize()
	if !field.Checked || changes != 1 {
		t.Fatalf("user change = checked %t, notifications %d", field.Checked, changes)
	}

	field.Checked = false
	field.synchronize()
	if field.checkbox.Value || changes != 1 {
		t.Fatalf("external change = widget %t, notifications %d", field.checkbox.Value, changes)
	}
}

func TestColourPickerSynchronizesExternalValue(t *testing.T) {
	field := NewColourPicker("Color", color.NRGBA{R: 0xff, A: 0xff})
	next := color.NRGBA{G: 0x80, B: 0xff, A: 0x7f}
	field.Value = next

	field.syncFromValue()
	field.syncSliders()

	if field.model.NRGBA() != next {
		t.Fatalf("model color = %#v, want %#v", field.model.NRGBA(), next)
	}
	oklch := field.model.OKLCH()
	if field.lightness.Value != oklch.L*100 || field.chroma.Value != oklch.C || field.hue.Value != oklch.H {
		t.Fatalf("sliders = L %f C %f H %f, want %#v", field.lightness.Value, field.chroma.Value, field.hue.Value, oklch)
	}
	if field.alpha.Value != float64(next.A) {
		t.Fatalf("alpha = %f, want %d", field.alpha.Value, next.A)
	}
}

func TestColourPickerPublishesSliderModelChangesOnce(t *testing.T) {
	field := NewColourPicker("Color", color.NRGBA{R: 0xff, A: 0xff})
	var changes []color.NRGBA
	field.AddEventListener(func(value color.NRGBA) { changes = append(changes, value) })
	field.lightness.Value = 0
	field.chroma.Value = 0
	field.hue.Value = 0
	field.alpha.Value = 64

	field.updateFromSliders()
	field.updateFromSliders()

	want := color.NRGBA{A: 64}
	if field.Value != want || field.model.NRGBA() != want {
		t.Fatalf("slider color = picker %#v model %#v, want %#v", field.Value, field.model.NRGBA(), want)
	}
	if len(changes) != 1 || changes[0] != want {
		t.Fatalf("notifications = %#v, want one %#v", changes, want)
	}
}

func TestDropdownNotificationIgnoresInvalidSelection(t *testing.T) {
	field := NewDropdown(nil, -1)
	called := false
	field.AddEventListener(func(int, DropdownItem) { called = true })

	field.notifyEventListeners()

	if called {
		t.Fatal("listener called for an invalid selection")
	}
}

func TestFloatRejectsMalformedInputButAllowsEditingStates(t *testing.T) {
	for _, value := range []string{"", "-", ".", "-.", "0", "-12.5", ".25"} {
		if !validFloatInput(value) {
			t.Errorf("validFloatInput(%q) = false", value)
		}
	}
	for _, value := range []string{"--1", "1-", "1.2.3", "-.2-"} {
		if validFloatInput(value) {
			t.Errorf("validFloatInput(%q) = true", value)
		}
	}

	field := NewFloat("Level", 1)
	if field.applyText("1.2.3") {
		t.Fatal("malformed value was accepted")
	}
	if field.Value != 1 {
		t.Fatalf("Value = %f after malformed input", field.Value)
	}
	if !field.applyText("-3.5") || field.Value != -3.5 {
		t.Fatalf("valid value = %f", field.Value)
	}
}

func TestChangeListenerReplacesModelBindingWithoutReplacingObservers(t *testing.T) {
	field := NewInteger("Value", 1)
	firstBinding, secondBinding, observations := 0, 0, 0
	field.AddEventListener(func(int) { observations++ })
	field.SetChangeListener(func(int) { firstBinding++ })

	field.applyText("2")
	field.SetChangeListener(func(int) { secondBinding++ })
	field.applyText("3")

	if firstBinding != 1 || secondBinding != 1 {
		t.Fatalf("model bindings = first %d, second %d; want 1 each", firstBinding, secondBinding)
	}
	if observations != 2 {
		t.Fatalf("observer notifications = %d, want 2", observations)
	}
}
