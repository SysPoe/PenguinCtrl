//go:build windows

package project

import "golang.org/x/sys/windows"

func cacheAvailableBytes(path string) (uint64, error) {
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	err = windows.GetDiskFreeSpaceEx(root, &available, nil, nil)
	return available, err
}
