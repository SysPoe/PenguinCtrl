package redundancy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
)

// BuildFingerprint constructs the production identity shared with a warm spare.
// Machine-local media paths and operator-local acknowledgement state are excluded.
func BuildFingerprint(current show.Show, settings config.Settings, files []project.File, preflightReady bool) (Fingerprint, error) {
	logicalShow, mediaHash, mediaReady, err := productionIdentity(current, settings, files)
	if err != nil {
		return Fingerprint{}, err
	}
	showHash, err := logicalShow.Digest()
	if err != nil {
		return Fingerprint{}, err
	}
	routingHash, err := routingDigest(settings)
	if err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{
		Show: hex.EncodeToString(showHash[:]), Media: mediaHash, Routing: routingHash,
		Ready: preflightReady && mediaReady,
	}, nil
}

type mediaIdentity struct {
	Kind string `json:"kind"`
	Hash string `json:"sha256"`
}

func productionIdentity(current show.Show, settings config.Settings, files []project.File) (show.Show, string, bool, error) {
	available := mediaIndex(files)
	logical := current
	logical.AcknowledgedProblems = nil
	logical.Cues = make([]show.Cue, len(current.Cues))
	identities := make(map[mediaIdentity]struct{})
	ready := true
	for index, cue := range current.Cues {
		logical.Cues[index] = show.CloneCue(cue)
		for _, source := range project.ResolvedMediaSources(cue, settings) {
			path, ok := canonicalMediaPath(source)
			identity, exists := available[path]
			if !ok || !exists {
				ready = false
				continue
			}
			identities[identity] = struct{}{}
			canonicalizeCueMedia(&logical.Cues[index], identity)
		}
	}
	digest, err := mediaIdentityDigest(identities)
	return logical, digest, ready, err
}

func mediaIndex(files []project.File) map[string]mediaIdentity {
	available := make(map[string]mediaIdentity, len(files))
	for _, file := range files {
		path, ok := canonicalMediaPath(file.Source)
		hash := strings.ToLower(strings.TrimSpace(file.Hash))
		decoded, err := hex.DecodeString(hash)
		if !ok || err != nil || len(decoded) != sha256.Size {
			continue
		}
		available[path] = mediaIdentity{Kind: strings.ToLower(strings.TrimSpace(file.Kind)), Hash: hash}
	}
	return available
}

func mediaIdentityDigest(identities map[mediaIdentity]struct{}) (string, error) {
	ordered := make([]mediaIdentity, 0, len(identities))
	for identity := range identities {
		ordered = append(ordered, identity)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		return ordered[i].Hash < ordered[j].Hash
	})
	return digestJSON(ordered)
}

func canonicalizeCueMedia(cue *show.Cue, identity mediaIdentity) {
	canonical := "sha256:" + identity.Kind + ":" + identity.Hash
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			cue.Play.Sound.File = canonical
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			cue.Play.Video.File = canonical
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			cue.Play.Image.File = canonical
		}
	}
}

func routingDigest(settings config.Settings) (string, error) {
	video := append([]config.VideoOutput(nil), settings.VideoOutputs...)
	sort.Slice(video, func(i, j int) bool { return video[i].Stage < video[j].Stage })
	remote := append([]config.RemoteTarget(nil), settings.RemoteTargets...)
	sort.Slice(remote, func(i, j int) bool {
		if remote[i].Name != remote[j].Name {
			return remote[i].Name < remote[j].Name
		}
		return remote[i].Host < remote[j].Host
	})
	routing := struct {
		DefaultPlayback, DefaultMediaOutput                                   string
		PlaybackAudioDevice, PlaybackAudioRecovery, PlaybackBackupAudioDevice string
		PreviewAudioDevice, PreviewAudioRecovery, PreviewBackupAudioDevice    string
		RemoteSuccessPolicy, TimecodeSource, TimecodePolicy                   string
		TimecodeFrameRate                                                     float64
		Video                                                                 []config.VideoOutput
		Remote                                                                []config.RemoteTarget
		Variables                                                             map[string]string
	}{
		settings.DefaultPlayback, settings.DefaultMediaOutput,
		settings.PlaybackAudioDevice, settings.PlaybackAudioRecovery, settings.PlaybackBackupAudioDevice,
		settings.PreviewAudioDevice, settings.PreviewAudioRecovery, settings.PreviewBackupAudioDevice,
		settings.RemoteSuccessPolicy, settings.TimecodeSource, settings.TimecodePolicy,
		settings.TimecodeFrameRate, video, remote, settings.Variables,
	}
	return digestJSON(routing)
}

func canonicalMediaPath(source string) (string, bool) {
	path, err := project.LocalPath(source)
	if err != nil {
		return "", false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return strings.ToLower(filepath.Clean(path)), true
}

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
