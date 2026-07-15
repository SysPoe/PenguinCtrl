package cache

import (
	"os"
	"path/filepath"
)

const (
	ApplicationDirectory = "CuSus"
	ShowsDirectory       = "shows"
	TranscodedDirectory  = "transcoded"
)

type Layout struct {
	Root       string
	Shows      string
	Transcoded string
}

func CurrentLayout() (Layout, error) {
	userRoot, err := os.UserCacheDir()
	if err != nil {
		return Layout{}, err
	}
	return LayoutAt(userRoot), nil
}

func LayoutAt(userRoot string) Layout {
	return LayoutFromRoot(filepath.Join(userRoot, ApplicationDirectory))
}

func LayoutFromRoot(root string) Layout {
	root = filepath.Clean(root)
	return Layout{
		Root:       root,
		Shows:      filepath.Join(root, ShowsDirectory),
		Transcoded: filepath.Join(root, TranscodedDirectory),
	}
}

func (layout Layout) ObjectRoots() []string {
	return []string{layout.Shows, layout.Transcoded}
}
