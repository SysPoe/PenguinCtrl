package main

import (
	"gioui.org/unit"
	"github.com/syspoe/cusus/config"
)

const windowsBaselineDPI = 96

// operatorWindowSize converts a persisted physical size back to Gio's logical
// units. The native placement code scales it to the destination monitor again.
func operatorWindowSize(placement config.WindowPlacement) (unit.Dp, unit.Dp) {
	dpi := placement.DPI
	if dpi <= 0 {
		dpi = windowsBaselineDPI
	}
	return unit.Dp(float32(placement.Width*windowsBaselineDPI) / float32(dpi)),
		unit.Dp(float32(placement.Height*windowsBaselineDPI) / float32(dpi))
}
