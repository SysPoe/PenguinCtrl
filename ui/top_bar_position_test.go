package ui

import (
	"image"
	"testing"
)

func TestTopBarMenusAlignWithTheirButtons(t *testing.T) {
	var tb TopBar
	tb.setMenuPositions(
		1078,
		40,
		200,
		image.Pt(128, 40),
		image.Pt(155, 40),
		image.Pt(90, 40),
		image.Pt(147, 40),
	)

	if want := image.Pt(558, 40); tb.actionPos != want {
		t.Fatalf("action menu position = %v, want %v", tb.actionPos, want)
	}
	if want := image.Pt(686, 40); tb.addCuePos != want {
		t.Fatalf("add cue menu position = %v, want %v", tb.addCuePos, want)
	}
	if want := image.Pt(841, 40); tb.filePos != want {
		t.Fatalf("file menu position = %v, want %v", tb.filePos, want)
	}
}

func TestTopBarMenusStayWithinWindow(t *testing.T) {
	var tb TopBar
	tb.setMenuPositions(
		300,
		40,
		200,
		image.Pt(100, 40),
		image.Pt(100, 40),
		image.Pt(100, 40),
		image.Pt(100, 40),
	)

	for name, pos := range map[string]image.Point{
		"action":  tb.actionPos,
		"add cue": tb.addCuePos,
		"file":    tb.filePos,
	} {
		if pos.X < 0 || pos.X > 100 {
			t.Errorf("%s menu x = %d, want within [0, 100]", name, pos.X)
		}
	}
}
