package project

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

// ExtractedArchive keeps durable archive data separate from its machine-local
// extraction. Manifest.Show retains portable media/... references; Files point
// at the verified cache copies available to the current session.
type ExtractedArchive struct {
	Manifest Manifest
	Files    []File
	root     string
}

// HydrateShow returns a playback-ready copy without mutating the portable
// manifest retained by the extraction result.
func (archive ExtractedArchive) HydrateShow() show.Show {
	loaded := show.CloneShow(archive.Manifest.Show)
	resolveLoadedPaths(&loaded, archive.root)
	return loaded
}

// Load is the compatibility entry point for callers that expect a hydrated
// Manifest.Show and a library snapshot in one operation. New session code can
// use Extract and call HydrateShow explicitly at the runtime boundary.
func Load(path string) (Manifest, []File, error) {
	archive, err := Extract(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	manifest := archive.Manifest
	manifest.Show = archive.HydrateShow()
	return manifest, archive.Files, nil
}

// Extract verifies and publishes a .cusus archive while preserving its
// archive-relative manifest references.
func Extract(path string) (result ExtractedArchive, resultErr error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ExtractedArchive{}, fmt.Errorf("open .cusus archive: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, zr.Close()) }()
	if len(zr.File) > maxArchiveEntries {
		return ExtractedArchive{}, fmt.Errorf("archive has %d entries; limit is %d", len(zr.File), maxArchiveEntries)
	}
	entries := make(map[string]*zip.File, len(zr.File))
	var totalBytes uint64
	for _, entry := range zr.File {
		name := filepath.ToSlash(entry.Name)
		if _, duplicate := entries[name]; duplicate {
			return ExtractedArchive{}, fmt.Errorf("duplicate archive entry %q", name)
		}
		entries[name] = entry
		totalBytes += entry.UncompressedSize64
		if totalBytes > maxArchiveBytes {
			return ExtractedArchive{}, fmt.Errorf("archive expands beyond %d bytes", int64(maxArchiveBytes))
		}
		if entry.UncompressedSize64 > 0 && entry.CompressedSize64 == 0 {
			return ExtractedArchive{}, fmt.Errorf("archive entry %q has an invalid compressed size", name)
		}
		if entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > maxExpansionRatio {
			return ExtractedArchive{}, fmt.Errorf("archive entry %q exceeds the expansion-ratio limit", name)
		}
	}
	var manifest Manifest
	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		return ExtractedArchive{}, fmt.Errorf("archive has no manifest.json")
	}
	if manifestEntry.UncompressedSize64 > maxManifestBytes {
		return ExtractedArchive{}, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	reader, err := manifestEntry.Open()
	if err != nil {
		return ExtractedArchive{}, err
	}
	err = decodeManifest(io.LimitReader(reader, maxManifestBytes+1), &manifest)
	err = errors.Join(err, reader.Close())
	if err != nil {
		return ExtractedArchive{}, fmt.Errorf("decode show manifest: %w", err)
	}
	if err := migrateManifest(&manifest); err != nil {
		return ExtractedArchive{}, err
	}
	if err := validateManifestSchema(manifest); err != nil {
		return ExtractedArchive{}, err
	}
	cache, err := currentCacheLayout()
	if err != nil {
		return ExtractedArchive{}, err
	}
	digest, err := HashFile(path)
	if err != nil {
		return ExtractedArchive{}, err
	}
	root := filepath.Join(cache.Shows, digest[:archiveHashPrefixLength])
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ExtractedArchive{}, err
	}
	temporary, err := os.MkdirTemp(parent, ".extract-*")
	if err != nil {
		return ExtractedArchive{}, fmt.Errorf("create extraction directory: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, os.RemoveAll(temporary))
	}()
	if err := os.MkdirAll(filepath.Join(temporary, "media"), 0o755); err != nil {
		return ExtractedArchive{}, err
	}
	assetByPath := make(map[string]Asset, len(manifest.Assets))
	assetIDs := make(map[string]struct{}, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		name := filepath.ToSlash(asset.Path)
		if err := validateAsset(asset, name, manifest.OriginalVersion == 1); err != nil {
			return ExtractedArchive{}, err
		}
		if _, duplicate := assetByPath[name]; duplicate {
			return ExtractedArchive{}, fmt.Errorf("duplicate manifest asset path %q", name)
		}
		if _, duplicate := assetIDs[asset.ID]; duplicate {
			return ExtractedArchive{}, fmt.Errorf("duplicate manifest asset ID %q", asset.ID)
		}
		assetByPath[name] = asset
		assetIDs[asset.ID] = struct{}{}
	}
	if err := validateShowAssetReferences(manifest.Show, assetByPath); err != nil {
		return ExtractedArchive{}, err
	}
	files := make([]File, 0, len(manifest.Assets))
	for name, asset := range assetByPath {
		entry := entries[name]
		if entry == nil {
			return ExtractedArchive{}, fmt.Errorf("manifest asset %q is missing from archive", name)
		}
		if entry.UncompressedSize64 != uint64(asset.Size) {
			return ExtractedArchive{}, fmt.Errorf("asset %q size is %d bytes; manifest declares %d", name, entry.UncompressedSize64, asset.Size)
		}
		target := filepath.Join(temporary, filepath.FromSlash(name))
		reader, err := entry.Open()
		if err != nil {
			return ExtractedArchive{}, err
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
		err = errors.Join(err, reader.Close())
		if err != nil {
			return ExtractedArchive{}, fmt.Errorf("extract %q: %w", asset.Name, err)
		}
		if copied != asset.Size {
			return ExtractedArchive{}, fmt.Errorf("asset %q extracted %d bytes; expected %d", name, copied, asset.Size)
		}
		contentHash := hex.EncodeToString(hash.Sum(nil))
		if asset.ContentSHA256 != "" && !strings.EqualFold(contentHash, asset.ContentSHA256) {
			return ExtractedArchive{}, fmt.Errorf("asset %q failed SHA-256 verification", name)
		}
		files = append(files, File{Name: asset.Name, Source: filepath.Join(root, filepath.FromSlash(name)), Hash: asset.SourceSHA256, Kind: asset.Kind})
	}
	for name := range entries {
		if strings.HasPrefix(name, "media/") {
			if _, declared := assetByPath[name]; !declared {
				return ExtractedArchive{}, fmt.Errorf("archive contains undeclared media entry %q", name)
			}
		}
	}
	if err := publishExtractedShow(temporary, root); err != nil {
		return ExtractedArchive{}, err
	}
	if err := touchCachePath(root); err != nil {
		return ExtractedArchive{}, fmt.Errorf("refresh extracted show cache: %w", err)
	}
	return ExtractedArchive{Manifest: manifest, Files: files, root: root}, nil
}
