//go:build !windows

package main

// Gio exposes the native pre-destroy close message on Windows, the supported
// show-control platform. Other platforms retain crash-journal recovery.
type windowCloseInterceptor struct{}

func (*windowCloseInterceptor) HandleEvent(any, func()) error { return nil }
func (*windowCloseInterceptor) AllowAndClose() error          { return nil }
func (*windowCloseInterceptor) ResetRequest()                 {}
