package project

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
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
				Path: "media/" + id + ext, SourceSHA256: hash, Format: format,
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

func prepareAsset(ffmpegPath, source, kind, format, sourceHash string) (string, error) {
	extension := strings.ToLower(filepath.Ext(source))
	if (kind == "video" && format == strings.TrimPrefix(extension, ".")) ||
		(kind == "audio" && extension == ".opus") ||
		(kind == "image" && extension == ".webp") {
		return source, nil
	}
	return transcode(ffmpegPath, source, kind, sourceHash)
}

// Load reads and extracts a .cusus file. Returned cue paths point at a stable
// per-archive cache directory, ready for the playback engine.
func Load(path string) (Manifest, []File, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open .cusus archive: %w", err)
	}
	defer zr.Close()
	if len(zr.File) > maxArchiveEntries {
		return Manifest{}, nil, fmt.Errorf("archive has %d entries; limit is %d", len(zr.File), maxArchiveEntries)
	}
	entries := make(map[string]*zip.File, len(zr.File))
	var totalBytes uint64
	for _, entry := range zr.File {
		name := filepath.ToSlash(entry.Name)
		if _, duplicate := entries[name]; duplicate {
			return Manifest{}, nil, fmt.Errorf("duplicate archive entry %q", name)
		}
		entries[name] = entry
		totalBytes += entry.UncompressedSize64
		if totalBytes > maxArchiveBytes {
			return Manifest{}, nil, fmt.Errorf("archive expands beyond %d bytes", int64(maxArchiveBytes))
		}
		if entry.UncompressedSize64 > 0 && entry.CompressedSize64 == 0 {
			return Manifest{}, nil, fmt.Errorf("archive entry %q has an invalid compressed size", name)
		}
		if entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > maxExpansionRatio {
			return Manifest{}, nil, fmt.Errorf("archive entry %q exceeds the expansion-ratio limit", name)
		}
	}
	var manifest Manifest
	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		return Manifest{}, nil, fmt.Errorf("archive has no manifest.json")
	}
	if manifestEntry.UncompressedSize64 > maxManifestBytes {
		return Manifest{}, nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	reader, err := manifestEntry.Open()
	if err != nil {
		return Manifest{}, nil, err
	}
	err = decodeManifest(io.LimitReader(reader, maxManifestBytes+1), &manifest)
	reader.Close()
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("decode show manifest: %w", err)
	}
	if err := migrateManifest(&manifest); err != nil {
		return Manifest{}, nil, err
	}
	if err := validateManifestSchema(manifest); err != nil {
		return Manifest{}, nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return Manifest{}, nil, err
	}
	digest, err := HashFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	root := filepath.Join(cacheRoot, "CuSus", "shows", digest[:24])
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Manifest{}, nil, err
	}
	temporary, err := os.MkdirTemp(parent, ".extract-*")
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("create extraction directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := os.MkdirAll(filepath.Join(temporary, "media"), 0o755); err != nil {
		return Manifest{}, nil, err
	}
	assetByPath := make(map[string]Asset, len(manifest.Assets))
	assetIDs := make(map[string]struct{}, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		name := filepath.ToSlash(asset.Path)
		if err := validateAsset(asset, name, manifest.OriginalVersion == 1); err != nil {
			return Manifest{}, nil, err
		}
		if _, duplicate := assetByPath[name]; duplicate {
			return Manifest{}, nil, fmt.Errorf("duplicate manifest asset path %q", name)
		}
		if _, duplicate := assetIDs[asset.ID]; duplicate {
			return Manifest{}, nil, fmt.Errorf("duplicate manifest asset ID %q", asset.ID)
		}
		assetByPath[name] = asset
		assetIDs[asset.ID] = struct{}{}
	}
	if err := validateShowAssetReferences(manifest.Show, assetByPath); err != nil {
		return Manifest{}, nil, err
	}
	files := make([]File, 0, len(manifest.Assets))
	for name, asset := range assetByPath {
		entry := entries[name]
		if entry == nil {
			return Manifest{}, nil, fmt.Errorf("manifest asset %q is missing from archive", name)
		}
		if entry.UncompressedSize64 != uint64(asset.Size) {
			return Manifest{}, nil, fmt.Errorf("asset %q size is %d bytes; manifest declares %d", name, entry.UncompressedSize64, asset.Size)
		}
		target := filepath.Join(temporary, filepath.FromSlash(name))
		reader, err := entry.Open()
		if err != nil {
			return Manifest{}, nil, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		hash := sha256.New()
		var copied int64
		if err == nil {
			copied, err = io.Copy(io.MultiWriter(out, hash), io.LimitReader(reader, asset.Size+1))
			if syncErr := out.Sync(); err == nil {
				err = syncErr
			}
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
		}
		reader.Close()
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("extract %q: %w", asset.Name, err)
		}
		if copied != asset.Size {
			return Manifest{}, nil, fmt.Errorf("asset %q extracted %d bytes; expected %d", name, copied, asset.Size)
		}
		contentHash := fmt.Sprintf("%x", hash.Sum(nil))
		if asset.ContentSHA256 != "" && !strings.EqualFold(contentHash, asset.ContentSHA256) {
			return Manifest{}, nil, fmt.Errorf("asset %q failed SHA-256 verification", name)
		}
		files = append(files, File{Name: asset.Name, Source: filepath.Join(root, filepath.FromSlash(name)), Hash: asset.SourceSHA256, Kind: asset.Kind})
	}
	for name := range entries {
		if strings.HasPrefix(name, "media/") {
			if _, declared := assetByPath[name]; !declared {
				return Manifest{}, nil, fmt.Errorf("archive contains undeclared media entry %q", name)
			}
		}
	}
	if err := publishExtractedShow(temporary, root); err != nil {
		return Manifest{}, nil, err
	}
	touchCachePath(root)
	resolveLoadedPaths(&manifest.Show, root)
	return manifest, files, nil
}

func validateAsset(asset Asset, name string, allowMissingContentHash bool) error {
	if asset.ID == "" || strings.TrimSpace(asset.Name) == "" {
		return fmt.Errorf("manifest contains an asset with no ID or name")
	}
	if name != filepath.ToSlash(filepath.Clean(name)) || !strings.HasPrefix(name, "media/") || strings.Contains(name, "\\") {
		return fmt.Errorf("unsafe media path %q", asset.Path)
	}
	if asset.Size <= 0 || asset.Size > maxAssetBytes {
		return fmt.Errorf("asset %q has invalid size %d", name, asset.Size)
	}
	validFormat := false
	switch asset.Kind {
	case "audio":
		validFormat = asset.Format == "opus"
	case "image":
		validFormat = asset.Format == "webp"
	case "video":
		switch asset.Format {
		case "mp4", "mov", "mkv", "webm", "avi":
			validFormat = true
		}
	}
	if !validFormat {
		return fmt.Errorf("asset %q has unsupported kind/format %q/%q", name, asset.Kind, asset.Format)
	}
	for label, value := range map[string]string{"source": asset.SourceSHA256, "content": asset.ContentSHA256} {
		if value == "" && label == "content" && allowMissingContentHash {
			continue
		}
		if len(value) != sha256.Size*2 {
			return fmt.Errorf("asset %q has invalid %s SHA-256", name, label)
		}
		for _, char := range value {
			if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
				return fmt.Errorf("asset %q has invalid %s SHA-256", name, label)
			}
		}
	}
	return nil
}

func validateShowAssetReferences(current show.Show, assets map[string]Asset) error {
	for _, cue := range current.Cues {
		var source string
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil {
				source = cue.Play.Sound.File
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				source = cue.Play.Video.File
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				source = cue.Play.Image.File
			}
		}
		source = filepath.ToSlash(strings.TrimSpace(source))
		if source == "" {
			continue
		}
		if _, ok := assets[source]; !ok {
			return fmt.Errorf("cue %q references undeclared archive asset %q", cue.CueNumber, source)
		}
	}
	return nil
}

func publishExtractedShow(temporary, root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err := os.Rename(temporary, root); err != nil {
			return fmt.Errorf("publish extracted show: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect extracted show cache: %w", err)
	}
	backup := root + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove old extraction backup: %w", err)
	}
	if err := os.Rename(root, backup); err != nil {
		return fmt.Errorf("preserve extracted show cache: %w", err)
	}
	if err := os.Rename(temporary, root); err != nil {
		_ = os.Rename(backup, root)
		return fmt.Errorf("publish extracted show: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

func resolveLoadedPaths(loaded *show.Show, root string) {
	for i := range loaded.Cues {
		cue := &loaded.Cues[i]
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Sound.File), "media/") {
				cue.Play.Sound.File = filepath.Join(root, filepath.FromSlash(cue.Play.Sound.File))
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Video.File), "media/") {
				cue.Play.Video.File = filepath.Join(root, filepath.FromSlash(cue.Play.Video.File))
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil && strings.HasPrefix(filepath.ToSlash(cue.Play.Image.File), "media/") {
				cue.Play.Image.File = filepath.Join(root, filepath.FromSlash(cue.Play.Image.File))
			}
		}
	}
}

func transcode(ffmpegPath, source, kind, sourceHash string) (string, error) {
	ext := map[string]string{"audio": ".opus", "video": ".webm", "image": ".webp"}[kind]
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(cacheRoot, "CuSus", "transcoded")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	output := filepath.Join(cacheDir, sourceHash+"-"+kind+ext)
	if info, err := os.Stat(output); err == nil && info.Size() > 0 {
		touchCachePath(output)
		return output, nil
	}
	tmp, err := os.CreateTemp(cacheDir, "cusus-media-*"+ext)
	if err != nil {
		return "", err
	}
	temporary := tmp.Name()
	tmp.Close()
	_ = os.Remove(temporary)

	common := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", source, "-map_metadata", "-1"}
	var attempts [][]string
	switch kind {
	case "audio":
		attempts = [][]string{{"-vn", "-c:a", "libopus", "-b:a", "128k", "-vbr", "on", "-compression_level", "10", temporary}}
	case "video":
		attempts = [][]string{
			{"-c:v", "libsvtav1", "-preset", "8", "-crf", "32", "-pix_fmt", "yuv420p10le", "-c:a", "libopus", "-b:a", "128k", temporary},
			{"-c:v", "libvpx-vp9", "-crf", "31", "-b:v", "0", "-row-mt", "1", "-c:a", "libopus", "-b:a", "128k", temporary},
		}
	case "image":
		attempts = [][]string{{"-c:v", "libwebp", "-quality", "86", "-compression_level", "6", temporary}}
	default:
		return "", fmt.Errorf("unknown media kind %q", kind)
	}
	var lastOutput []byte
	for _, args := range attempts {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		lastOutput, err = processgroup.CombinedOutput(processgroup.CommandContext(ctx, ffmpegPath, append(common, args...)...))
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if err == nil {
			if err := os.Rename(temporary, output); err != nil {
				_ = os.Remove(temporary)
				return "", err
			}
			touchCachePath(output)
			return output, nil
		}
		_ = os.Remove(temporary)
		if timedOut {
			return "", fmt.Errorf("ffmpeg conversion timed out after 6h")
		}
	}
	return "", fmt.Errorf("ffmpeg conversion failed: %v: %s", err, strings.TrimSpace(string(lastOutput)))
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
