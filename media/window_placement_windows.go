//go:build windows

package media

import (
	"unsafe"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
)

const (
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
)

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
	if hwnd == 0 {
		return false
	}
	placement, found := mediaWindowPlacement(route, displays)
	ok, _, _ := procSetWindowPos.Call(
		hwnd,
		placement.insertAfter,
		uintptr(placement.x),
		uintptr(placement.y),
		uintptr(placement.width),
		uintptr(placement.height),
		placement.flags,
	)
	return found && ok != 0
}

type nativeWindowPlacement struct {
	x, y, width, height int
	insertAfter         uintptr
	flags               uintptr
}

func mediaWindowPlacement(route config.VideoOutput, displays []VideoDisplay) (nativeWindowPlacement, bool) {
	placement := nativeWindowPlacement{
		insertAfter: ^uintptr(1), // HWND_NOTOPMOST (-2).
		flags:       swpNoActivate | swpShowWindow,
	}
	if route.AlwaysOnTop {
		placement.insertAfter = ^uintptr(0) // HWND_TOPMOST (-1).
	}
	if len(displays) == 0 {
		// Display enumeration can briefly lag window creation. Make the window
		// visible at Gio's current geometry and let the topology refresh reroute it.
		placement.flags |= swpNoMove | swpNoSize
		return placement, false
	}

	display, found := resolveDisplayForGeometry(route.DisplayID, displays)
	if route.Fullscreen {
		placement.x, placement.y = display.X, display.Y
		placement.width, placement.height = display.Width, display.Height
		return placement, found
	}

	placement.width = min(route.Width, display.Width)
	placement.height = min(route.Height, display.Height)
	placement.x = min(max(display.X+route.X, display.X), display.X+display.Width-placement.width)
	placement.y = min(max(display.Y+route.Y, display.Y), display.Y+display.Height-placement.height)
	return placement, found
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
