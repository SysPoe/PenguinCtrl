package ui

// OverlayVisible reports whether the operator panel is covering the cue list.
// Callers use this to suppress deferred UI, such as tooltips, that would
// otherwise be painted above the overlay at the end of the frame.
// TODO(micro): one-liner could live next to LayoutOverlay on OperatorPanel; tiny file only for this predicate is fine but consider colocating
func (p *OperatorPanel) OverlayVisible() bool {
	return p.showBlocker || p.showLog || p.showPreflight
}
