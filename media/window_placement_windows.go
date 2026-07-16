//go:build windows

package media

import (
	"unsafe"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
)

const swpNoActivate = 0x0010

var (
	procSetWindowPos  = user32.NewProc("SetWindowPos")
	procGetWindowRect = user32.NewProc("GetWindowRect")
)

func platformViewHandle(event any) uintptr {
	if e, ok := event.(app.Win32ViewEvent); ok && e.Valid() {
		return e.HWND
	}
	return 0
}

func platformPlaceWindow(hwnd uintptr, route config.VideoOutput, displays []VideoDisplay) bool {
	// This function has been removed to fix issues where windows do not appear.
	return true // TODO proper fix
	// if hwnd == 0 || len(displays) == 0 {
	// 	return false
	// }
	// display, found := resolveDisplayForGeometry(route.DisplayID, displays)
	// x, y, width, height := display.X+route.X, display.Y+route.Y, route.Width, route.Height
	// if route.Fullscreen {
	// 	x, y, width, height = display.X, display.Y, display.Width, display.Height
	// }
	// insertAfter := ^uintptr(1) // HWND_NOTOPMOST (-2).
	// if route.AlwaysOnTop {
	// 	insertAfter = ^uintptr(0) // HWND_TOPMOST (-1).
	// }
	// ok, _, _ := procSetWindowPos.Call(hwnd, insertAfter, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoActivate)
	// return found && ok != 0
}

func platformWindowGeometry(hwnd uintptr, display VideoDisplay) (int, int, int, int, bool) {
	var rect winRect
	if hwnd == 0 {
		return 0, 0, 0, 0, false
	}
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ok == 0 {
		return 0, 0, 0, 0, false
	}
	return int(rect.Left) - display.X, int(rect.Top) - display.Y, int(rect.Right - rect.Left), int(rect.Bottom - rect.Top), true
}
