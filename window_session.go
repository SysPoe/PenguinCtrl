package main

import "gioui.org/app"

// windowSession owns one operator window's event loop and background-service
// lifetimes. Document and per-frame show-control policy are delegated to their
// focused controllers.
type windowSession struct {
	application *App
	window      *app.Window
}

func (a *App) run(window *app.Window) error {
	return (&windowSession{application: a, window: window}).run()
}
