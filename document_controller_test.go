package main

import (
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui"
)

func TestDocumentControllerOwnsNewAndLoadedDocumentTransitions(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager := show.NewShowManager()
	manager.AddCue(show.NewSoundCue())
	library := project.NewLibrary()
	library.Replace([]project.File{{Name: "old.wav", Source: "old.wav", Kind: "audio"}})
	session := newDocumentSession("old.cusus", manager.ShowSnapshot(), false)
	controller := newDocumentController(documentControllerConfig{
		manager: manager, playback: playback.NewEngine(manager, settings), library: library,
		session: session, events: operatorlog.NewStore(), panel: &ui.OperatorPanel{},
	})

	controller.New()
	if got := manager.Snapshot(); len(got) != 0 {
		t.Fatalf("new show cues = %d; want 0", len(got))
	}
	if files := library.Files(""); len(files) != 0 {
		t.Fatalf("new show library = %#v; want empty", files)
	}
	if path := session.pathSnapshot(); path != "" {
		t.Fatalf("new show path = %q; want empty", path)
	}

	loaded := show.Show{Title: "Loaded", Cues: []show.Cue{show.NewWaitCue()}}
	files := []project.File{{Name: "loaded.wav", Source: "loaded.wav", Kind: "audio"}}
	controller.replaceLoaded("loaded.cusus", loaded, files)
	if got := manager.ShowSnapshot(); got.Title != "Loaded" || len(got.Cues) != 1 {
		t.Fatalf("loaded show = %#v", got)
	}
	if got := library.Files(""); len(got) != 1 || got[0].Name != "loaded.wav" {
		t.Fatalf("loaded library = %#v", got)
	}
	if path := session.pathSnapshot(); path != "loaded.cusus" {
		t.Fatalf("loaded path = %q", path)
	}
}
