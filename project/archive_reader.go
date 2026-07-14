package project

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Load reads and extracts a .cusus file. Returned cue paths point at a stable
// per-archive cache directory, ready for the playback engine.
func Load(path string) (Manifest, []File, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open .cusus archive: %w", err)
	}
	// TODO(micro): Explicitly mark this read-only close as best effort or propagate its error through a named return.
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
	// TODO(micro): Combine the manifest reader's Close error with decode failure instead of discarding it.
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
	// TODO(micro): Explicitly mark failed temporary-tree cleanup as best effort or report it for cache hygiene.
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
		// TODO(micro): Fold the archive entry reader's Close error into err before deciding extraction succeeded.
		reader.Close()
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("extract %q: %w", asset.Name, err)
		}
		if copied != asset.Size {
			return Manifest{}, nil, fmt.Errorf("asset %q extracted %d bytes; expected %d", name, copied, asset.Size)
		}
		// TODO(micro): Use hex.EncodeToString for the digest instead of fmt.Sprintf.
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
