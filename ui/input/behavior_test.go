package input

import (
	"image/color"
	"math"
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

	want := nrgbaToOKLCH(next)
	if math.Abs(field.oklch.L-want.L) > 1e-9 || math.Abs(field.oklch.C-want.C) > 1e-9 || math.Abs(field.oklch.H-want.H) > 1e-9 {
		t.Fatalf("oklch = %#v, want %#v", field.oklch, want)
	}
	if field.alpha.Value != float64(next.A) {
		t.Fatalf("alpha = %f, want %d", field.alpha.Value, next.A)
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
