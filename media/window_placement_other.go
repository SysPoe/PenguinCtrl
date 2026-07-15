//go:build !windows

package media

import "github.com/syspoe/cusus/config"

func platformViewHandle(any) uintptr                                       { return 0 }
func platformPlaceWindow(uintptr, config.VideoOutput, []VideoDisplay) bool { return true }
func platformWindowGeometry(uintptr, VideoDisplay) (int, int, int, int, bool) {
	return 0, 0, 0, 0, false
}
