package ui

import (
	"image"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestConstrainModalPanelClampsFixedSizeToViewport(t *testing.T) {
	gtx := modalTestContext(image.Pt(320, 150))
	got := constrainModalPanel(gtx, modalPanelStyle{width: dialogPanelWidth, height: unit.Dp(180)})
	if got.Constraints.Min != (image.Pt(320, 150)) || got.Constraints.Max != (image.Pt(320, 150)) {
		t.Fatalf("fixed constraints = %+v", got.Constraints)
	}
}

func TestConstrainModalPanelKeepsFlexibleWidthValid(t *testing.T) {
	tests := []struct {
		viewport int
		wantMin  int
		wantMax  int
	}{
		{viewport: 800, wantMin: 480, wantMax: 620},
		{viewport: 500, wantMin: 480, wantMax: 500},
		{viewport: 400, wantMin: 400, wantMax: 400},
	}
	for _, test := range tests {
		gtx := modalTestContext(image.Pt(test.viewport, 600))
		got := constrainModalPanel(gtx, modalPanelStyle{minWidth: unit.Dp(480), maxWidth: unit.Dp(620)})
		if got.Constraints.Min.X != test.wantMin || got.Constraints.Max.X != test.wantMax {
			t.Errorf("viewport %d constraints = %+v", test.viewport, got.Constraints)
		}
	}
}

func TestConfirmationActionPreservesDialogKeyboardContract(t *testing.T) {
	tests := map[key.Name]confirmationKeyAction{
		key.NameEscape: confirmationKeyCancel,
		key.NameReturn: confirmationKeyAccept,
		key.NameEnter:  confirmationKeyAccept,
		key.NameSpace:  confirmationKeyNone,
	}
	for name, want := range tests {
		if got := confirmationAction(name); got != want {
			t.Errorf("confirmationAction(%q) = %v, want %v", name, got, want)
		}
	}
}

func modalTestContext(max image.Point) layout.Context {
	return layout.Context{
		Constraints: layout.Constraints{Max: max},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops:         new(op.Ops),
	}
}
