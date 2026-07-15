package show

// These helpers are the canonical access layer for the persisted CuePlay
// optional-field union. Construction, repair, and validation must use them so
// the exactly-one-payload invariant is defined in one place.
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
