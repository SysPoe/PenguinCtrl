//go:build windows

package media

import (
	"errors"
	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"golang.org/x/sys/windows"
	"runtime/cgo"
	"syscall"
	"unsafe"
)

const (
	monitorInfoPrimary        = 1
	eddGetDeviceInterfaceName = 1
	swpNoActivate             = 0x0010
	enumCurrentSettings       = ^uint32(0)
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procEnumDisplaySettings = user32.NewProc("EnumDisplaySettingsW")
	shcore                  = windows.NewLazySystemDLL("shcore.dll")
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
	displayMonitorCallback  = syscall.NewCallback(enumDisplayMonitor)
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
type deviceMode struct {
	DeviceName                                                                                     [32]uint16
	SpecVersion, DriverVersion, Size, DriverExtra                                                  uint16
	Fields                                                                                         uint32
	PositionX, PositionY                                                                           int32
	DisplayOrientation, DisplayFixedOutput                                                         uint32
	Color, Duplex, YResolution, TTOption, Collate                                                  uint16
	FormName                                                                                       [32]uint16
	LogPixels                                                                                      uint16
	BitsPerPel, PelsWidth, PelsHeight, DisplayFlags, DisplayFrequency                              uint32
	ICMMethod, ICMIntent, MediaType, DitherType, Reserved1, Reserved2, PanningWidth, PanningHeight uint32
}

type displayEnumeration struct {
	result []VideoDisplay
}

func enumDisplayMonitor(monitor, _ uintptr, _ *winRect, data uintptr) uintptr {
	enumeration := cgo.Handle(data).Value().(*displayEnumeration)
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
	mode := deviceMode{Size: uint16(unsafe.Sizeof(deviceMode{}))}
	// TODO(micro): Check both Win32 call results before publishing refresh-rate/DPI values; the current defaults hide API failure.
	procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(adapterPtr)), uintptr(enumCurrentSettings), uintptr(unsafe.Pointer(&mode)))
	var dpiX, dpiY uint32 = 96, 96
	procGetDpiForMonitor.Call(monitor, 0, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	enumeration.result = append(enumeration.result, VideoDisplay{ID: id, Name: name, Primary: info.Flags&monitorInfoPrimary != 0, X: int(info.Monitor.Left), Y: int(info.Monitor.Top), Width: int(info.Monitor.Right - info.Monitor.Left), Height: int(info.Monitor.Bottom - info.Monitor.Top), RefreshRate: int(mode.DisplayFrequency), DPI: int(dpiX)})
	return 1
}

func enumerateVideoDisplays() ([]VideoDisplay, error) {
	enumeration := displayEnumeration{}
	handle := cgo.NewHandle(&enumeration)
	defer handle.Delete()
	ok, _, callErr := procEnumDisplayMonitors.Call(0, 0, displayMonitorCallback, uintptr(handle))
	if ok == 0 {
		return nil, callErr
	}
	if len(enumeration.result) == 0 {
		// TODO(micro): Start this error with lowercase text so callers can wrap it without producing mid-sentence capitalization.
		return nil, errors.New("Windows reported no connected displays")
	}
	return enumeration.result, nil
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
	insertAfter := ^uintptr(1) // HWND_NOTOPMOST (-2).
	if route.AlwaysOnTop {
		insertAfter = ^uintptr(0) // HWND_TOPMOST (-1).
	}
	// TODO(micro): Return SetWindowPos failure instead of reporting placement success unconditionally.
	procSetWindowPos.Call(hwnd, insertAfter, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate)
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
