package project

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/show"
)

const (
	Format  = "cusus-show"
	Version = 1
)

type Asset struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	SourceSHA256 string `json:"sourceSha256"`
	Format       string `json:"format"`
	Size         int64  `json:"size"`
}

type Manifest struct {
	Format  string    `json:"format"`
	Version int       `json:"version"`
	Show    show.Show `json:"show"`
	Assets  []Asset   `json:"assets"`
}

// Save writes a portable .cusus ZIP archive. Audio is normalized to Opus,
// video to AV1/Opus WebM (with VP9 fallback), and images to WebP. Identical
// source content of the same media kind is transcoded and stored only once.
func Save(dst io.Writer, current show.Show, ffmpegPath string) (Manifest, error) {
	manifest := Manifest{Format: Format, Version: Version, Show: current}
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
			ext := map[string]string{"audio": ".opus", "video": ".webm", "image": ".webp"}[kind]
			id := hash[:24] + "-" + kind
			pending = pendingAsset{asset: Asset{
				ID: id, Name: filepath.Base(path), Kind: kind,
				Path: "media/" + id + ext, SourceSHA256: hash, Format: strings.TrimPrefix(ext, "."),
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
	for _, key := range keys {
		pending := assets[key]
		converted, err := transcode(ffmpegPath, pending.source, pending.asset.Kind, pending.asset.SourceSHA256)
		if err != nil {
			return Manifest{}, fmt.Errorf("prepare %s %q: %w", pending.asset.Kind, pending.asset.Name, err)
		}
		info, err := os.Stat(converted)
		if err != nil {
			return Manifest{}, err
		}
		pending.asset.Size = info.Size()
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

// Load reads and extracts a .cusus file. Returned cue paths point at a stable
// per-archive cache directory, ready for the playback engine.
func Load(path string) (Manifest, []File, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open .cusus archive: %w", err)
	}
	defer zr.Close()
	var manifest Manifest
	for _, entry := range zr.File {
		if entry.Name != "manifest.json" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return Manifest{}, nil, err
		}
		err = json.NewDecoder(reader).Decode(&manifest)
		reader.Close()
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("decode show manifest: %w", err)
		}
		break
	}
	if manifest.Format != Format || manifest.Version != Version {
		return Manifest{}, nil, fmt.Errorf("unsupported .cusus format %q version %d", manifest.Format, manifest.Version)
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
	if err := os.MkdirAll(filepath.Join(root, "media"), 0o755); err != nil {
		return Manifest{}, nil, err
	}
	assetByPath := make(map[string]Asset, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		assetByPath[filepath.ToSlash(asset.Path)] = asset
	}
	files := make([]File, 0, len(manifest.Assets))
	for _, entry := range zr.File {
		asset, ok := assetByPath[filepath.ToSlash(entry.Name)]
		if !ok {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(asset.Path))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(root)+string(os.PathSeparator)) {
			return Manifest{}, nil, fmt.Errorf("unsafe media path %q", asset.Path)
		}
		reader, err := entry.Open()
		if err != nil {
			return Manifest{}, nil, err
		}
		out, err := os.Create(target)
		if err == nil {
			_, err = io.Copy(out, reader)
			out.Close()
		}
		reader.Close()
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("extract %q: %w", asset.Name, err)
		}
		files = append(files, File{Name: asset.Name, Source: target, Hash: asset.SourceSHA256, Kind: asset.Kind})
	}
	resolveLoadedPaths(&manifest.Show, root)
	return manifest, files, nil
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
		lastOutput, err = exec.Command(ffmpegPath, append(common, args...)...).CombinedOutput()
		if err == nil {
			if err := os.Rename(temporary, output); err != nil {
				_ = os.Remove(temporary)
				return "", err
			}
			return output, nil
		}
		_ = os.Remove(temporary)
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
