//go:build !windows

package main

type powerKeeper struct{}

func startPowerKeeper() *powerKeeper { return &powerKeeper{} }
func (*powerKeeper) Close()          {}
