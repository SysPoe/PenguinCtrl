package project

import projectcache "github.com/syspoe/cusus/project/internal/cache"

const (
	cacheApplicationDirectory = projectcache.ApplicationDirectory
	cacheShowsDirectory       = projectcache.ShowsDirectory
	cacheTranscodedDirectory  = projectcache.TranscodedDirectory
)

type cacheLayout struct {
	Root       string
	Shows      string
	Transcoded string
}

func currentCacheLayout() (cacheLayout, error) {
	layout, err := projectcache.CurrentLayout()
	return cacheLayoutFromInternal(layout), err
}

func cacheLayoutAt(userRoot string) cacheLayout {
	return cacheLayoutFromInternal(projectcache.LayoutAt(userRoot))
}

func cacheLayoutFromRoot(root string) cacheLayout {
	return cacheLayoutFromInternal(projectcache.LayoutFromRoot(root))
}

func cacheLayoutFromInternal(layout projectcache.Layout) cacheLayout {
	return cacheLayout{Root: layout.Root, Shows: layout.Shows, Transcoded: layout.Transcoded}
}

func (layout cacheLayout) objectRoots() []string {
	return []string{layout.Shows, layout.Transcoded}
}
