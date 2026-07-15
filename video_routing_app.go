package main

import (
	"fmt"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/ui"
)

// TODO(macro): Optional videoRoutingBackend type-asserts a partial media.Backend surface for
// settings wiring. Fold display enumeration/warning/refresh into media.Backend (or a named
// media.VideoRouting port) so composition root adapters stop inventing private capability
// interfaces that only package main can satisfy-check.
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
		// TODO(micro): On err, still builds result from (likely nil) displays and returns both; prefer early `return nil, err`.
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
