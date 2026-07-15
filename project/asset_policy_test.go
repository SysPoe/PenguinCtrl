package project

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestArchiveAssetPolicyFormatsAndPassthrough(t *testing.T) {
	tests := []struct {
		name            string
		kind            string
		source          string
		wantExtension   string
		wantFormat      string
		wantPassThrough bool
	}{
		{name: "audio encode", kind: "audio", source: "track.wav", wantExtension: ".opus", wantFormat: "opus"},
		{name: "audio passthrough", kind: "audio", source: "track.OPUS", wantExtension: ".opus", wantFormat: "opus", wantPassThrough: true},
		{name: "image encode", kind: "image", source: "slide.png", wantExtension: ".webp", wantFormat: "webp"},
		{name: "image passthrough", kind: "image", source: "slide.WEBP", wantExtension: ".webp", wantFormat: "webp", wantPassThrough: true},
		{name: "mp4 passthrough", kind: "video", source: "clip.mp4", wantExtension: ".mp4", wantFormat: "mp4", wantPassThrough: true},
		{name: "mov passthrough", kind: "video", source: "clip.MOV", wantExtension: ".mov", wantFormat: "mov", wantPassThrough: true},
		{name: "mkv passthrough", kind: "video", source: "clip.mkv", wantExtension: ".mkv", wantFormat: "mkv", wantPassThrough: true},
		{name: "webm passthrough", kind: "video", source: "clip.webm", wantExtension: ".webm", wantFormat: "webm", wantPassThrough: true},
		{name: "avi passthrough", kind: "video", source: "clip.avi", wantExtension: ".avi", wantFormat: "avi", wantPassThrough: true},
		{name: "video encode", kind: "video", source: "clip.mpeg", wantExtension: ".webm", wantFormat: "webm"},
		{name: "unknown", kind: "document", source: "notes.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extension, format := archiveAssetFormat(test.kind, test.source)
			if extension != test.wantExtension || format != test.wantFormat {
				t.Fatalf("archiveAssetFormat(%q, %q) = %q, %q; want %q, %q", test.kind, test.source, extension, format, test.wantExtension, test.wantFormat)
			}
			if got := archiveAssetCanPassThrough(test.kind, test.source, format); got != test.wantPassThrough {
				t.Fatalf("archiveAssetCanPassThrough(%q, %q, %q) = %t; want %t", test.kind, test.source, format, got, test.wantPassThrough)
			}
			if format != "" && !archiveAssetFormatAllowed(test.kind, format) {
				t.Fatalf("selected archive format %q is not allowed for %q", format, test.kind)
			}
		})
	}
}

func TestValidateAssetUsesSharedFormatAndHexPolicy(t *testing.T) {
	valid := Asset{
		ID:            "asset-1",
		Name:          "Track",
		Kind:          "audio",
		Path:          "media/track.opus",
		SourceSHA256:  strings.Repeat("aB", 32),
		ContentSHA256: strings.Repeat("01", 32),
		Format:        "opus",
		Size:          128,
	}
	if err := validateAsset(valid, valid.Path, false); err != nil {
		t.Fatalf("validate valid asset: %v", err)
	}

	unsupported := valid
	unsupported.Format = "webp"
	if err := validateAsset(unsupported, unsupported.Path, false); err == nil || !strings.Contains(err.Error(), "unsupported kind/format") {
		t.Fatalf("validate unsupported format = %v; want unsupported kind/format error", err)
	}

	badHash := valid
	badHash.SourceSHA256 = strings.Repeat("0", 63) + "g"
	if err := validateAsset(badHash, badHash.Path, false); err == nil || !strings.Contains(err.Error(), "invalid source SHA-256") {
		t.Fatalf("validate non-hex hash = %v; want invalid source SHA-256 error", err)
	}

	legacy := valid
	legacy.ContentSHA256 = ""
	if err := validateAsset(legacy, legacy.Path, true); err != nil {
		t.Fatalf("validate legacy asset without content hash: %v", err)
	}
	if err := validateAsset(legacy, legacy.Path, false); err == nil || !strings.Contains(err.Error(), "invalid content SHA-256") {
		t.Fatalf("validate current asset without content hash = %v; want invalid content SHA-256 error", err)
	}
}

func TestMediaEncodingAttemptsRemainStable(t *testing.T) {
	if mediaConversionTimeout != 6*time.Hour {
		t.Fatalf("mediaConversionTimeout = %s; want 6h", mediaConversionTimeout)
	}

	output := "prepared-media"
	tests := []struct {
		kind string
		want [][]string
	}{
		{kind: "audio", want: [][]string{{"-vn", "-c:a", "libopus", "-b:a", "128k", "-vbr", "on", "-compression_level", "10", output}}},
		{kind: "video", want: [][]string{
			{"-c:v", "libsvtav1", "-preset", "8", "-crf", "32", "-pix_fmt", "yuv420p10le", "-c:a", "libopus", "-b:a", "128k", output},
			{"-c:v", "libvpx-vp9", "-crf", "31", "-b:v", "0", "-row-mt", "1", "-c:a", "libopus", "-b:a", "128k", output},
		}},
		{kind: "image", want: [][]string{{"-c:v", "libwebp", "-quality", "86", "-compression_level", "6", output}}},
	}

	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			got, err := mediaEncodingAttempts(test.kind, output)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("mediaEncodingAttempts(%q) = %#v; want %#v", test.kind, got, test.want)
			}
		})
	}

	if _, err := mediaEncodingAttempts("document", output); err == nil || !strings.Contains(err.Error(), "unknown media kind") {
		t.Fatalf("mediaEncodingAttempts(unknown) = %v; want unknown media kind error", err)
	}
}

func TestMediaPreparerPassesThroughSupportedArchiveFormats(t *testing.T) {
	preparer := newMediaPreparer("missing-ffmpeg")
	for _, test := range []struct {
		kind   string
		format string
		source string
	}{
		{kind: "audio", format: "opus", source: "missing.OPUS"},
		{kind: "image", format: "webp", source: "missing.WEBP"},
		{kind: "video", format: "mp4", source: "missing.MP4"},
	} {
		got, err := preparer.Prepare(test.source, test.kind, test.format, "unused-hash")
		if err != nil || got != test.source {
			t.Fatalf("Prepare(%q, %q, %q) = %q, %v; want passthrough %q", test.source, test.kind, test.format, got, err, test.source)
		}
	}
}
