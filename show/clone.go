package show

// CloneCue returns a deep copy suitable for editing, copying, or duplicating.
func CloneCue(cue Cue) Cue {
	clone := cue
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
