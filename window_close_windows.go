//go:build windows

package main

import (
	"errors"
	"sync/atomic"

	"gioui.org/app"
	"golang.org/x/sys/windows"
)

const (
	gwlpWndProc = ^uintptr(3) // -4, expressed without an architecture-sized overflow.
	wmClose     = 0x0010
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procCallWindowProcW  = user32.NewProc("CallWindowProcW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
)

// windowCloseInterceptor subclasses the Gio window procedure so WM_CLOSE can
// be deferred while the application asks what to do with unsaved changes.
type windowCloseInterceptor struct {
	hwnd      uintptr
	original  uintptr
	callback  uintptr
	request   func()
	requested atomic.Bool
	allow     atomic.Bool
}

func (g *windowCloseInterceptor) HandleEvent(event any, request func()) error {
	view, ok := event.(app.Win32ViewEvent)
	if !ok || view.HWND == 0 || g.hwnd != 0 {
		return nil
	}
	g.hwnd, g.request = view.HWND, request
	g.callback = windows.NewCallback(func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		if message == wmClose && !g.allow.Load() {
			if g.requested.CompareAndSwap(false, true) && g.request != nil {
				g.request()
			}
			return 0
		}
		result, _, _ := procCallWindowProcW.Call(g.original, hwnd, uintptr(message), wParam, lParam)
		return result
	})
	original, _, callErr := procSetWindowLongPtr.Call(g.hwnd, gwlpWndProc, g.callback)
	if original == 0 {
		g.hwnd = 0
		return errors.New("could not install the Windows close interceptor: " + callErr.Error())
	}
	g.original = original
	return nil
}

func (g *windowCloseInterceptor) AllowAndClose() {
	if g.hwnd == 0 {
		return
	}
	g.allow.Store(true)
	procPostMessageW.Call(g.hwnd, wmClose, 0, 0)
}

func (g *windowCloseInterceptor) ResetRequest() {
	g.requested.Store(false)
}
