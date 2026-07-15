package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

func publishExtractedShow(temporary, root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err := os.Rename(temporary, root); err != nil {
			return fmt.Errorf("publish extracted show: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect extracted show cache: %w", err)
	}
	backup := root + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove old extraction backup: %w", err)
	}
	if err := os.Rename(root, backup); err != nil {
		return fmt.Errorf("preserve extracted show cache: %w", err)
	}
	if err := os.Rename(temporary, root); err != nil {
		_ = os.Rename(backup, root)
		return fmt.Errorf("publish extracted show: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

func resolveLoadedPaths(loaded *show.Show, root string) {
	for i := range loaded.Cues {
		cue := &loaded.Cues[i]
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Sound.File), "media/") {
				cue.Play.Sound.File = filepath.Join(root, filepath.FromSlash(cue.Play.Sound.File))
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Video.File), "media/") {
				cue.Play.Video.File = filepath.Join(root, filepath.FromSlash(cue.Play.Video.File))
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Image.File), "media/") {
				cue.Play.Image.File = filepath.Join(root, filepath.FromSlash(cue.Play.Image.File))
			}
		}
	}
}
