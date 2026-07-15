package project

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

func archiveAssetFormat(kind, source string) (extension, format string) {
	switch kind {
	case "audio":
		return ".opus", "opus"
	case "image":
		return ".webp", "webp"
	case "video":
		extension = strings.ToLower(filepath.Ext(source))
		if archiveAssetFormatAllowed(kind, strings.TrimPrefix(extension, ".")) {
			return extension, strings.TrimPrefix(extension, ".")
		}
		return ".webm", "webm"
	default:
		return "", ""
	}
}

func archiveAssetFormatAllowed(kind, format string) bool {
	switch kind {
	case "audio":
		return format == "opus"
	case "image":
		return format == "webp"
	case "video":
		switch format {
		case "mp4", "mov", "mkv", "webm", "avi":
			return true
		}
	}
	return false
}

func archiveAssetCanPassThrough(kind, source, format string) bool {
	extension := strings.ToLower(filepath.Ext(source))
	return archiveAssetFormatAllowed(kind, format) && format == strings.TrimPrefix(extension, ".")
}

func transcodedAssetExtension(kind string) string {
	extension, _ := archiveAssetFormat(kind, "")
	return extension
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
	if !archiveAssetFormatAllowed(asset.Kind, asset.Format) {
		return fmt.Errorf("asset %q has unsupported kind/format %q/%q", name, asset.Kind, asset.Format)
	}
	for label, value := range map[string]string{"source": asset.SourceSHA256, "content": asset.ContentSHA256} {
		if value == "" && label == "content" && allowMissingContentHash {
			continue
		}
		if len(value) != sha256.Size*2 || !isHexString(value) {
			return fmt.Errorf("asset %q has invalid %s SHA-256", name, label)
		}
	}
	return nil
}

func isHexString(value string) bool {
	for index := range len(value) {
		char := value[index]
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
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
