package show

import "encoding/json"

// CloneShow returns a deep copy suitable for archive preparation or editing.
// TODO(macro): Clone is hand-maintained field-by-field for every CuePlay payload
// and will drift whenever a play type gains nested slices/maps. Prefer a single
// generated or reflection-backed deep clone (or typed sum-type Clone methods)
// so archive/edit/duplicate paths cannot silently share nested pointers.
func CloneShow(current Show) Show {
	clone := Show{
		Title:      current.Title,
		Cues:       cloneCues(current.Cues),
		Extensions: cloneExtensions(current.Extensions),
	}
	// TODO(micro): Reuse cloneAcknowledgements instead of a second map-copy path with different empty/false semantics.
	if current.AcknowledgedProblems != nil {
		// TODO(micro): replace m[k]=v loop with maps.Copy
		clone.AcknowledgedProblems = make(map[string]bool, len(current.AcknowledgedProblems))
		for fingerprint, acknowledged := range current.AcknowledgedProblems {
			clone.AcknowledgedProblems[fingerprint] = acknowledged
		}
	}
	return clone
}

// CloneCue returns a deep copy suitable for editing, copying, or duplicating.
func CloneCue(cue Cue) Cue {
	clone := cue
	// TODO(micro): prefer slices.Clone(cue.Tags) over append([]string(nil), ...)
	clone.Tags = append([]string(nil), cue.Tags...)
	clone.Play = cloneCuePlay(cue.Play)
	return clone
}

func cloneCuePlay(play CuePlay) CuePlay {
	clone := CuePlay{}
	if play.Sound != nil {
		value := *play.Sound
		value.Timecode = cloneTimecode(value.Timecode)
		clone.Sound = &value
	}
	if play.Video != nil {
		value := *play.Video
		value.Timecode = cloneTimecode(value.Timecode)
		clone.Video = &value
	}
	if play.Image != nil {
		value := *play.Image
		value.Timecode = cloneTimecode(value.Timecode)
		clone.Image = &value
	}
	if play.Remote != nil {
		value := *play.Remote
		// TODO(micro): prefer slices.Clone(value.Values) over append([]RemoteValue(nil), ...)
		value.Values = append([]RemoteValue(nil), value.Values...)
		clone.Remote = &value
	}
	if play.Wait != nil {
		value := *play.Wait
		clone.Wait = &value
	}
	if play.MediaControl != nil {
		value := *play.MediaControl
		if value.LevelDB != nil {
			level := *value.LevelDB
			value.LevelDB = &level
		}
		if value.SeekToMs != nil {
			position := *value.SeekToMs
			value.SeekToMs = &position
		}
		clone.MediaControl = &value
	}
	if play.OutputControl != nil {
		value := *play.OutputControl
		clone.OutputControl = &value
	}
	return clone
}

func cloneTimecode(markers []TimecodeMarker) []TimecodeMarker {
	clone := make([]TimecodeMarker, len(markers))
	for index, marker := range markers {
		clone[index] = marker
		clone[index].Action = cloneCuePlay(marker.Action)
	}
	return clone
}

func cloneExtensions(extensions map[string]json.RawMessage) map[string]json.RawMessage {
	if extensions == nil {
		return nil
	}
	clone := make(map[string]json.RawMessage, len(extensions))
	for key, value := range extensions {
		// TODO(micro): prefer slices.Clone(value) over append(json.RawMessage(nil), value...)
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}
