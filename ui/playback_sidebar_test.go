package ui

import "testing"

func TestGoButtonLabelShowsRestartForActiveCue(t *testing.T) {
	if got := goButtonLabel(false); got != "GO" {
		t.Fatalf("inactive label = %q", got)
	}
	if got := goButtonLabel(true); got != "RESTART" {
		t.Fatalf("active label = %q", got)
	}
}
