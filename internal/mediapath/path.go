// Package mediapath owns conversion of native-picker file references into
// local operating-system paths.
package mediapath

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// Local converts a file URI or plain local path into OS path syntax.
func Local(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("media path is empty")
	}
	parsed, err := url.Parse(source)
	if err == nil && strings.EqualFold(parsed.Scheme, "file") {
		path, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return "", fmt.Errorf("decode media URI: %w", err)
		}
		if parsed.Host != "" {
			path = "//" + parsed.Host + path
		}
		if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		return filepath.FromSlash(path), nil
	}
	if err == nil && parsed.Scheme != "" && len(parsed.Scheme) != 1 {
		return "", fmt.Errorf("unsupported media URI %q", source)
	}
	return filepath.Clean(source), nil
}
