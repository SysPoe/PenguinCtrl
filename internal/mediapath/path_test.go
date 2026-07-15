package mediapath

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalConvertsFileURIAndRejectsOtherSchemes(t *testing.T) {
	path, err := Local("file:///tmp/My%20Show/clip.wav")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.FromSlash("/tmp/My Show/clip.wav")
	if runtime.GOOS == "windows" {
		want = filepath.FromSlash("/tmp/My Show/clip.wav")
	}
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := Local("https://example.invalid/clip.wav"); err == nil {
		t.Fatal("remote URI was accepted as a local media path")
	}
}
