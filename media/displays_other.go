//go:build !windows

package media

import "github.com/syspoe/cusus/config"

func enumerateVideoDisplays() ([]VideoDisplay, error) {
	// TODO(micro): stub primary display uses magic 1920x1080@60/DPI 96; name nonWindowsStubDisplay constants.
	return []VideoDisplay{{ID: "primary", Name: "Primary display", Primary: true, Width: 1920, Height: 1080, RefreshRate: 60, DPI: 96}}, nil
}
func platformViewHandle(any) uintptr                                       { return 0 }
func platformPlaceWindow(uintptr, config.VideoOutput, []VideoDisplay) bool { return true }
func platformWindowGeometry(uintptr, VideoDisplay) (int, int, int, int, bool) {
	return 0, 0, 0, 0, false
}
