package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
)

const mediaConversionTimeout = 6 * time.Hour

type mediaPreparer struct {
	ffmpegPath string
}

func newMediaPreparer(ffmpegPath string) mediaPreparer {
	return mediaPreparer{ffmpegPath: ffmpegPath}
}

func (preparer mediaPreparer) Prepare(source, kind, format, sourceHash string) (string, error) {
	if archiveAssetCanPassThrough(kind, source, format) {
		return source, nil
	}
	return preparer.transcode(source, kind, sourceHash)
}

func (preparer mediaPreparer) transcode(source, kind, sourceHash string) (string, error) {
	ext := transcodedAssetExtension(kind)
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
	attempts, err := mediaEncodingAttempts(kind, temporary)
	if err != nil {
		return "", err
	}
	var lastOutput []byte
	for _, args := range attempts {
		ctx, cancel := context.WithTimeout(context.Background(), mediaConversionTimeout)
		lastOutput, err = processgroup.CombinedOutput(processgroup.CommandContext(ctx, preparer.ffmpegPath, append(common, args...)...))
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

func mediaEncodingAttempts(kind, output string) ([][]string, error) {
	switch kind {
	case "audio":
		return [][]string{{"-vn", "-c:a", "libopus", "-b:a", "128k", "-vbr", "on", "-compression_level", "10", output}}, nil
	case "video":
		return [][]string{
			{"-c:v", "libsvtav1", "-preset", "8", "-crf", "32", "-pix_fmt", "yuv420p10le", "-c:a", "libopus", "-b:a", "128k", output},
			{"-c:v", "libvpx-vp9", "-crf", "31", "-b:v", "0", "-row-mt", "1", "-c:a", "libopus", "-b:a", "128k", output},
		}, nil
	case "image":
		return [][]string{{"-c:v", "libwebp", "-quality", "86", "-compression_level", "6", output}}, nil
	default:
		return nil, fmt.Errorf("unknown media kind %q", kind)
	}
}
