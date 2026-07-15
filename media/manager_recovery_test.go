package media

import "testing"

func TestOutputRecoveryOnlyRunsForDesiredLiveOutputs(t *testing.T) {
	controller := &outputController{windows: map[string]*outputWindow{}, desired: map[string]struct{}{"stage": {}}}
	if !controller.shouldRecoverOutput("stage") {
		t.Fatal("desired output was not recoverable")
	}
	if controller.shouldRecoverOutput("removed") {
		t.Fatal("removed output was recoverable")
	}
	controller.closed = true
	if controller.shouldRecoverOutput("stage") {
		t.Fatal("output was recoverable during application shutdown")
	}
}
