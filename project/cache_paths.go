package project

import (
	"os"
	"path/filepath"
)

const (
	cacheApplicationDirectory = "CuSus"
	cacheShowsDirectory       = "shows"
	cacheTranscodedDirectory  = "transcoded"
)

// cacheLayout is the single owner of the on-disk project cache namespaces.
// Producers and maintenance must derive their paths through this layout so a
// namespace cannot be published outside the tree that maintenance reclaims.
type cacheLayout struct {
	Root       string
	Shows      string
	Transcoded string
}

func currentCacheLayout() (cacheLayout, error) {
	userRoot, err := os.UserCacheDir()
	if err != nil {
		return cacheLayout{}, err
	}
	return cacheLayoutAt(userRoot), nil
}

func cacheLayoutAt(userRoot string) cacheLayout {
	return cacheLayoutFromRoot(filepath.Join(userRoot, cacheApplicationDirectory))
}

func cacheLayoutFromRoot(root string) cacheLayout {
	root = filepath.Clean(root)
	return cacheLayout{
		Root:       root,
		Shows:      filepath.Join(root, cacheShowsDirectory),
		Transcoded: filepath.Join(root, cacheTranscodedDirectory),
	}
}

func (layout cacheLayout) objectRoots() []string {
	return []string{layout.Shows, layout.Transcoded}
}
