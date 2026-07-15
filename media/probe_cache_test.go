package media

import (
	"errors"
	"testing"
)

func TestDeviceAccessorsReturnCachedWorkerResults(t *testing.T) {
	audioErr := errors.New("cached audio failure")
	manager := &Manager{
		topology: &deviceTopology{
			audioDevices: []AudioDevice{{ID: "speaker"}}, audioDevicesErr: audioErr,
			displays: []VideoDisplay{{ID: "stage"}}, displaysErr: errors.New("cached display failure"),
		},
	}
	devices, err := manager.AudioDevices()
	if len(devices) != 1 || !errors.Is(err, audioErr) {
		t.Fatalf("audio cache = %#v, %v", devices, err)
	}
	displays, err := manager.VideoDisplays()
	if len(displays) != 1 || err == nil {
		t.Fatalf("display cache = %#v, %v", displays, err)
	}
}
