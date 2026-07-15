package playback

import (
	"context"
	"testing"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

type operatorControlHostStub struct {
	outputs      []outputEvent
	visuals      map[string]Event
	reports      []string
	reset        bool
	signals      int
	outputIDs    []string
	instances    []Instance
	mediaControl show.MediaControlPlay
}

func (*operatorControlHostStub) operatorLogStore() *operatorlog.Store { return nil }
func (host *operatorControlHostStub) resetPlaybackRuns()              { host.reset = true }
func (host *operatorControlHostStub) ActiveInstances() []Instance {
	return append([]Instance(nil), host.instances...)
}
func (host *operatorControlHostStub) OutputIDs() []string {
	return append([]string(nil), host.outputIDs...)
}
func (host *operatorControlHostStub) publishOutput(event outputEvent) {
	host.outputs = append(host.outputs, event)
}
func (host *operatorControlHostStub) HandleOutputReport(id, report string) {
	host.reports = append(host.reports, id+":"+report)
}
func (host *operatorControlHostStub) executeMediaControl(cue show.Cue, _ context.Context) error {
	host.mediaControl = *cue.Play.MediaControl
	return nil
}
func (*operatorControlHostStub) currentRunContext() context.Context { return context.Background() }
func (host *operatorControlHostStub) matchingInstances(show.MediaTarget) []Instance {
	return append([]Instance(nil), host.instances...)
}
func (*operatorControlHostStub) recordError(string, error) {}
func (host *operatorControlHostStub) rememberOutputVisual(outputID string, event Event) {
	host.visuals[outputID] = event
}
func (host *operatorControlHostStub) signalState() { host.signals++ }

func TestOperatorControllerStopAllAddressesEveryOutputAndRetiresKnownInstances(t *testing.T) {
	host := &operatorControlHostStub{
		outputIDs: []string{"main", "stage"},
		instances: []Instance{{ID: "playing"}},
		visuals:   map[string]Event{},
	}
	newOperatorController(host).stopAll()

	if !host.reset || len(host.outputs) != 2 || len(host.reports) != 1 || host.reports[0] != "playing:stopped" {
		t.Fatalf("stop all effects = reset %v outputs %#v reports %#v", host.reset, host.outputs, host.reports)
	}
	for _, output := range host.outputs {
		if event := output.compatibilityEvent(); event.Control != string(mediaCommandStopAll) {
			t.Fatalf("stop all output = %#v", event)
		}
	}
}

func TestOperatorControllerBlackoutOwnsOutputMutation(t *testing.T) {
	host := &operatorControlHostStub{outputIDs: []string{"main", "stage"}, visuals: map[string]Event{}}
	newOperatorController(host).blackoutAll()

	if len(host.outputs) != 2 || len(host.visuals) != 2 || host.signals != 1 {
		t.Fatalf("blackout effects = outputs %#v visuals %#v signals %d", host.outputs, host.visuals, host.signals)
	}
	for outputID, event := range host.visuals {
		if event.OutputID != outputID || event.Control != string(outputCommandBlackout) {
			t.Fatalf("blackout visual %q = %#v", outputID, event)
		}
	}
}
