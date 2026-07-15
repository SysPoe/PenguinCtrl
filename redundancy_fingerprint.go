package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): Production identity / media content fingerprinting is pure redundancy domain
// (show+media+routing digests) but lives in package main and depends on cueMediaSources from
// preflight. Move buildRedundancyFingerprint and helpers into package redundancy (or project
// identity) so warm-spare matching is not owned by the Gio composition root.
func buildRedundancyFingerprint(current show.Show, settings config.Settings, files []project.File, preflightReady bool) (redundancy.Fingerprint, error) {
	logicalShow, mediaHash, mediaReady, err := redundancyProductionIdentity(current, settings, files)
	if err != nil {
		return redundancy.Fingerprint{}, err
	}
	showHash, err := logicalShow.Digest()
	if err != nil {
		return redundancy.Fingerprint{}, err
	}
	routingHash, err := redundancyRoutingDigest(settings)
	if err != nil {
		return redundancy.Fingerprint{}, err
	}
	return redundancy.Fingerprint{
		Show: hex.EncodeToString(showHash[:]), Media: mediaHash, Routing: routingHash,
		Ready: preflightReady && mediaReady,
	}, nil
}

func updateRedundancyFingerprint(service *redundancy.Service, current show.Show, settings config.Settings, files []project.File, preflightReady bool, previousError string, report func(string)) string {
	fingerprint, err := buildRedundancyFingerprint(current, settings, files, preflightReady)
	if err == nil {
		service.UpdateFingerprint(fingerprint)
		return ""
	}
	service.UpdateFingerprint(redundancy.Fingerprint{})
	if message := err.Error(); message != previousError {
		if report != nil {
			report(message)
		}
		return message
	}
	return previousError
}

func redundancyPreflightReady(checks []operatorlog.PreflightCheck) bool {
	for _, check := range checks {
		if check.Severity == operatorlog.ShowStopping {
			return false
		}
	}
	return true
}

type redundancyMediaIdentity struct {
	Kind string `json:"kind"`
	Hash string `json:"sha256"`
}

func redundancyProductionIdentity(current show.Show, settings config.Settings, files []project.File) (show.Show, string, bool, error) {
	available := redundancyMediaIndex(files)
	logical := current
	logical.AcknowledgedProblems = nil
	logical.Cues = make([]show.Cue, len(current.Cues))
	identities := make(map[redundancyMediaIdentity]struct{})
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
	digest, err := redundancyMediaIdentityDigest(identities)
	return logical, digest, ready, err
}

func redundancyMediaIndex(files []project.File) map[string]redundancyMediaIdentity {
	available := make(map[string]redundancyMediaIdentity, len(files))
	for _, file := range files {
		path, ok := canonicalMediaPath(file.Source)
		hash := strings.ToLower(strings.TrimSpace(file.Hash))
		decoded, err := hex.DecodeString(hash)
		if !ok || err != nil || len(decoded) != sha256.Size {
			continue
		}
		available[path] = redundancyMediaIdentity{Kind: strings.ToLower(strings.TrimSpace(file.Kind)), Hash: hash}
	}
	return available
}

func redundancyMediaIdentityDigest(identities map[redundancyMediaIdentity]struct{}) (string, error) {
	ordered := make([]redundancyMediaIdentity, 0, len(identities))
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

func canonicalizeCueMedia(cue *show.Cue, identity redundancyMediaIdentity) {
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

func redundancyRoutingDigest(settings config.Settings) (string, error) {
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
