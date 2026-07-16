//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"golang.org/x/sys/windows"
)

var operatorUser32 = windows.NewLazySystemDLL("user32.dll")
var operatorSetWindowPos = operatorUser32.NewProc("SetWindowPos")
var operatorGetWindowRect = operatorUser32.NewProc("GetWindowRect")
var operatorMonitorFromRect = operatorUser32.NewProc("MonitorFromRect")
var operatorGetMonitorInfo = operatorUser32.NewProc("GetMonitorInfoW")

type operatorRect struct{ Left, Top, Right, Bottom int32 }

type operatorMonitorInfo struct {
	Size          uint32
	Monitor, Work operatorRect
	Flags         uint32
}

func operatorViewHandle(event any) uintptr {
	if event, ok := event.(app.Win32ViewEvent); ok && event.Valid() {
		return event.HWND
	}
	return 0
}

func applyOperatorPlacement(handle uintptr, placement config.WindowPlacement) error {
	if handle == 0 {
		return errors.New("operator window handle is unavailable")
	}
	if placement.Width <= 0 || placement.Height <= 0 {
		return fmt.Errorf("invalid operator window size %dx%d", placement.Width, placement.Height)
	}

	rect := operatorRect{
		Left:   int32(placement.X),
		Top:    int32(placement.Y),
		Right:  int32(placement.X + placement.Width),
		Bottom: int32(placement.Y + placement.Height),
	}
	const monitorDefaultToNearest = 0x00000002
	monitor, _, _ := operatorMonitorFromRect.Call(uintptr(unsafe.Pointer(&rect)), monitorDefaultToNearest)
	if monitor == 0 {
		return errors.New("restore operator window placement: no display is available")
	}
	info := operatorMonitorInfo{Size: uint32(unsafe.Sizeof(operatorMonitorInfo{}))}
	if ok, _, callErr := operatorGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return fmt.Errorf("restore operator window placement: get display work area: %w", callErr)
	}
	placement = fitOperatorPlacement(placement, info.Work)

	const (
		noZOrder   = 0x0004
		noActivate = 0x0010
	)
	ok, _, callErr := operatorSetWindowPos.Call(handle, 0, uintptr(placement.X), uintptr(placement.Y), uintptr(placement.Width), uintptr(placement.Height), noZOrder|noActivate)
	if ok == 0 {
		return fmt.Errorf("restore operator window placement: %w", callErr)
	}
	return nil
}

func fitOperatorPlacement(placement config.WindowPlacement, work operatorRect) config.WindowPlacement {
	workWidth := int(work.Right - work.Left)
	workHeight := int(work.Bottom - work.Top)
	placement.Width = min(placement.Width, workWidth)
	placement.Height = min(placement.Height, workHeight)
	placement.X = min(max(placement.X, int(work.Left)), int(work.Right)-placement.Width)
	placement.Y = min(max(placement.Y, int(work.Top)), int(work.Bottom)-placement.Height)
	return placement
}

func operatorWindowPlacement(handle uintptr) (config.WindowPlacement, bool) {
	var rect operatorRect
	ok, _, _ := operatorGetWindowRect.Call(handle, uintptr(unsafe.Pointer(&rect)))
	if ok == 0 {
		return config.WindowPlacement{}, false
	}
	return config.WindowPlacement{X: int(rect.Left), Y: int(rect.Top), Width: int(rect.Right - rect.Left), Height: int(rect.Bottom - rect.Top)}, true
}
