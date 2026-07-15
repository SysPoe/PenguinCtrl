package main

import (
	"fmt"

	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/ui"
)

func configureVideoRoutingSettings(page *ui.SettingsPage, backend media.Backend) {
	page.SetVideoDisplayProvider(func() ([]ui.VideoDisplay, error) {
		displays, err := backend.VideoDisplays()
		if err != nil {
			return nil, err
		}
		result := make([]ui.VideoDisplay, len(displays))
		for i, display := range displays {
			name := fmt.Sprintf("%s · %dx%d @ %d Hz · %d DPI", display.Name, display.Width, display.Height, display.RefreshRate, display.DPI)
			result[i] = ui.VideoDisplay{ID: display.ID, Name: name, Primary: display.Primary}
		}
		return result, nil
	})
}

func refreshVideoRouting(backend media.Backend) {
	backend.RefreshVideoOutputStatus()
}

func videoRoutingWarning(backend media.Backend) string {
	return backend.VideoOutputWarning()
}
