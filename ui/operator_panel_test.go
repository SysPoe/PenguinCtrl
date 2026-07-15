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
	panel.ackButton.Click()

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 54)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	panel.LayoutBar(material.NewTheme(), gtx, operatorlog.NewStore(), nil)

	if panel.status != "" {
		t.Fatalf("acknowledged status = %q, want empty", panel.status)
	}
}

func TestOperatorEventListFillsViewportWidth(t *testing.T) {
	var panel OperatorPanel
	panel.list.Axis = layout.Vertical
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

	dimensions := panel.layoutEvents(material.NewTheme(), gtx, events)
	if dimensions.Size.X != gtx.Constraints.Max.X {
		t.Fatalf("event list width = %d, want viewport width %d", dimensions.Size.X, gtx.Constraints.Max.X)
	}
}

func TestPreflightPresentationDoesNotCallChecksFailures(t *testing.T) {
	checks := []operatorlog.PreflightCheck{
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
	checks := []operatorlog.PreflightCheck{
		{Severity: operatorlog.ShowStopping},
		{Severity: operatorlog.CueFailure},
		{Severity: operatorlog.Recoverable},
		{Severity: operatorlog.Warning},
	}

	filtered := filterPreflight(checks, 1)
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
	checks := []operatorlog.PreflightCheck{{Severity: operatorlog.Info}}
	if got := preflightCount(checks); got != "READY" {
		t.Fatalf("preflight count = %q, want READY", got)
	}
	if preflightRequiresAttention(checks) {
		t.Fatal("informational preflight requires attention")
	}

	checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.Warning})
	if got := preflightCount(checks); got != "1 ITEM" {
		t.Fatalf("preflight count = %q, want 1 ITEM", got)
	}
	if !preflightRequiresAttention(checks) {
		t.Fatal("warning preflight does not require attention")
	}
}
