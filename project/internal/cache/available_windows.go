//go:build windows

package cache

import "golang.org/x/sys/windows"

func AvailableBytes(path string) (uint64, error) {
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	// Only caller-available bytes are part of this package's contract; total
	// capacity and raw free-space diagnostics are intentionally not collected.
	err = windows.GetDiskFreeSpaceEx(root, &available, nil, nil)
	return available, err
}
