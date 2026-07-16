//go:build windows

package main

import (
	// "errors"
	// "fmt"
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

func applyOperatorPlacement(handle uintptr, placement config.WindowPlacement) error {
	// This function has been removed to fix issues where the operator window doesn't want to appear.
	// Most likely something to do with it being on another desktop / monitor??
	return nil // TODO do a proper fix
	// if handle == 0 {
	// 	return errors.New("operator window handle is unavailable")
	// }
	// if placement.Width <= 0 || placement.Height <= 0 {
	// 	return fmt.Errorf("invalid operator window size %dx%d", placement.Width, placement.Height)
	// }
	// const noActivate = 0x0010
	// ok, _, callErr := operatorSetWindowPos.Call(handle, 0, uintptr(placement.X), uintptr(placement.Y), uintptr(placement.Width), uintptr(placement.Height), noActivate)
	// if ok == 0 {
	// 	return fmt.Errorf("restore operator window placement: %w", callErr)
	// }
	// return nil
}

func operatorWindowPlacement(handle uintptr) (config.WindowPlacement, bool) {
	var rect operatorRect
	ok, _, _ := operatorGetWindowRect.Call(handle, uintptr(unsafe.Pointer(&rect)))
	if ok == 0 {
		return config.WindowPlacement{}, false
	}
	return config.WindowPlacement{X: int(rect.Left), Y: int(rect.Top), Width: int(rect.Right - rect.Left), Height: int(rect.Bottom - rect.Top)}, true
}
