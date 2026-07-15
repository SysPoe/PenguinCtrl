package project

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syspoe/cusus/internal/atomicfile"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): Split package project by responsibility — archive I/O, schema
// migration, content-addressed library, disk cache GC, and recovery journal all
// share one package name, so "project" is a grab-bag rather than a coherent
// domain boundary. Split archive, library, cache, and journal so FFmpeg publish
// policy, in-memory pick list, and recovery journal do not share one import surface.
const (
	Format                 = "cusus-show"
	Version                = 3
	oldestSupportedVersion = 1

	maxArchiveEntries = 16384
	maxManifestBytes  = 16 << 20
	maxAssetBytes     = 16 << 30
	maxArchiveBytes   = 128 << 30
	maxExpansionRatio = 1000

	archiveHashPrefixLength = 24
)

type Asset struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	SourceSHA256  string `json:"sourceSha256"`
	ContentSHA256 string `json:"contentSha256,omitempty"`
	Format        string `json:"format"`
	Size          int64  `json:"size"`
}

type Manifest struct {
	Format          string                     `json:"format"`
	Version         int                        `json:"version"`
	Show            show.Show                  `json:"show"`
	Assets          []Asset                    `json:"assets"`
	Extensions      map[string]json.RawMessage `json:"extensions,omitempty"`
	OriginalVersion int                        `json:"-"`
}

type SaveProgress struct {
	Current int
	Total   int
	Kind    string
	Name    string
}

// Save writes a portable .cusus ZIP archive.
func Save(dst io.Writer, current show.Show, ffmpegPath string) (Manifest, error) {
	return SaveWithProgress(dst, current, ffmpegPath, nil)
}

// SaveAtPathWithProgress publishes a portable show archive atomically while
// retaining the previous archive as a last-known-good backup.
func SaveAtPathWithProgress(path string, current show.Show, ffmpegPath string, progress func(SaveProgress)) (Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return Manifest{}, errors.New("show has no file path; use Save As")
	}
	var manifest Manifest
	err := atomicfile.WriteFunc(path, 0o600, func(file *os.File) error {
		var err error
		manifest, err = SaveWithProgress(file, current, ffmpegPath, progress)
		return err
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("save show archive: %w", err)
	}
	return manifest, nil
}

// TODO(macro): Keep archive-relative media refs distinct from runtime absolute
// paths — Save rewrites cue File fields to media/... and Load rewrites them to
// cache-absolute paths on the same show.Show type, so the domain model cannot
// tell portable document paths from machine-local playback paths.
// SaveWithProgress writes a portable .cusus ZIP archive and reports each asset
// before it is prepared. Video containers supported by the playback engine are
// bundled unchanged so saving a show does not require a lengthy offline encode.
// Audio and images retain the normalized Opus/WebP archive representation.
func SaveWithProgress(dst io.Writer, current show.Show, ffmpegPath string, progress func(SaveProgress)) (Manifest, error) {
	manifest := Manifest{Format: Format, Version: Version, Show: show.CloneShow(current)}
	normalizeShowSchema(&manifest.Show, Version)
	if err := validateManifestSchema(manifest); err != nil {
		return Manifest{}, err
	}
	type pendingAsset struct {
		asset  Asset
		source string
	}
	assets := map[string]pendingAsset{}
	usedAssetPaths := map[string]struct{}{}

	// TODO(macro): Ask the show domain for a cue's portable assets through one
	// asset-enumeration contract. This archive-layer cue-type switch duplicates
	// domain knowledge and can silently omit assets when a media cue type evolves.
	for i := range manifest.Show.Cues {
		cue := &manifest.Show.Cues[i]
		var source, kind string
		var replace func(string)
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil {
				source, kind = cue.Play.Sound.File, "audio"
				replace = func(path string) { cue.Play.Sound.File = path }
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				source, kind = cue.Play.Video.File, "video"
				replace = func(path string) { cue.Play.Video.File = path }
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				source, kind = cue.Play.Image.File, "image"
				replace = func(path string) { cue.Play.Image.File = path }
			}
		}
		if strings.TrimSpace(source) == "" || replace == nil {
			continue
		}
		path, err := LocalPath(source)
		if err != nil {
			return Manifest{}, fmt.Errorf("cue %q: %w", cue.CueNumber, err)
		}
		hash, err := HashFile(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("cue %q: %w", cue.CueNumber, err)
		}
		key := kind + ":" + hash
		pending, ok := assets[key]
		if !ok {
			ext, format := archiveAssetFormat(kind, path)
			id := hash[:archiveHashPrefixLength] + "-" + kind
			pending = pendingAsset{asset: Asset{
				ID: id, Name: filepath.Base(path), Kind: kind,
				Path: uniqueAssetPath(path, ext, usedAssetPaths), SourceSHA256: hash, Format: format,
			}, source: path}
			assets[key] = pending
		}
		replace(pending.asset.Path)
	}

	zw := zip.NewWriter(dst)
	preparer := newMediaPreparer(ffmpegPath)
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	// Stable ordering keeps archives reproducible for the same inputs.
	sort.Strings(keys)
	for index, key := range keys {
		pending := assets[key]
		if progress != nil {
			progress(SaveProgress{Current: index + 1, Total: len(keys), Kind: pending.asset.Kind, Name: pending.asset.Name})
		}
		converted, err := preparer.Prepare(pending.source, pending.asset.Kind, pending.asset.Format, pending.asset.SourceSHA256)
		if err != nil {
			return Manifest{}, fmt.Errorf("prepare %s %q: %w", pending.asset.Kind, pending.asset.Name, err)
		}
		info, err := os.Stat(converted)
		if err != nil {
			return Manifest{}, err
		}
		pending.asset.Size = info.Size()
		pending.asset.ContentSHA256, err = HashFile(converted)
		if err != nil {
			return Manifest{}, fmt.Errorf("hash bundled %s %q: %w", pending.asset.Kind, pending.asset.Name, err)
		}
		entry, err := zw.CreateHeader(&zip.FileHeader{Name: pending.asset.Path, Method: zip.Store})
		if err == nil {
			var input *os.File
			input, err = os.Open(converted)
			if err == nil {
				_, err = io.Copy(entry, input)
				err = errors.Join(err, input.Close())
			}
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("bundle %q: %w", pending.asset.Name, err)
		}
		manifest.Assets = append(manifest.Assets, pending.asset)
	}

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode show manifest: %w", err)
	}
	entry, err := zw.Create("manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	if _, err := entry.Write(append(raw, '\n')); err != nil {
		return Manifest{}, err
	}
	if err := zw.Close(); err != nil {
		return Manifest{}, fmt.Errorf("finish show archive: %w", err)
	}
	return manifest, nil
}

func uniqueAssetPath(source, extension string, used map[string]struct{}) string {
	base := filepath.Base(source)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = base
	}
	for suffix := 0; ; suffix++ {
		name := stem + extension
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d%s", stem, suffix, extension)
		}
		path := "media/" + name
		key := strings.ToLower(path)
		if _, exists := used[key]; exists {
			continue
		}
		used[key] = struct{}{}
		return path
	}
}
