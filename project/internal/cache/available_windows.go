//go:build windows

package cache

import "golang.org/x/sys/windows"

func AvailableBytes(path string) (uint64, error) {
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	// TODO(micro): total/free-for-caller out-params are nil; if diagnostics need total capacity later, capture them once here
	err = windows.GetDiskFreeSpaceEx(root, &available, nil, nil)
	return available, err
}
