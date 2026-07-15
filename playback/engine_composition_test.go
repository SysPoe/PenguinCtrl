package playback

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestEngineComposesOwnedSchedulingRuntimeAndOutputState(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	if engine.scheduler == nil || engine.scheduler.engine != engine {
		t.Fatal("command coordinator was not composed with the engine facade")
	}
	if engine.runtime == nil || engine.outputs == nil || engine.hooks == nil || engine.analysis == nil {
		t.Fatalf("engine collaborators = scheduler %p runtime %p output %p hooks %p analysis %T",
			engine.scheduler, engine.runtime, engine.outputs, engine.hooks, engine.analysis)
	}
	if engine.scheduler.dispatch == nil || engine.scheduler.audit == nil || engine.scheduler.executions == nil {
		t.Fatal("command coordinator does not own dispatch, audit, and execution state")
	}
}

func TestEngineFacadesReadCollaboratorSnapshots(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: "media", OutputID: "main"}})
	engine.runtime.mu.Unlock()
	visual := Event{Kind: OutputEventOutput, OutputID: "main", Control: string(outputCommandBlackout)}
	engine.outputs.rememberVisual("main", visual)
	executionID := engine.scheduler.executions.start(command{cue: show.NewWaitCue()}, "action", 1000)
	defer engine.scheduler.executions.finish(executionID)

	if instances := engine.ActiveInstances(); len(instances) != 1 || instances[0].ID != "media" {
		t.Fatalf("ActiveInstances facade = %#v", instances)
	}
	if executions := engine.ActiveExecutions(); len(executions) != 1 || executions[0].ID != executionID {
		t.Fatalf("ActiveExecutions facade = %#v", executions)
	}
	events, _ := engine.OutputSnapshot("main")
	if len(events) != 2 || events[1].Control != string(outputCommandBlackout) {
		t.Fatalf("OutputSnapshot facade = %#v", events)
	}
}

func TestEnginePlayFacadeQueuesThroughCommandCoordinator(t *testing.T) {
	cue := show.NewWaitCue()
	showAccess := &memoryShowAccess{cues: []show.Cue{cue}, selected: 0}
	engine := NewEngineWithRemote(showAccess, staticSettingsAccess{}, &recordingRemotePort{dispatched: make(chan show.Cue, 1)})

	if err := engine.PlaySelected(); err != nil {
		t.Fatal(err)
	}
	next := <-engine.scheduler.queue
	if next.cue.ID != cue.ID || next.sequence != 1 || next.runOwner != commandOwnsRun {
		t.Fatalf("queued command = %#v", next)
	}
}
