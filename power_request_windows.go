//go:build windows

package main

import (
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	esContinuous          = 0x80000000
	esSystemRequired      = 0x00000001
	esDisplayRequired     = 0x00000002
	powerReassertInterval = 30 * time.Second
)

var procSetThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

type powerKeeper struct {
	done chan struct{}
	wg   sync.WaitGroup
}

func startPowerKeeper(report func(error)) *powerKeeper {
	keeper := &powerKeeper{done: make(chan struct{})}
	keeper.wg.Add(1)
	go func() {
		defer keeper.wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ticker := time.NewTicker(powerReassertInterval)
		defer ticker.Stop()
		for {
			if result, _, callErr := procSetThreadExecutionState.Call(esContinuous | esSystemRequired | esDisplayRequired); result == 0 && report != nil {
				report(callErr)
			}
			select {
			case <-keeper.done:
				if result, _, callErr := procSetThreadExecutionState.Call(esContinuous); result == 0 && report != nil {
					report(callErr)
				}
				return
			case <-ticker.C:
			}
		}
	}()
	return keeper
}

func (k *powerKeeper) Close() { close(k.done); k.wg.Wait() }
