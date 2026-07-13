package media

import "testing"

func TestOutputRecoveryOnlyRunsForDesiredLiveOutputs(t *testing.T) {
	manager := &Manager{windows: map[string]*outputWindow{}, desired: map[string]struct{}{"stage": {}}}
	if !manager.shouldRecoverOutput("stage") {
		t.Fatal("desired output was not recoverable")
	}
	if manager.shouldRecoverOutput("removed") {
		t.Fatal("removed output was recoverable")
	}
	manager.closed = true
	if manager.shouldRecoverOutput("stage") {
		t.Fatal("output was recoverable during application shutdown")
	}
}
