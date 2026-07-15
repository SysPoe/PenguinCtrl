//go:build !windows

package media

const (
	nonWindowsDisplayWidth   = 1920
	nonWindowsDisplayHeight  = 1080
	nonWindowsDisplayRefresh = 60
)

func enumerateVideoDisplays() ([]VideoDisplay, error) {
	return []VideoDisplay{{ID: "primary", Name: "Primary display", Primary: true, Width: nonWindowsDisplayWidth, Height: nonWindowsDisplayHeight, RefreshRate: nonWindowsDisplayRefresh, DPI: defaultDisplayDPI}}, nil
}
