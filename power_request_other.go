//go:build !windows

package main

type powerKeeper struct{}

func startPowerKeeper(func(error)) *powerKeeper { return &powerKeeper{} }
func (*powerKeeper) Close()                     {}
