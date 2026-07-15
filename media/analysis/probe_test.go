package analysis

import (
	"strings"
	"testing"
)

func TestParseFrameRate(t *testing.T) {
	for input, want := range map[string]float64{"30000/1001": 29.97002997002997, "25/1": 25, "60": 60, "0/0": 0} {
		if got := parseFrameRate(input); got != want {
			t.Errorf("parseFrameRate(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseInfoUsesAutorotatedFrameDimensions(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","width":1080,"height":1920,"avg_frame_rate":"30000/1001","side_data_list":[{"side_data_type":"Mastering display metadata"},{"rotation":-90}]},{"codec_type":"audio"}]}`
	info, err := parseInfo([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasVideo || !info.HasAudio {
		t.Fatalf("streams not detected: %#v", info)
	}
	if info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("decoded dimensions = %dx%d, want 1920x1080", info.Width, info.Height)
	}
	if info.FPS < 29.9 || info.FPS > 30 {
		t.Fatalf("fps = %v, want approximately 29.97", info.FPS)
	}
}

func TestParseInfoUsesLegacyRotateTag(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","width":1920,"height":1080,"avg_frame_rate":"25/1","tags":{"rotate":"90"}}]}`
	info, err := parseInfo([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 1080 || info.Height != 1920 {
		t.Fatalf("decoded dimensions = %dx%d, want 1080x1920", info.Width, info.Height)
	}
}

func TestParseInfoRejectsInvalidJSON(t *testing.T) {
	_, err := parseInfo([]byte(`{"streams":`))
	if err == nil || !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("error = %v, want JSON parse failure", err)
	}
}

func TestParseInfoRejectsUnsafeDecodeResources(t *testing.T) {
	for _, raw := range []string{
		`{"streams":[{"codec_type":"video","width":9000,"height":1080,"avg_frame_rate":"30/1"}]}`,
		`{"streams":[{"codec_type":"video","width":7680,"height":4320,"avg_frame_rate":"300/1"}]}`,
		`{"streams":[{"codec_type":"video","width":1920,"height":1080,"avg_frame_rate":"30/1","bit_rate":"500000001"}]}`,
	} {
		if _, err := parseInfo([]byte(raw)); err == nil {
			t.Fatalf("parseInfo(%s) succeeded, want resource-limit error", raw)
		}
	}
}
