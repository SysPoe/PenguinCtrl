//go:build windows

package main

import (
	"unsafe"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"golang.org/x/sys/windows"
)

var operatorUser32 = windows.NewLazySystemDLL("user32.dll")
var operatorSetWindowPos = operatorUser32.NewProc("SetWindowPos")
var operatorGetWindowRect = operatorUser32.NewProc("GetWindowRect")

type operatorRect struct{ Left, Top, Right, Bottom int32 }

func operatorViewHandle(event any) uintptr {
	if event, ok := event.(app.Win32ViewEvent); ok && event.Valid() {
		return event.HWND
	}
	return 0
}

func applyOperatorPlacement(handle uintptr, placement config.WindowPlacement) {
	if handle == 0 {
		return
	}
	const noActivate = 0x0010
	// TODO(micro): Check SetWindowPos's return value so failed operator-window restoration is not reported as success.
	operatorSetWindowPos.Call(handle, 0, uintptr(placement.X), uintptr(placement.Y), uintptr(placement.Width), uintptr(placement.Height), noActivate)
}

func operatorWindowPlacement(handle uintptr) (config.WindowPlacement, bool) {
	var rect operatorRect
	ok, _, _ := operatorGetWindowRect.Call(handle, uintptr(unsafe.Pointer(&rect)))
	if ok == 0 {
		return config.WindowPlacement{}, false
	}
	return config.WindowPlacement{X: int(rect.Left), Y: int(rect.Top), Width: int(rect.Right - rect.Left), Height: int(rect.Bottom - rect.Top)}, true
}
