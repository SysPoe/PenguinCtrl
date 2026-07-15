package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/show"
)

func TestRedundancyFingerprintRejectsUnencodableShow(t *testing.T) {
	current := show.Show{Extensions: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)}}
	if _, err := redundancy.BuildFingerprint(current, config.Defaults(), nil, true); err == nil {
		t.Fatal("invalid show produced a redundancy fingerprint")
	}
}

func TestRedundancyFingerprintRequiresEveryReferencedMediaHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cue.wav")
	current := show.Show{Cues: []show.Cue{{
		ID: show.NewCueID(), Type: show.CueTypeSound,
		Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: path}}},
	}}}
	settings := config.Defaults()
	missing, err := redundancy.BuildFingerprint(current, settings, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Ready {
		t.Fatal("fingerprint was ready without a content hash for referenced media")
	}
	ready, err := redundancy.BuildFingerprint(current, settings, []project.File{{Source: path, Hash: strings.Repeat("a", 64), Kind: "audio"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ready.Ready || ready.Media == "" {
		t.Fatalf("validated fingerprint = %+v", ready)
	}
}

func TestRedundancyFingerprintUsesContentNotMachineLocalPath(t *testing.T) {
	firstPath := filepath.Join(t.TempDir(), "one.wav")
	secondPath := filepath.Join(t.TempDir(), "two.wav")
	settings := config.Defaults()
	cueID := show.NewCueID()
	contentHash := strings.Repeat("b", 64)
	first, err := redundancy.BuildFingerprint(
		show.Show{Cues: []show.Cue{{ID: cueID, Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: firstPath}}}}}},
		settings, []project.File{{Source: firstPath, Hash: contentHash, Kind: "audio"}}, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondShow := show.Show{Cues: []show.Cue{{ID: cueID, Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: secondPath}}}}}}
	second, err := redundancy.BuildFingerprint(secondShow, settings, []project.File{{Source: secondPath, Hash: contentHash, Kind: "audio"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Media != second.Media {
		t.Fatalf("same media content produced different digests: %s != %s", first.Media, second.Media)
	}
	if first.Show != second.Show {
		t.Fatalf("machine-local extracted paths changed logical show digest: %s != %s", first.Show, second.Show)
	}
	secondShow.AcknowledgedProblems = map[string]bool{"operator-local-warning": true}
	acknowledged, err := redundancy.BuildFingerprint(secondShow, settings, []project.File{{Source: secondPath, Hash: contentHash, Kind: "audio"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if second.Show != acknowledged.Show {
		t.Fatal("operator-local warning acknowledgement changed production show identity")
	}
}

func TestRedundancyRoutingFingerprintChangesWithOutputMapping(t *testing.T) {
	settings := config.Defaults()
	first, err := redundancy.BuildFingerprint(show.Show{}, settings, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	settings.VideoOutputs[0].DisplayID = "projector-b"
	second, err := redundancy.BuildFingerprint(show.Show{}, settings, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Routing == second.Routing {
		t.Fatal("display mapping change did not alter routing fingerprint")
	}
}
