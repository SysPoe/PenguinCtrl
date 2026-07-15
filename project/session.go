package project

import (
	"encoding/json"
	"path/filepath"
	"slices"

	"github.com/syspoe/cusus/show"
)

// ProjectSession owns the boundary between a portable archive document and the
// machine-local media paths used while that document is open. The portable
// manifest remains unchanged; RuntimeShow and MediaFiles expose resolved cache
// paths, and ProtectedPaths identifies the cache object that must stay alive.
type ProjectSession struct {
	archivePath    string
	manifest       Manifest
	runtimeShow    show.Show
	library        Library
	protectedPaths []string
}

// OpenSession verifies an archive and constructs its complete runtime view.
func OpenSession(path string) (*ProjectSession, error) {
	archive, err := Extract(path)
	if err != nil {
		return nil, err
	}
	return newProjectSession(path, archive), nil
}

func newProjectSession(path string, archive ExtractedArchive) *ProjectSession {
	session := &ProjectSession{
		archivePath: filepath.Clean(path),
		manifest:    cloneManifest(archive.Manifest),
		runtimeShow: archive.HydrateShow(),
	}
	session.library.Replace(archive.Files)
	if archive.root != "" {
		session.protectedPaths = []string{filepath.Clean(archive.root)}
	}
	return session
}

// ArchivePath returns the .cusus archive from which this session was opened.
func (session *ProjectSession) ArchivePath() string {
	return session.archivePath
}

// PortableManifest returns an isolated snapshot whose cue media references
// remain archive-relative.
func (session *ProjectSession) PortableManifest() Manifest {
	return cloneManifest(session.manifest)
}

// RuntimeShow returns an isolated playback-ready snapshot with local paths.
func (session *ProjectSession) RuntimeShow() show.Show {
	return show.CloneShow(session.runtimeShow)
}

// MediaFiles returns the session library's local media entries.
func (session *ProjectSession) MediaFiles(kind string) []File {
	return session.library.Files(kind)
}

// AddMedia adds a local picker result to this session's content-addressed
// library. User-selected files do not expand the managed-cache protection set.
func (session *ProjectSession) AddMedia(source, kind string) (File, bool, error) {
	return session.library.Add(source, kind)
}

// ProtectedPaths returns the managed cache objects required by this session.
func (session *ProjectSession) ProtectedPaths() []string {
	return slices.Clone(session.protectedPaths)
}

func cloneManifest(manifest Manifest) Manifest {
	clone := manifest
	clone.Show = show.CloneShow(manifest.Show)
	clone.Assets = slices.Clone(manifest.Assets)
	if manifest.Extensions != nil {
		clone.Extensions = make(map[string]json.RawMessage, len(manifest.Extensions))
		for key, value := range manifest.Extensions {
			clone.Extensions[key] = slices.Clone(value)
		}
	}
	return clone
}
