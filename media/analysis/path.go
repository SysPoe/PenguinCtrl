package analysis

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveSource returns an absolute filesystem path for a media source. Both
// native paths and file URLs are accepted because archived shows may contain
// either representation.
func ResolveSource(source string) (string, error) {
	if strings.HasPrefix(source, "file:") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		source = parsed.Path
		if runtime.GOOS == "windows" && len(source) >= 3 && source[0] == '/' && source[2] == ':' {
			source = source[1:]
		}
	}
	source = filepath.FromSlash(source)
	if !filepath.IsAbs(source) {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		source = absolute
	}
	return source, nil
}

func ffprobeExecutable(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}
