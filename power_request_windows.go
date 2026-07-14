//go:build windows

package main

import (
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

var procSetThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

type powerKeeper struct {
	done chan struct{}
	wg   sync.WaitGroup
}

func startPowerKeeper() *powerKeeper {
	keeper := &powerKeeper{done: make(chan struct{})}
	keeper.wg.Add(1)
	go func() {
		defer keeper.wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			// TODO(micro): Check SetThreadExecutionState results; a zero return means the show-safety power request was not applied.
			procSetThreadExecutionState.Call(esContinuous | esSystemRequired | esDisplayRequired)
			select {
			case <-keeper.done:
				// TODO(micro): Check or explicitly discard the error when restoring the default execution state.
				procSetThreadExecutionState.Call(esContinuous)
				return
			case <-ticker.C:
			}
		}
	}()
	return keeper
}

func (k *powerKeeper) Close() { close(k.done); k.wg.Wait() }
