package ui

import "testing"

func TestEmergencyStopRequestIsOneShotAndSuppressedDuringReset(t *testing.T) {
	var topBar TopBar
	topBar.RequestEmergencyStop()
	if !topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop request was not reported")
	}
	if topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop request was not consumed")
	}

	topBar.SetEmergencyResetting(true)
	topBar.RequestEmergencyStop()
	if topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop was accepted while reset was already running")
	}
}

func TestTopBarStatusRoutesToOperatorStatusSink(t *testing.T) {
	var topBar TopBar
	var got string
	topBar.SetStatusSink(func(status string) { got = status })

	topBar.SetStatus("Saving show")

	if got != "Saving show" {
		t.Fatalf("routed status = %q", got)
	}
}
