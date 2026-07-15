package project

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): archive_publish.go owns FFmpeg re-encode policy, asset validation,
// and progress reporting while archive.go/archive_reader.go own zip I/O — but
// validation rules and format allowlists are duplicated/split across them. Make
// publish a dedicated pipeline (resolve sources → encode → zip) with a single
// asset-policy module so load and save cannot diverge on allowed formats/limits.
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
		// TODO(micro): replace ContainsRune hex scan with a small isHexString helper (and share with crashreport safeName style)
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

// TODO(macro): Move transcode/encode policy out of archive packaging — prepare
// and transcode own long-running ffmpeg, disk cache under CuSus/transcoded, and
// codec choices, which is media-pipeline work nested inside project I/O.
func transcode(ffmpegPath, source, kind, sourceHash string) (string, error) {
	ext := map[string]string{"audio": ".opus", "video": ".webm", "image": ".webp"}[kind]
	cache, err := currentCacheLayout()
	if err != nil {
		return "", err
	}
	cacheDir := cache.Transcoded
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	output := filepath.Join(cacheDir, sourceHash+"-"+kind+ext)
	if info, err := os.Stat(output); err == nil && info.Size() > 0 {
		if err := touchCachePath(output); err != nil {
			return "", fmt.Errorf("refresh transcoded cache: %w", err)
		}
		return output, nil
	}
	tmp, err := os.CreateTemp(cacheDir, "cusus-media-*"+ext)
	if err != nil {
		return "", err
	}
	temporary := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temporary transcode output: %w", err)
	}
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
	// TODO(micro): name ffmpeg conversion timeout (6h) as a constant
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
			if err := touchCachePath(output); err != nil {
				return "", fmt.Errorf("refresh transcoded cache: %w", err)
			}
			return output, nil
		}
		_ = os.Remove(temporary)
		if timedOut {
			return "", fmt.Errorf("ffmpeg conversion timed out after 6h")
		}
	}
	return "", fmt.Errorf("ffmpeg conversion failed: %v: %s", err, strings.TrimSpace(string(lastOutput)))
}

// TODO(micro): replace hand-rolled insertion sort with sort.Strings
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
