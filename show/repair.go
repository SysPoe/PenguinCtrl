package show

// TODO(macro): Define a single cue-invariant repair pass — this only strips
// extra CuePlay arms and does not create missing payloads, normalize groups,
// clear invalid links/targets, or align Type with content, so "repair" is not a
// complete domain boundary for import/save.
// RepairCueData removes stale play payloads that do not belong to the cue's
// selected type. It is safe to apply when saving an imported or legacy cue.
func RepairCueData(cue *Cue) bool {
	if cue == nil {
		return false
	}
	original := cue.Play
	// TODO(micro): comparison cue.Play != original is shallow; pointer identity only detects arm swaps, not deep field changes — fine for strip-only but document that
	switch cue.Type {
	case CueTypeSound:
		cue.Play = CuePlay{Sound: original.Sound}
	case CueTypeVideo:
		cue.Play = CuePlay{Video: original.Video}
	case CueTypeImage:
		cue.Play = CuePlay{Image: original.Image}
	case CueTypeRemote:
		cue.Play = CuePlay{Remote: original.Remote}
	case CueTypeWait:
		cue.Play = CuePlay{Wait: original.Wait}
	case CueTypeMediaControl:
		cue.Play = CuePlay{MediaControl: original.MediaControl}
	case CueTypeOutputControl:
		cue.Play = CuePlay{OutputControl: original.OutputControl}
	default:
		return false
	}
	return cue.Play != original
}
