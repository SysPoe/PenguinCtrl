package ui

// OverlayVisible reports whether the operator panel is covering the cue list.
// Callers use this to suppress deferred UI, such as tooltips, that would
// otherwise be painted above the overlay at the end of the frame.
func (p *OperatorPanel) OverlayVisible() bool {
	return p.showBlocker || p.showLog || p.showPreflight
}
