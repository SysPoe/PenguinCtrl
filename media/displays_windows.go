//go:build windows

package media

import (
	"errors"
	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

const (
	monitorInfoPrimary        = 1
	eddGetDeviceInterfaceName = 1
	swpNoActivate             = 0x0010
	swpNoZOrder               = 0x0004
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type monitorInfoEx struct {
	Size          uint32
	Monitor, Work winRect
	Flags         uint32
	DeviceName    [32]uint16
}
type displayDevice struct {
	Size         uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

func enumerateVideoDisplays() ([]VideoDisplay, error) {
	var result []VideoDisplay
	callback := syscall.NewCallback(func(monitor, _ uintptr, _ *winRect, _ uintptr) uintptr {
		info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
		if ok == 0 {
			return 1
		}
		adapter := windows.UTF16ToString(info.DeviceName[:])
		device := displayDevice{Size: uint32(unsafe.Sizeof(displayDevice{}))}
		adapterPtr, _ := windows.UTF16PtrFromString(adapter)
		ok, _, _ = procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(adapterPtr)), 0, uintptr(unsafe.Pointer(&device)), eddGetDeviceInterfaceName)
		id, name := adapter, adapter
		if ok != 0 {
			if v := windows.UTF16ToString(device.DeviceID[:]); v != "" {
				id = v
			}
			if v := windows.UTF16ToString(device.DeviceString[:]); v != "" {
				name = v + " (" + adapter + ")"
			}
		}
		result = append(result, VideoDisplay{ID: id, Name: name, Primary: info.Flags&monitorInfoPrimary != 0, X: int(info.Monitor.Left), Y: int(info.Monitor.Top), Width: int(info.Monitor.Right - info.Monitor.Left), Height: int(info.Monitor.Bottom - info.Monitor.Top)})
		return 1
	})
	ok, _, callErr := procEnumDisplayMonitors.Call(0, 0, callback, 0)
	if ok == 0 {
		return nil, callErr
	}
	if len(result) == 0 {
		return nil, errors.New("Windows reported no connected displays")
	}
	return result, nil
}
func platformViewHandle(event any) uintptr {
	if e, ok := event.(app.Win32ViewEvent); ok && e.Valid() {
		return e.HWND
	}
	return 0
}
func platformPlaceWindow(hwnd uintptr, route config.VideoOutput, displays []VideoDisplay) bool {
	if hwnd == 0 || len(displays) == 0 {
		return false
	}
	d, found := resolveDisplayForGeometry(route.DisplayID, displays)
	x, y, w, h := d.X+route.X, d.Y+route.Y, route.Width, route.Height
	if route.Fullscreen {
		x, y, w, h = d.X, d.Y, d.Width, d.Height
	}
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate|swpNoZOrder)
	return found
}
func platformWindowGeometry(hwnd uintptr, d VideoDisplay) (int, int, int, int, bool) {
	var r winRect
	if hwnd == 0 {
		return 0, 0, 0, 0, false
	}
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return 0, 0, 0, 0, false
	}
	return int(r.Left) - d.X, int(r.Top) - d.Y, int(r.Right - r.Left), int(r.Bottom - r.Top), true
}
