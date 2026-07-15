package playback

import (
	"context"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

type lifecycleHostStub struct {
	links         []linkMoment
	timecodeCalls int
	finalizations []runFinalization
	signals       int
}

func (h *lifecycleHostStub) goOwned(work func()) bool {
	work()
	return true
}

func (h *lifecycleHostStub) scheduleLink(_ show.Cue, _ int, _ int64, moment linkMoment, _ context.Context) {
	h.links = append(h.links, moment)
}

func (h *lifecycleHostStub) scheduleTimecode(_ string, _ show.Cue, _ int) {
	h.timecodeCalls++
}

func (*lifecycleHostStub) replaceSingleLayerVisual(liveInstance) {}

func (h *lifecycleHostStub) finishCueRun(_ cueRunToken, finalization runFinalization) {
	h.finalizations = append(h.finalizations, finalization)
}

func (h *lifecycleHostStub) signalState() {
	h.signals++
}

func TestPrepareInstanceLifecycleClampsFadeToRemainingPlayback(t *testing.T) {
	instance := &liveInstance{
		Instance:            Instance{ID: "timed", BackendStarted: true, DurationMs: 1000, ClipStartMs: 100, PositionMs: 800, FadeOutMs: 500},
		lifecycleGeneration: 4,
	}

	schedule, ok := prepareInstanceLifecycle(instance, time.Unix(100, 0))
	if !ok {
		t.Fatal("active timed instance did not produce a lifecycle schedule")
	}
	if schedule.fadeAfter != 0 || schedule.fadeFor != 300 || schedule.endAfter != 300*time.Millisecond {
		t.Fatalf("schedule = %#v, want immediate 300ms fade and end after 300ms", schedule)
	}
	if !instance.endScheduled || instance.lifecycleGeneration != 5 || schedule.generation != 5 {
		t.Fatalf("lifecycle identity = instance %#v schedule %#v", instance, schedule)
	}
	if _, ok := prepareInstanceLifecycle(instance, time.Unix(100, 0)); ok {
		t.Fatal("already-scheduled instance produced a duplicate timer plan")
	}
}

func TestLifecycleControllerRoutesReportEffectsAndRetiresInstance(t *testing.T) {
	host := &lifecycleHostStub{}
	runtime := newRuntimeState(context.Background())
	registry := runtime.instances
	outputs := newOutputBus()
	controller := newLifecycleController(host, runtime, outputs)
	cue := show.NewSoundCue()
	registry.register(&liveInstance{
		Instance:   Instance{ID: "reported", CueID: cue.ID, OutputID: "main", FadeInMs: 0},
		cue:        cue,
		cueIndex:   7,
		postWaitMs: 20,
		link:       show.CueLink{Mode: show.CueLinkManual},
		run:        cueRunToken{cueID: cue.ID, id: 1, ctx: context.Background()},
	})
	outputEvents := outputs.subscribe("main")

	controller.handleOutputReport("reported", outputReportStarted)
	controller.handleOutputReport("reported", outputReportEnded)

	if registry.has("reported") {
		t.Fatal("ended report left instance registered")
	}
	if len(host.links) != 2 || host.links[0] != linkFadeIn || host.links[1] != linkEnd {
		t.Fatalf("lifecycle links = %#v, want fade-in then end", host.links)
	}
	if host.timecodeCalls != 1 || host.signals != 2 {
		t.Fatalf("timecode calls = %d, signals = %d", host.timecodeCalls, host.signals)
	}
	if len(host.finalizations) != 1 || host.finalizations[0] != runAborted {
		t.Fatalf("finalizations = %#v, want manual run aborted", host.finalizations)
	}
	select {
	case event := <-outputEvents:
		if event.Action != "remove" || len(event.InstanceIDs) != 1 || event.InstanceIDs[0] != "reported" {
			t.Fatalf("removal event = %#v", event)
		}
	default:
		t.Fatal("ended report did not publish removal")
	}
}
