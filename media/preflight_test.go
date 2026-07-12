package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestValidateSourceRejectsUnsupportedImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-an-image.png")
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := ValidateSource("ffmpeg", path, show.CueTypeImage)
	if err == nil || !strings.Contains(err.Error(), "unsupported image") {
		t.Fatalf("error = %v", err)
	}
}
