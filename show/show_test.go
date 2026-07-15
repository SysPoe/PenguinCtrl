package show

import (
	"encoding/json"
	"testing"
)

func TestShowDigestRejectsInvalidExtensionJSON(t *testing.T) {
	current := Show{Extensions: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)}}
	if _, err := current.Digest(); err == nil {
		t.Fatal("invalid extension JSON produced a show digest")
	}
}

func TestShowDigestIsStable(t *testing.T) {
	current := Show{Title: "Matinee", Cues: []Cue{NewWaitCue()}}
	first, err := current.Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := current.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest changed: %x != %x", first, second)
	}
}
