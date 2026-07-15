package playback

import (
	"context"
	"errors"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func TestMediaCatalogIgnoresStaleProbeAfterCueMediaChanges(t *testing.T) {
	catalog := newMediaCatalog(context.Background())
	catalog.setDurationProbe(func(string) (int64, error) { return 1200, nil })
	cue := show.NewSoundCue()
	cue.Play.Sound.File = "first.wav"

	first := catalog.planRefresh([]show.Cue{cue}, config.Defaults())
	if len(first.durationTasks) != 1 || !first.changed {
		t.Fatalf("first plan = %#v", first)
	}
	firstKey := first.durationTasks[0].key

	cue.Play.Sound.File = "second.wav"
	second := catalog.planRefresh([]show.Cue{cue}, config.Defaults())
	if len(second.durationTasks) != 1 || !second.changed {
		t.Fatalf("second plan = %#v", second)
	}
	secondKey := second.durationTasks[0].key
	if firstKey == secondKey {
		t.Fatal("media change did not change the duration cache key")
	}
	if catalog.runDurationProbe(first.durationTasks[0]) {
		t.Fatal("stale duration probe reported a catalog change")
	}
	if got := catalog.duration(cue.ID, firstKey); got != 0 {
		t.Fatalf("stale duration was cached as %dms", got)
	}
	if !catalog.runDurationProbe(second.durationTasks[0]) {
		t.Fatal("current duration probe did not report a catalog change")
	}
	if got := catalog.duration(cue.ID, secondKey); got != 1200 {
		t.Fatalf("current duration = %dms, want 1200ms", got)
	}
}

func TestMediaCatalogTracksValidationPendingErrorAndInvalidation(t *testing.T) {
	catalog := newMediaCatalog(context.Background())
	catalog.setValidator(func(string, show.CueType) error { return errors.New("decode failed") })
	cue := show.NewImageCue()
	cue.Play.Image.File = "first.png"

	first := catalog.planRefresh([]show.Cue{cue}, config.Defaults())
	if len(first.validationTasks) != 1 {
		t.Fatalf("validation tasks = %#v", first.validationTasks)
	}
	firstKey := first.validationTasks[0].key
	pending := catalog.warning(cue.ID, firstKey)
	if !pending.trackValidation || !pending.validationPending || pending.validationChecked || pending.probeError != "" {
		t.Fatalf("pending validation state = %#v", pending)
	}
	if !catalog.runValidation(first.validationTasks[0]) {
		t.Fatal("current validation result was ignored")
	}
	checked := catalog.warning(cue.ID, firstKey)
	if checked.validationPending || !checked.validationChecked || checked.probeError != "decode failed" {
		t.Fatalf("completed validation state = %#v", checked)
	}

	cue.Play.Image.File = "second.png"
	second := catalog.planRefresh([]show.Cue{cue}, config.Defaults())
	if len(second.validationTasks) != 1 {
		t.Fatalf("replacement validation tasks = %#v", second.validationTasks)
	}
	replacement := catalog.warning(cue.ID, second.validationTasks[0].key)
	if !replacement.validationPending || replacement.validationChecked || replacement.probeError != "" {
		t.Fatalf("replacement validation state retained stale result: %#v", replacement)
	}
}
