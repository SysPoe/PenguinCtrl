package ui

import "testing"

func TestEmergencyStopRequiresConfirmationAndRequestIsOneShot(t *testing.T) {
	var topBar TopBar
	topBar.RequestEmergencyStop()
	if !topBar.EmergencyStopConfirmationOpen() {
		t.Fatal("emergency stop confirmation did not open")
	}
	if topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop was requested before confirmation")
	}
	if !topBar.ConfirmEmergencyStop() {
		t.Fatal("emergency stop confirmation was not accepted")
	}
	if !topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop request was not reported")
	}
	if topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop request was not consumed")
	}

	topBar.SetEmergencyResetting(true)
	topBar.RequestEmergencyStop()
	if topBar.EmergencyStopConfirmationOpen() {
		t.Fatal("emergency stop confirmation opened while reset was already running")
	}
	if topBar.TakeEmergencyStopRequest() {
		t.Fatal("emergency stop was accepted while reset was already running")
	}
}

func TestEmergencyStopConfirmationCanBeCancelled(t *testing.T) {
	var topBar TopBar
	topBar.RequestEmergencyStop()
	topBar.CancelEmergencyStop()

	if topBar.EmergencyStopConfirmationOpen() {
		t.Fatal("emergency stop confirmation remained open after cancellation")
	}
	if topBar.ConfirmEmergencyStop() || topBar.TakeEmergencyStopRequest() {
		t.Fatal("cancelled emergency stop was dispatched")
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
