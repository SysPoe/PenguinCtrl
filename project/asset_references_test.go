package project

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestShowAssetReferenceTraversalOwnsAllMediaKinds(t *testing.T) {
	current := show.Show{Cues: []show.Cue{
		{CueNumber: "1", Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: "sound.wav"}}}},
		{CueNumber: "2", Type: show.CueTypeVideo, Play: show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: "video.mov"}}}},
		{CueNumber: "3", Type: show.CueTypeImage, Play: show.CuePlay{Image: &show.ImagePlay{File: "image.png"}}},
		{CueNumber: "4", Type: show.CueTypeWait, Play: show.CuePlay{Wait: &show.WaitPlay{}}},
		{CueNumber: "5", Type: show.CueTypeSound},
	}}
	var got []string
	err := visitShowAssetReferences(&current, func(reference showAssetReference) error {
		got = append(got, reference.CueNumber+":"+reference.Kind+":"+reference.Path())
		reference.SetPath("media/" + reference.Kind)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1:audio:sound.wav", "2:video:video.mov", "3:image:image.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("asset references = %v; want %v", got, want)
	}
	if current.Cues[0].Play.Sound.File != "media/audio" || current.Cues[1].Play.Video.File != "media/video" || current.Cues[2].Play.Image.File != "media/image" {
		t.Fatalf("reference mutations did not reach cue payloads: %#v", current.Cues)
	}
}

func TestPortableAssetReferencesHydrateAndPublishRoundTrip(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		name    string
		content string
	}{
		{name: "sound.opus", content: "audio"},
		{name: "video.mp4", content: "video"},
		{name: "image.webp", content: "image"},
	}
	for _, file := range files {
		path := filepath.Join(root, "media", file.name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	portable := show.Show{Cues: []show.Cue{
		{CueNumber: "1", Type: show.CueTypeSound, Play: show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: "media/sound.opus"}}}},
		{CueNumber: "2", Type: show.CueTypeVideo, Play: show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: "media/video.mp4"}}}},
		{CueNumber: "3", Type: show.CueTypeImage, Play: show.CuePlay{Image: &show.ImagePlay{File: "media/image.webp"}}},
	}}
	runtimeShow := show.CloneShow(portable)
	resolveLoadedPaths(&runtimeShow, root)
	for index, path := range []string{
		runtimeShow.Cues[0].Play.Sound.File,
		runtimeShow.Cues[1].Play.Video.File,
		runtimeShow.Cues[2].Play.Image.File,
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("runtime cue %d path = %q; want absolute", index, path)
		}
	}

	var archive bytes.Buffer
	manifest, err := Save(&archive, runtimeShow, filepath.Join(t.TempDir(), "missing-ffmpeg"))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{
		manifest.Show.Cues[0].Play.Sound.File,
		manifest.Show.Cues[1].Play.Video.File,
		manifest.Show.Cues[2].Play.Image.File,
	}
	want := []string{"media/sound.opus", "media/video.mp4", "media/image.webp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("republished portable paths = %v; want %v", got, want)
	}
	for index, path := range []string{
		runtimeShow.Cues[0].Play.Sound.File,
		runtimeShow.Cues[1].Play.Video.File,
		runtimeShow.Cues[2].Play.Image.File,
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("save mutated runtime cue %d path to %q", index, path)
		}
	}
}
