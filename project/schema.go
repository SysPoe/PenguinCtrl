package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
)

const maxShowCues = 100_000

func decodeManifest(reader io.Reader, manifest *Manifest) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(manifest); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("manifest contains multiple JSON values")
		}
		return fmt.Errorf("manifest has trailing data: %w", err)
	}
	return nil
}

func migrateManifest(manifest *Manifest) error {
	if manifest.Format != Format {
		return fmt.Errorf("unsupported .cusus format %q", manifest.Format)
	}
	if manifest.Version < oldestSupportedVersion {
		return fmt.Errorf(".cusus version %d predates the oldest supported version %d", manifest.Version, oldestSupportedVersion)
	}
	if manifest.Version > Version {
		return fmt.Errorf(".cusus version %d is newer than this build supports (%d); update CuSus rather than rewriting the only copy", manifest.Version, Version)
	}
	manifest.OriginalVersion = manifest.Version
	if len(manifest.Extensions) > 0 {
		if manifest.Show.Extensions == nil {
			manifest.Show.Extensions = map[string]json.RawMessage{}
		}
		for key, value := range manifest.Extensions {
			manifest.Show.Extensions[key] = append(json.RawMessage(nil), value...)
		}
		manifest.Extensions = nil
	}
	for manifest.Version < Version {
		switch manifest.Version {
		case 1:
			normalizeShowSchema(&manifest.Show, 2)
			manifest.Version = 2
		case 2:
			manifest.Version = 3
		default:
			return fmt.Errorf("no migration is registered from .cusus version %d", manifest.Version)
		}
	}
	normalizeShowSchema(&manifest.Show, manifest.Version)
	return nil
}

func normalizeShowSchema(current *show.Show, version int) {
	if current.Cues == nil {
		current.Cues = []show.Cue{}
	}
	if current.AcknowledgedProblems == nil {
		current.AcknowledgedProblems = map[string]bool{}
	}
	if current.Extensions == nil {
		current.Extensions = map[string]json.RawMessage{}
	}
	for index := range current.Cues {
		cue := &current.Cues[index]
		if cue.ID == (show.CueID{}) {
			// TODO(micro): SHA1 name-space ID for missing cue IDs is deterministic but non-V7; document stability contract or use NewCueID when inventing IDs is acceptable
			seed := fmt.Sprintf("cusus-v%d-cue-%d-%s-%s", version, index, cue.CueNumber, cue.Description)
			cue.ID = show.CueID(uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)))
		}
		if cue.Tags == nil {
			cue.Tags = []string{}
		}
	}
}

func validateManifestSchema(manifest Manifest) error {
	if manifest.Format != Format || manifest.Version != Version {
		return fmt.Errorf("manifest schema is %q version %d; expected %q version %d", manifest.Format, manifest.Version, Format, Version)
	}
	if len(manifest.Show.Cues) > maxShowCues {
		return fmt.Errorf("show contains %d cues; limit is %d", len(manifest.Show.Cues), maxShowCues)
	}
	seen := make(map[show.CueID]struct{}, len(manifest.Show.Cues))
	groups := make(map[show.GroupID]string)
	for index, cue := range manifest.Show.Cues {
		if cue.ID == (show.CueID{}) {
			return fmt.Errorf("cue %d has no stable ID", index+1)
		}
		if _, duplicate := seen[cue.ID]; duplicate {
			return fmt.Errorf("cue %d duplicates cue ID %v", index+1, cue.ID)
		}
		seen[cue.ID] = struct{}{}
		if cue.Type < show.CueTypeSound || cue.Type > show.CueTypeOutputControl {
			return fmt.Errorf("cue %d has unknown type %d", index+1, cue.Type)
		}
		if cue.Timing.PreWaitMs < 0 || cue.Timing.PostWaitMs < 0 {
			return fmt.Errorf("cue %d has negative timing", index+1)
		}
		if !cuePayloadMatchesType(cue) {
			return fmt.Errorf("cue %d payload does not match type %d", index+1, cue.Type)
		}
		if cue.GroupID == (show.GroupID{}) {
			if cue.GroupTitle != "" {
				return fmt.Errorf("cue %d has a group title without a group ID", index+1)
			}
		} else if title, exists := groups[cue.GroupID]; exists && title != cue.GroupTitle {
			return fmt.Errorf("cue %d has inconsistent title for its group", index+1)
		} else {
			groups[cue.GroupID] = cue.GroupTitle
		}
	}
	for index, cue := range manifest.Show.Cues {
		if cue.Link.Mode != show.CueLinkManual && cue.Link.Target.Kind == show.CueTargetCue {
			if _, exists := seen[cue.Link.Target.CueID]; !exists {
				return fmt.Errorf("cue %d links to an unknown cue ID", index+1)
			}
		}
		for _, target := range cueMediaTargets(cue) {
			switch target.Kind {
			case show.MediaTargetCue:
				if _, exists := seen[target.CueID]; !exists {
					return fmt.Errorf("cue %d targets an unknown media cue ID", index+1)
				}
			case show.MediaTargetGroup:
				if _, exists := groups[target.GroupID]; !exists {
					return fmt.Errorf("cue %d targets an unknown media group ID", index+1)
				}
			}
		}
	}
	return nil
}

func cuePayloadMatchesType(cue show.Cue) bool {
	present := 0
	for _, configured := range []bool{
		cue.Play.Sound != nil, cue.Play.Video != nil, cue.Play.Image != nil,
		cue.Play.Remote != nil, cue.Play.Wait != nil, cue.Play.MediaControl != nil,
		cue.Play.OutputControl != nil,
	} {
		if configured {
			present++
		}
	}
	if present != 1 {
		return false
	}
	switch cue.Type {
	case show.CueTypeSound:
		return cue.Play.Sound != nil
	case show.CueTypeVideo:
		return cue.Play.Video != nil
	case show.CueTypeImage:
		return cue.Play.Image != nil
	case show.CueTypeRemote:
		return cue.Play.Remote != nil
	case show.CueTypeWait:
		return cue.Play.Wait != nil
	case show.CueTypeMediaControl:
		return cue.Play.MediaControl != nil
	case show.CueTypeOutputControl:
		return cue.Play.OutputControl != nil
	default:
		return false
	}
}

func cueMediaTargets(cue show.Cue) []show.MediaTarget {
	var targets []show.MediaTarget
	if cue.Play.Wait != nil && cue.Play.Wait.Kind >= show.WaitMediaStart && cue.Play.Wait.Kind <= show.WaitInstanceStopped {
		targets = append(targets, cue.Play.Wait.Media)
	}
	if cue.Play.MediaControl != nil {
		targets = append(targets, cue.Play.MediaControl.Target)
	}
	for _, markers := range [][]show.TimecodeMarker{
		func() []show.TimecodeMarker {
			if cue.Play.Sound != nil {
				return cue.Play.Sound.Timecode
			}
			return nil
		}(),
		func() []show.TimecodeMarker {
			if cue.Play.Video != nil {
				return cue.Play.Video.Timecode
			}
			return nil
		}(),
		func() []show.TimecodeMarker {
			if cue.Play.Image != nil {
				return cue.Play.Image.Timecode
			}
			return nil
		}(),
	} {
		for _, marker := range markers {
			if play := marker.Action.MediaControl(); play != nil {
				targets = append(targets, play.Target)
			}
		}
	}
	return targets
}
