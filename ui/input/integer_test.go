package input

import "testing"

func TestOptionalIntegerCanBeCleared(t *testing.T) {
	field := NewOptionalInteger("Port", 8000)

	field.applyText("")

	if field.Value != 0 {
		t.Fatalf("Value = %d, want 0", field.Value)
	}
	if !field.empty {
		t.Fatal("optional integer should retain its empty display state")
	}
}

func TestOptionalIntegerAcceptsValueAfterBeingCleared(t *testing.T) {
	field := NewOptionalInteger("Port", 8000)
	field.applyText("")

	field.applyText("6553")

	if field.Value != 6553 {
		t.Fatalf("Value = %d, want 6553", field.Value)
	}
	if field.empty {
		t.Fatal("optional integer should no longer be empty")
	}
}

func TestOptionalIntegerDisplaysZeroValueAsBlank(t *testing.T) {
	field := NewOptionalInteger("Port", 0)

	if field.text != "" {
		t.Fatalf("text = %q, want blank", field.text)
	}
}
