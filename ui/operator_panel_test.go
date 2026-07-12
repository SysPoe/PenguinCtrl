package ui

import (
	"testing"

	"github.com/syspoe/cusus/operatorlog"
)

func TestActivePreflightSeverityUsesHighestUnacknowledgedCheck(t *testing.T) {
	checks := []operatorlog.PreflightCheck{
		{Severity: operatorlog.Warning},
		{Severity: operatorlog.Recoverable, Acknowledged: true},
		{Severity: operatorlog.ShowStopping},
	}

	severity, active := activePreflightSeverity(checks)

	if !active || severity != operatorlog.ShowStopping {
		t.Fatalf("active, severity = %v, %v; want true, show-stopping", active, severity)
	}
}

func TestActivePreflightSeverityIgnoresAcknowledgedChecks(t *testing.T) {
	checks := []operatorlog.PreflightCheck{{Severity: operatorlog.Warning, Acknowledged: true}}

	_, active := activePreflightSeverity(checks)

	if active {
		t.Fatal("acknowledged preflight check was treated as active")
	}
}
