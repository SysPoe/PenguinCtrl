package media

import (
	"encoding/base64"
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

func TestValidateSourceAcceptsWebPImage(t *testing.T) {
	// A 2x2 WebP image, matching the format used for images bundled in .cusus files.
	data, err := base64.StdEncoding.DecodeString("UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoCAAIAAgA0JaACdLoB+AADsAD+8Oj3/yC5YXXI1/8gP+QH/ID/+PIAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "image.webp")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSource("ffmpeg", path, show.CueTypeImage); err != nil {
		t.Fatalf("ValidateSource() error = %v", err)
	}
}
