package media

import (
	"testing"

	"github.com/syspoe/cusus/playback"
)

type fakeMediaOutput struct{}

var _ playback.MediaOutput = (*fakeMediaOutput)(nil)

func (*fakeMediaOutput) Subscribe(string) (<-chan playback.Event, func()) {
	return make(chan playback.Event), func() {}
}

func (*fakeMediaOutput) OutputSnapshot(string) ([]playback.Event, uint64) { return nil, 0 }

func (*fakeMediaOutput) HandleOutputReport(string, string) {}

func (*fakeMediaOutput) HandleOutputDuration(string, int64) {}

func (*fakeMediaOutput) HandleOutputError(string, error) {}

func TestOutputControllerDependsOnMediaOutputPort(t *testing.T) {
	var port playback.MediaOutput = &fakeMediaOutput{}
	controller := newOutputController(port, nil, nil, nil)
	if controller.port != port {
		t.Fatal("output controller did not retain the media output port")
	}
}
