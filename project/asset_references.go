package project

import (
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

const (
	assetKindAudio = "audio"
	assetKindVideo = "video"
	assetKindImage = "image"
)

// showAssetReference is the project boundary's descriptor for a cue-owned
// media path. A reference may contain a runtime source before publication or a
// portable media/... path in a manifest; all conversions use this descriptor.
type showAssetReference struct {
	CueNumber string
	Kind      string
	path      *string
}

func (reference showAssetReference) Path() string {
	if reference.path == nil {
		return ""
	}
	return *reference.path
}

func (reference showAssetReference) SetPath(path string) {
	if reference.path != nil {
		*reference.path = path
	}
}

func (reference showAssetReference) PortablePath() string {
	return filepath.ToSlash(strings.TrimSpace(reference.Path()))
}

func showAssetReferenceForCue(cue *show.Cue) (showAssetReference, bool) {
	reference := showAssetReference{CueNumber: cue.CueNumber}
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			reference.Kind, reference.path = assetKindAudio, &cue.Play.Sound.File
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			reference.Kind, reference.path = assetKindVideo, &cue.Play.Video.File
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			reference.Kind, reference.path = assetKindImage, &cue.Play.Image.File
		}
	}
	return reference, reference.path != nil
}

func visitShowAssetReferences(current *show.Show, visit func(showAssetReference) error) error {
	for index := range current.Cues {
		reference, ok := showAssetReferenceForCue(&current.Cues[index])
		if !ok {
			continue
		}
		if err := visit(reference); err != nil {
			return err
		}
	}
	return nil
}

func resolveLoadedPaths(loaded *show.Show, root string) {
	_ = visitShowAssetReferences(loaded, func(reference showAssetReference) error {
		portable := reference.PortablePath()
		if strings.HasPrefix(portable, "media/") {
			reference.SetPath(filepath.Join(root, filepath.FromSlash(portable)))
		}
		return nil
	})
}
