package ui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/show"
)

func TestOperatorStatusPresentation(t *testing.T) {
	tests := []struct {
		name                  string
		status, health        string
		wantColor             color.NRGBA
		wantTitle, wantDetail string
	}{
		{name: "clear", health: "NORMAL", wantColor: palette.Success, wantTitle: "CLEAR"},
		{name: "status", status: "Saved show", health: "NORMAL", wantColor: palette.Primary, wantTitle: "STATUS", wantDetail: "Saved show"},
		{name: "degraded health", status: "Audio route unavailable", health: "DEGRADED", wantColor: palette.Warning, wantTitle: "HEALTH DEGRADED", wantDetail: "Audio route unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotColor, gotTitle, gotDetail := operatorStatusPresentation(test.status, test.health)
			if gotColor != test.wantColor || gotTitle != test.wantTitle || gotDetail != test.wantDetail {
				t.Fatalf("presentation = %#v, %q, %q; want %#v, %q, %q", gotColor, gotTitle, gotDetail, test.wantColor, test.wantTitle, test.wantDetail)
			}
		})
	}
}

func TestOperatorStatusCanBeAcknowledged(t *testing.T) {
	var panel OperatorPanel
	panel.SetStatus("Loaded show.cusus · recovery journal on")
	panel.bar.ackButton.Click()

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 54)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	panel.LayoutBar(material.NewTheme(), gtx, operatorlog.NewStore(), nil)

	if panel.bar.status != "" {
		t.Fatalf("acknowledged status = %q, want empty", panel.bar.status)
	}
}

func TestOperatorEventListFillsViewportWidth(t *testing.T) {
	var panel OperatorPanel
	panel.eventLog.list.Axis = layout.Vertical
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Constraints{Max: image.Pt(640, 480)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	events := []operatorlog.Event{{
		Severity: operatorlog.Recoverable,
		Source:   "Media output",
		Message:  "player is closed",
	}}

	dimensions := panel.eventLog.layoutEvents(material.NewTheme(), gtx, events)
	if dimensions.Size.X != gtx.Constraints.Max.X {
		t.Fatalf("event list width = %d, want viewport width %d", dimensions.Size.X, gtx.Constraints.Max.X)
	}
}

func TestOperatorPanelSwitchesFocusedChildrenWithoutSharingScrollState(t *testing.T) {
	var panel OperatorPanel
	panel.eventLog.list.Position.First = 4
	panel.preflight.list.Position.First = 1
	store := operatorlog.NewStore()

	panel.bar.logButton.Click()
	panel.LayoutBar(material.NewTheme(), operatorTestContext(1280, 54), store, nil)
	if panel.view != operatorPanelEventLog {
		t.Fatalf("selected view = %v, want event log", panel.view)
	}
	panel.bar.preflightButton.Click()
	panel.LayoutBar(material.NewTheme(), operatorTestContext(1280, 54), store, nil)
	if panel.view != operatorPanelPreflight {
		t.Fatalf("selected view = %v, want preflight", panel.view)
	}
	if panel.eventLog.list.Position.First != 4 || panel.preflight.list.Position.First != 1 {
		t.Fatalf("mode switch changed child scroll positions: log=%d preflight=%d", panel.eventLog.list.Position.First, panel.preflight.list.Position.First)
	}
	panel.bar.preflightButton.Click()
	panel.LayoutBar(material.NewTheme(), operatorTestContext(1280, 54), store, nil)
	if panel.view != operatorPanelClosed {
		t.Fatalf("second preflight toggle selected %v, want closed", panel.view)
	}
}

func TestOperatorPreflightNavigatorOwnsFilterAndNavigationState(t *testing.T) {
	blockerID, warningID := show.NewCueID(), show.NewCueID()
	checks := []preflight.Check{
		{Severity: operatorlog.ShowStopping, CueID: blockerID, Field: "file"},
		{Severity: operatorlog.Warning, CueID: warningID, Field: "routing"},
	}
	var navigator operatorPreflightNavigator
	navigator.problemIndex = 9
	navigator.filterButton.Click()
	if navigator.update(operatorTestContext(640, 480), checks, nil) {
		t.Fatal("filter update closed preflight")
	}
	if navigator.filter != preflightFilterBlockers || navigator.problemIndex != 0 {
		t.Fatalf("filter state = (%v, %d), want blockers at index zero", navigator.filter, navigator.problemIndex)
	}

	var gotID show.CueID
	var gotField string
	navigator.nextButton.Click()
	navigator.update(operatorTestContext(640, 480), checks, func(id show.CueID, _ bool, field string) {
		gotID, gotField = id, field
	})
	if gotID != blockerID || gotField != "file" {
		t.Fatalf("navigation = (%v, %q), want blocker file", gotID, gotField)
	}
}

func TestOperatorBlockerPreservesSelectedChild(t *testing.T) {
	panel := OperatorPanel{view: operatorPanelEventLog, blocker: operatorBlocker{visible: true}}
	panel.DismissBlocker()
	if panel.blocker.visible || panel.view != operatorPanelEventLog {
		t.Fatalf("dismissed blocker state = visible %v, view %v", panel.blocker.visible, panel.view)
	}
}

func TestPreflightPresentationDoesNotCallChecksFailures(t *testing.T) {
	checks := []preflight.Check{
		{Severity: operatorlog.ShowStopping},
		{Severity: operatorlog.CueFailure},
		{Severity: operatorlog.Recoverable},
	}
	if got := preflightSummary(checks); got != "2 blockers · 1 issue · 0 warnings" {
		t.Fatalf("preflight summary = %q", got)
	}
	if got := preflightSeverityLabel(operatorlog.ShowStopping); got != "BLOCKER" {
		t.Fatalf("preflight blocker label = %q", got)
	}
	if got := preflightSeverityLabel(operatorlog.Info); got != "INFORMATION" {
		t.Fatalf("preflight info label = %q", got)
	}
}

func TestBlockerFilterIncludesCueFailures(t *testing.T) {
	checks := []preflight.Check{
		{Severity: operatorlog.ShowStopping},
		{Severity: operatorlog.CueFailure},
		{Severity: operatorlog.Recoverable},
		{Severity: operatorlog.Warning},
	}

	filtered := filterPreflight(checks, preflightFilterBlockers)
	if len(filtered) != 2 || filtered[0].Severity != operatorlog.ShowStopping || filtered[1].Severity != operatorlog.CueFailure {
		t.Fatalf("blocker filter = %#v", filtered)
	}
}

func TestInvalidWarningIconIsReportedAsUnavailable(t *testing.T) {
	if icon := loadIcon("test", []byte("not iconvg")); icon != nil {
		t.Fatal("invalid icon data produced an icon")
	}
}

func TestInformationalPreflightIsReadyWithoutAttention(t *testing.T) {
	// TODO(micro): Give checks capacity two because the test appends one known element below.
	checks := []preflight.Check{{Severity: operatorlog.Info}}
	if got := preflightCount(checks); got != "READY" {
		t.Fatalf("preflight count = %q, want READY", got)
	}
	if preflightRequiresAttention(checks) {
		t.Fatal("informational preflight requires attention")
	}

	checks = append(checks, preflight.Check{Severity: operatorlog.Warning})
	if got := preflightCount(checks); got != "1 ITEM" {
		t.Fatalf("preflight count = %q, want 1 ITEM", got)
	}
	if !preflightRequiresAttention(checks) {
		t.Fatal("warning preflight does not require attention")
	}
}

func operatorTestContext(width, height int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
}
