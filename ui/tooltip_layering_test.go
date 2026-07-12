package ui

import (
	"testing"
	"time"

	"gioui.org/x/component"
)

func TestOperatorPanelOverlayVisible(t *testing.T) {
	tests := []struct {
		name  string
		panel OperatorPanel
		want  bool
	}{
		{name: "closed"},
		{name: "blocker", panel: OperatorPanel{showBlocker: true}, want: true},
		{name: "event log", panel: OperatorPanel{showLog: true}, want: true},
		{name: "preflight", panel: OperatorPanel{showPreflight: true}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.panel.OverlayVisible(); got != test.want {
				t.Fatalf("OverlayVisible() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHideWarningTooltipClearsDeferredState(t *testing.T) {
	var area component.TipArea
	area.VisibilityAnimation.State = component.Visible
	deadline := time.Now().Add(time.Second)
	area.Hover.SetTarget(deadline)
	area.Press.SetTarget(deadline)
	area.LongPress.SetTarget(deadline)

	hideWarningTooltip(&area)

	if area.VisibilityAnimation.State != component.Invisible {
		t.Fatalf("tooltip state = %v, want invisible", area.VisibilityAnimation.State)
	}
	if area.Hover.Active || area.Press.Active || area.LongPress.Active {
		t.Fatal("tooltip invalidation deadline remained active")
	}
}
