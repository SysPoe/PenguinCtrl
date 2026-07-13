package ui

import (
	"image/color"
	"testing"

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

func TestInformationalPreflightIsReadyWithoutAttention(t *testing.T) {
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
