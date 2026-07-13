package ui

import "testing"

func TestDocumentGuardRequiresExplicitDirtyDecision(t *testing.T) {
	var guard DocumentGuard
	if !guard.Request(DocumentActionOpen, false) {
		t.Fatal("clean document did not allow immediate open")
	}
	if guard.Request(DocumentActionNew, true) || !guard.Visible() {
		t.Fatal("dirty document did not pause new action")
	}
	if !guard.BeginSave() {
		t.Fatal("save did not begin")
	}
	if _, ok := guard.ResolveSave(false); ok || !guard.Visible() {
		t.Fatal("failed save discarded pending action")
	}
	if !guard.BeginSave() {
		t.Fatal("save could not be retried")
	}
	if action, ok := guard.ResolveSave(true); !ok || action != DocumentActionNew || guard.Visible() {
		t.Fatalf("successful save resolved %v, %v", action, ok)
	}
}

func TestDocumentGuardDiscardAndCancel(t *testing.T) {
	var guard DocumentGuard
	guard.Request(DocumentActionOpen, true)
	if action, ok := guard.Discard(); !ok || action != DocumentActionOpen {
		t.Fatalf("discard resolved %v, %v", action, ok)
	}
	guard.Request(DocumentActionNew, true)
	guard.Cancel()
	if guard.Visible() {
		t.Fatal("cancel left guard visible")
	}
}

func TestDocumentGuardSupportsClose(t *testing.T) {
	var guard DocumentGuard
	guard.Request(DocumentActionClose, true)
	if action, ok := guard.Discard(); !ok || action != DocumentActionClose {
		t.Fatalf("close discard resolved %v, %v", action, ok)
	}
}
