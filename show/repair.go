package show

import "strings"

// RepairCueData enforces the payload invariant for a cue mutation. A valid cue
// type is authoritative: stale payload arms are removed and a missing payload
// is replaced with that type's safe default.
func RepairCueData(cue *Cue) bool {
	if cue == nil {
		return false
	}
	targetType := cue.Type
	if !validCueType(targetType) {
		if detected, ok := soleCuePlayType(cue.Play); ok {
			targetType = detected
		} else {
			targetType = CueTypeSound
		}
	}
	repaired, present := cuePlayForType(cue.Play, targetType)
	changed := cue.Type != targetType || !cuePlayContainsOnly(cue.Play, targetType)
	if !present {
		repaired = defaultCuePlay(targetType)
		changed = true
	}
	cue.Type, cue.Play = targetType, repaired
	return changed
}

// RepairShowData enforces invariants that require the complete cue list. For
// imported data, one unambiguous payload may correct a stale type tag before
// cue repair. Group titles are canonicalized and invalid links/media targets
// are cleared to safe states.
func RepairShowData(current *Show) bool {
	if current == nil {
		return false
	}
	changed := false
	for index := range current.Cues {
		cue := &current.Cues[index]
		if !cuePlayHasType(cue.Play, cue.Type) {
			if detected, ok := soleCuePlayType(cue.Play); ok && cue.Type != detected {
				cue.Type = detected
				changed = true
			}
		}
		changed = RepairCueData(cue) || changed
	}

	ids := make(map[CueID]struct{}, len(current.Cues))
	groupTitles := make(map[GroupID]string)
	for _, cue := range current.Cues {
		if cue.ID != (CueID{}) {
			ids[cue.ID] = struct{}{}
		}
		if cue.GroupID != (GroupID{}) {
			title := strings.TrimSpace(cue.GroupTitle)
			if _, exists := groupTitles[cue.GroupID]; !exists || groupTitles[cue.GroupID] == "" {
				groupTitles[cue.GroupID] = title
			}
		}
	}
	groups := make(map[GroupID]struct{}, len(groupTitles))
	for id := range groupTitles {
		groups[id] = struct{}{}
	}

	for index := range current.Cues {
		cue := &current.Cues[index]
		if cue.GroupID == (GroupID{}) {
			if cue.GroupTitle != "" {
				cue.GroupTitle = ""
				changed = true
			}
		} else if title := groupTitles[cue.GroupID]; cue.GroupTitle != title {
			cue.GroupTitle = title
			changed = true
		}
		changed = repairCueLink(&cue.Link, index, ids) || changed
		changed = repairCueMediaTargets(cue, ids, groups) || changed
	}
	return changed
}

func validCueType(cueType CueType) bool {
	return cueType >= CueTypeSound && cueType <= CueTypeOutputControl
}

func soleCuePlayType(play CuePlay) (CueType, bool) {
	found, count := CueTypeSound, 0
	for cueType := CueTypeSound; cueType <= CueTypeOutputControl; cueType++ {
		if cuePlayHasType(play, cueType) {
			found, count = cueType, count+1
		}
	}
	return found, count == 1
}

func cuePlayHasType(play CuePlay, cueType CueType) bool {
	switch cueType {
	case CueTypeSound:
		return play.Sound != nil
	case CueTypeVideo:
		return play.Video != nil
	case CueTypeImage:
		return play.Image != nil
	case CueTypeRemote:
		return play.Remote != nil
	case CueTypeWait:
		return play.Wait != nil
	case CueTypeMediaControl:
		return play.MediaControl != nil
	case CueTypeOutputControl:
		return play.OutputControl != nil
	default:
		return false
	}
}

func cuePlayContainsOnly(play CuePlay, cueType CueType) bool {
	detected, ok := soleCuePlayType(play)
	return ok && detected == cueType
}

func cuePlayForType(play CuePlay, cueType CueType) (CuePlay, bool) {
	switch cueType {
	case CueTypeSound:
		return CuePlay{Sound: play.Sound}, play.Sound != nil
	case CueTypeVideo:
		return CuePlay{Video: play.Video}, play.Video != nil
	case CueTypeImage:
		return CuePlay{Image: play.Image}, play.Image != nil
	case CueTypeRemote:
		return CuePlay{Remote: play.Remote}, play.Remote != nil
	case CueTypeWait:
		return CuePlay{Wait: play.Wait}, play.Wait != nil
	case CueTypeMediaControl:
		return CuePlay{MediaControl: play.MediaControl}, play.MediaControl != nil
	case CueTypeOutputControl:
		return CuePlay{OutputControl: play.OutputControl}, play.OutputControl != nil
	default:
		return CuePlay{}, false
	}
}

func defaultCuePlay(cueType CueType) CuePlay {
	switch cueType {
	case CueTypeVideo:
		return CuePlay{Video: &VideoPlay{OutputID: "{defaultMediaOutput}"}}
	case CueTypeImage:
		return CuePlay{Image: &ImagePlay{OutputID: "{defaultMediaOutput}"}}
	case CueTypeRemote:
		return CuePlay{Remote: &RemotePlay{Protocol: RemoteProtocolAuto, Action: RemoteActionGoto, Playback: "{defaultPlayback}", CueNumber: "{cueNumber}"}}
	case CueTypeWait:
		return CuePlay{Wait: &WaitPlay{Kind: WaitDuration, DurationMs: 1000, Target: CueTarget{Kind: CueTargetNone}, Media: MediaTarget{Kind: MediaTargetAllMedia}}}
	case CueTypeMediaControl:
		return CuePlay{MediaControl: &MediaControlPlay{Action: MediaControlPause, Target: MediaTarget{Kind: MediaTargetAllMedia}}}
	case CueTypeOutputControl:
		return CuePlay{OutputControl: &OutputControlPlay{Action: OutputControlTestPattern}}
	default:
		return CuePlay{Sound: &SoundPlay{OutputID: "{defaultMediaOutput}"}}
	}
}

func repairCueLink(link *CueLink, index int, ids map[CueID]struct{}) bool {
	if link == nil {
		return false
	}
	valid := link.Mode >= CueLinkManual && link.Mode <= CueLinkEndPlay
	if link.Mode == CueLinkManual {
		valid = link.Target.Kind == CueTargetNone
	} else {
		switch link.Target.Kind {
		case CueTargetNext:
		case CueTargetPrevious:
			valid = valid && index > 0
		case CueTargetCue:
			_, exists := ids[link.Target.CueID]
			valid = valid && link.Target.CueID != (CueID{}) && exists
		default:
			valid = false
		}
	}
	if valid {
		return false
	}
	*link = CueLink{Mode: CueLinkManual, Target: CueTarget{Kind: CueTargetNone}}
	return true
}

func repairCueMediaTargets(cue *Cue, ids map[CueID]struct{}, groups map[GroupID]struct{}) bool {
	changed := false
	if cue.Play.Wait != nil && cue.Play.Wait.Kind >= WaitMediaStart && cue.Play.Wait.Kind <= WaitInstanceStopped {
		changed = repairMediaTarget(&cue.Play.Wait.Media, ids, groups) || changed
	}
	if cue.Play.MediaControl != nil {
		changed = repairMediaTarget(&cue.Play.MediaControl.Target, ids, groups) || changed
	}
	for _, markers := range cueTimecodeMarkerSets(cue) {
		for index := range markers {
			if markers[index].Action.MediaControl != nil {
				changed = repairMediaTarget(&markers[index].Action.MediaControl.Target, ids, groups) || changed
			}
		}
	}
	return changed
}

func cueTimecodeMarkerSets(cue *Cue) [][]TimecodeMarker {
	var result [][]TimecodeMarker
	if cue.Play.Sound != nil {
		result = append(result, cue.Play.Sound.Timecode)
	}
	if cue.Play.Video != nil {
		result = append(result, cue.Play.Video.Timecode)
	}
	if cue.Play.Image != nil {
		result = append(result, cue.Play.Image.Timecode)
	}
	return result
}

func repairMediaTarget(target *MediaTarget, ids map[CueID]struct{}, groups map[GroupID]struct{}) bool {
	if target == nil {
		return false
	}
	valid := true
	switch target.Kind {
	case MediaTargetCue:
		_, valid = ids[target.CueID]
		valid = valid && target.CueID != (CueID{})
	case MediaTargetGroup:
		_, valid = groups[target.GroupID]
		valid = valid && target.GroupID != (GroupID{})
	case MediaTargetInstance:
		valid = strings.TrimSpace(target.InstanceID) != ""
	case MediaTargetOutput:
		valid = strings.TrimSpace(target.OutputID) != ""
	case MediaTargetAllAudio, MediaTargetAllVideo, MediaTargetAllMedia, MediaTargetCurrentTrack:
	default:
		valid = false
	}
	if valid {
		return false
	}
	*target = MediaTarget{Kind: MediaTargetAllMedia}
	return true
}
