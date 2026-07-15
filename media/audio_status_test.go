package media

import (
	"errors"
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
)

func TestAudioDeviceWarningAllowsIndependentWindowsDefaultRouting(t *testing.T) {
	devices := []AudioDevice{{ID: "speakers", Name: "Speakers"}}
	tests := []struct {
		name     string
		settings config.Settings
	}{
		{name: "both default", settings: config.Settings{}},
		{name: "playback default", settings: config.Settings{AudioSettings: config.AudioSettings{PreviewAudioDevice: "speakers"}}},
		{name: "preview default", settings: config.Settings{AudioSettings: config.AudioSettings{PlaybackAudioDevice: "speakers"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if warning := audioDeviceWarning(test.settings, devices, nil); warning != "" {
				t.Fatalf("warning = %q, want none", warning)
			}
		})
	}
}

func TestAudioDeviceWarningReportsDisconnectedRoutes(t *testing.T) {
	devices := []AudioDevice{{ID: "connected", Name: "Connected"}}
	tests := []struct {
		name     string
		settings config.Settings
		want     string
	}{
		{name: "playback", settings: config.Settings{AudioSettings: config.AudioSettings{PlaybackAudioDevice: "missing"}}, want: "playback audio device"},
		{name: "preview", settings: config.Settings{AudioSettings: config.AudioSettings{PreviewAudioDevice: "missing"}}, want: "preview audio device"},
		{name: "both same", settings: config.Settings{AudioSettings: config.AudioSettings{PlaybackAudioDevice: "missing", PreviewAudioDevice: "missing"}}, want: "playback and preview audio device"},
		{name: "both separate", settings: config.Settings{AudioSettings: config.AudioSettings{PlaybackAudioDevice: "missing-1", PreviewAudioDevice: "missing-2"}}, want: "playback and preview audio devices"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			warning := audioDeviceWarning(test.settings, devices, nil)
			if !strings.Contains(warning, test.want) {
				t.Fatalf("warning = %q, want it to contain %q", warning, test.want)
			}
		})
	}
}

func TestAudioDeviceWarningReportsEnumerationFailure(t *testing.T) {
	warning := audioDeviceWarning(config.Settings{}, nil, errors.New("device service stopped"))
	if !strings.Contains(warning, "device service stopped") {
		t.Fatalf("warning = %q", warning)
	}
}
