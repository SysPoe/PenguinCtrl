package project

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

const (
	Format                 = "cusus-show"
	Version                = 3
	oldestSupportedVersion = 1

	maxArchiveEntries = 16384
	maxManifestBytes  = 16 << 20
	maxAssetBytes     = 16 << 30
	maxArchiveBytes   = 128 << 30
	maxExpansionRatio = 1000
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
			id := hash[:24] + "-" + kind
			pending = pendingAsset{asset: Asset{
				ID: id, Name: filepath.Base(path), Kind: kind,
				Path: uniqueAssetPath(path, ext, usedAssetPaths), SourceSHA256: hash, Format: format,
			}, source: path}
			assets[key] = pending
		}
		replace(pending.asset.Path)
	}

	zw := zip.NewWriter(dst)
	defer zw.Close()
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	// Stable ordering keeps archives reproducible for the same inputs.
	sortStrings(keys)
	for index, key := range keys {
		pending := assets[key]
		if progress != nil {
			progress(SaveProgress{Current: index + 1, Total: len(keys), Kind: pending.asset.Kind, Name: pending.asset.Name})
		}
		converted, err := prepareAsset(ffmpegPath, pending.source, pending.asset.Kind, pending.asset.Format, pending.asset.SourceSHA256)
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
				input.Close()
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

func archiveAssetFormat(kind, source string) (extension, format string) {
	switch kind {
	case "audio":
		return ".opus", "opus"
	case "image":
		return ".webp", "webp"
	case "video":
		extension = strings.ToLower(filepath.Ext(source))
		switch extension {
		case ".mp4", ".mov", ".mkv", ".webm", ".avi":
			return extension, strings.TrimPrefix(extension, ".")
		default:
			return ".webm", "webm"
		}
	default:
		return "", ""
	}
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

func prepareAsset(ffmpegPath, source, kind, format, sourceHash string) (string, error) {
	extension := strings.ToLower(filepath.Ext(source))
	if (kind == "video" && format == strings.TrimPrefix(extension, ".")) ||
		(kind == "audio" && extension == ".opus") ||
		(kind == "image" && extension == ".webp") {
		return source, nil
	}
	return transcode(ffmpegPath, source, kind, sourceHash)
}
