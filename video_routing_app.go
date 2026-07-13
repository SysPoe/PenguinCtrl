package main

import (
	"fmt"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/ui"
)

type videoRoutingBackend interface {
	VideoDisplays() ([]media.VideoDisplay, error)
	VideoOutputWarning() string
	RefreshVideoOutputStatus()
}

func configureVideoRoutingSettings(page *ui.SettingsPage, backend media.Backend) {
	routing, ok := backend.(videoRoutingBackend)
	if !ok {
		return
	}
	page.SetVideoDisplayProvider(func() ([]ui.VideoDisplay, error) {
		displays, err := routing.VideoDisplays()
		result := make([]ui.VideoDisplay, len(displays))
		for i, display := range displays {
			name := fmt.Sprintf("%s · %dx%d @ %d Hz · %d DPI", display.Name, display.Width, display.Height, display.RefreshRate, display.DPI)
			result[i] = ui.VideoDisplay{ID: display.ID, Name: name, Primary: display.Primary}
		}
		return result, err
	})
}

func refreshVideoRouting(backend media.Backend) {
	if routing, ok := backend.(videoRoutingBackend); ok {
		routing.RefreshVideoOutputStatus()
	}
}

func videoRoutingWarning(backend media.Backend) string {
	if routing, ok := backend.(videoRoutingBackend); ok {
		return routing.VideoOutputWarning()
	}
	return ""
}
