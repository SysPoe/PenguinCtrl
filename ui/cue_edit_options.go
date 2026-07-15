package ui

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

func cueDropdownLabel(cue show.Cue) string {
	number := strings.TrimSpace(cue.CueNumber)
	description := strings.TrimSpace(cue.Description)

	switch {
	case number != "" && description != "":
		return number + " - " + description
	case number != "":
		return number
	case description != "":
		return description
	default:
		return "Untitled cue"
	}
}

func cueDropdownSelectedIndex(items []input.DropdownItem, selectedCueID show.CueID) int {
	if selectedCueID != (show.CueID{}) {
		selectedValue := uuid.UUID(selectedCueID).String()
		for i, item := range items {
			if item.Value == selectedValue {
				return i
			}
		}
	}
	return 0
}

func newEnumDropdown(labels []string, selected int) *input.Dropdown {
	items := make([]input.DropdownItem, len(labels))
	for i, label := range labels {
		items[i] = input.DropdownItem{
			Label: label,
			Value: strconv.Itoa(i),
		}
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	return input.NewDropdown(items, selected)
}

func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func waitKindUsesMediaTarget(kind show.WaitKind) bool {
	return kind == show.WaitMediaStart ||
		kind == show.WaitMediaEnd ||
		kind == show.WaitFadeInComplete ||
		kind == show.WaitFadeOutComplete ||
		kind == show.WaitInstanceStopped
}

func mediaControlActionUsesLevel(action show.MediaControlAction) bool {
	return action == show.MediaControlFadeTo ||
		action == show.MediaControlSetVolume
}

// normalizeCueEditModel applies the invariants that the old frame-by-frame row
// binding established on first paint. Doing this once keeps row layout pure
// without changing the working cue produced by opening the editor.
func normalizeCueEditModel(cue *show.Cue) {
	if cue == nil {
		return
	}
	if cue.Link.Mode != show.CueLinkManual && cue.Link.Target.Kind == show.CueTargetNone {
		cue.Link.Target.Kind = show.CueTargetNext
	}
	play := cue.Play.MediaControl
	if play == nil {
		return
	}
	if mediaControlActionUsesLevel(play.Action) {
		if play.LevelDB == nil {
			play.LevelDB = ptr(0.0)
		}
	} else {
		play.LevelDB = nil
	}
	if play.Action == show.MediaControlSeek {
		if play.SeekToMs == nil {
			play.SeekToMs = ptr(int64(0))
		}
	} else {
		play.SeekToMs = nil
	}
}

func syncMediaControlOptionals(play *show.MediaControlPlay, fields *cueMediaControlInputs) {
	if mediaControlActionUsesLevel(play.Action) {
		play.LevelDB = &fields.levelDB.Value
	} else {
		play.LevelDB = nil
	}

	if play.Action == show.MediaControlSeek {
		play.SeekToMs = ptr(int64(fields.seekToMs.Value))
	} else {
		play.SeekToMs = nil
	}
}

// Fixes being unable to do e.g. &int64(5) in Go. So you would do ptr(int64(5)) instead.
func ptr[T any](value T) *T {
	return &value
}
