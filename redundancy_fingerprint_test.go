package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
)

func TestRedundancyFingerprintRequiresEveryReferencedMediaHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cue.wav")
	current := show.Show{Cues: []show.Cue{{
		ID: show.NewCueID(), Type: show.CueTypeSound,
		Play: show.CuePlay{Sound: &show.SoundPlay{File: path}},
	}}}
	settings := config.Defaults()
	missing := buildRedundancyFingerprint(current, settings, nil, true)
	if missing.Ready {
		t.Fatal("fingerprint was ready without a content hash for referenced media")
	}
	ready := buildRedundancyFingerprint(current, settings, []project.File{{Source: path, Hash: strings.Repeat("a", 64), Kind: "audio"}}, true)
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
	first := buildRedundancyFingerprint(
		show.Show{Cues: []show.Cue{{ID: cueID, Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{File: firstPath}}}}},
		settings, []project.File{{Source: firstPath, Hash: contentHash, Kind: "audio"}}, true,
	)
	secondShow := show.Show{Cues: []show.Cue{{ID: cueID, Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{File: secondPath}}}}}
	second := buildRedundancyFingerprint(secondShow, settings, []project.File{{Source: secondPath, Hash: contentHash, Kind: "audio"}}, true)
	if first.Media != second.Media {
		t.Fatalf("same media content produced different digests: %s != %s", first.Media, second.Media)
	}
	if first.Show != second.Show {
		t.Fatalf("machine-local extracted paths changed logical show digest: %s != %s", first.Show, second.Show)
	}
	secondShow.AcknowledgedProblems = map[string]bool{"operator-local-warning": true}
	acknowledged := buildRedundancyFingerprint(secondShow, settings, []project.File{{Source: secondPath, Hash: contentHash, Kind: "audio"}}, true)
	if second.Show != acknowledged.Show {
		t.Fatal("operator-local warning acknowledgement changed production show identity")
	}
}

func TestRedundancyRoutingFingerprintChangesWithOutputMapping(t *testing.T) {
	settings := config.Defaults()
	first := redundancyRoutingDigest(settings)
	settings.VideoOutputs[0].DisplayID = "projector-b"
	second := redundancyRoutingDigest(settings)
	if first == second {
		t.Fatal("display mapping change did not alter routing fingerprint")
	}
}
