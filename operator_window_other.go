//go:build !windows

package main

import "github.com/syspoe/cusus/config"

func operatorViewHandle(any) uintptr                               { return 0 }
func applyOperatorPlacement(uintptr, config.WindowPlacement) error { return nil }
func operatorWindowPlacement(uintptr) (config.WindowPlacement, bool) {
	return config.WindowPlacement{}, false
}
