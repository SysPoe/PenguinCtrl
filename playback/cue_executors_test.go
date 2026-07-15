package playback

import (
	"fmt"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueExecutorSetBindsEachSupportedCueType(t *testing.T) {
	executors := newCueExecutorSet(&Engine{})
	tests := []struct {
		cueType       show.CueType
		executorType  string
		advanceBefore bool
	}{
		{show.CueTypeSound, "playback.mediaCueExecutor", false},
		{show.CueTypeVideo, "playback.mediaCueExecutor", false},
		{show.CueTypeImage, "playback.mediaCueExecutor", false},
		{show.CueTypeRemote, "playback.remoteCueExecutor", false},
		{show.CueTypeWait, "playback.waitCueExecutor", true},
		{show.CueTypeMediaControl, "playback.mediaControlCueExecutor", false},
		{show.CueTypeOutputControl, "playback.outputControlCueExecutor", false},
	}
	for _, test := range tests {
		binding, ok := executors.forType(test.cueType)
		if !ok {
			t.Errorf("cue type %d has no executor", test.cueType)
			continue
		}
		if got := fmt.Sprintf("%T", binding.executor); got != test.executorType || binding.advanceBeforeExecution != test.advanceBefore {
			t.Errorf("cue type %d binding = %s advance-before=%v", test.cueType, got, binding.advanceBeforeExecution)
		}
	}
	if _, ok := executors.forType(show.CueType(-1)); ok {
		t.Fatal("unsupported cue type unexpectedly had an executor")
	}
}
