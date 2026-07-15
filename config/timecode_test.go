package config

import "testing"

func TestNormalizeTimecodeDefaultsAndSupportedValues(t *testing.T) {
	defaults := normalize(Settings{})
	if defaults.TimecodeSource != TimecodeInternal || defaults.TimecodePolicy != TimecodeHold || defaults.TimecodeFrameRate != 30 || defaults.TimecodeListenAddress == "" {
		t.Fatalf("timecode defaults = %+v", defaults)
	}
	configured := normalize(Settings{TimecodeSettings: TimecodeSettings{TimecodeSource: TimecodeMTC, TimecodePolicy: TimecodeChase, TimecodeFrameRate: 25, TimecodeListenAddress: "127.0.0.1:9010"}})
	if configured.TimecodeSource != TimecodeMTC || configured.TimecodePolicy != TimecodeChase || configured.TimecodeFrameRate != 25 {
		t.Fatalf("timecode configuration = %+v", configured)
	}
}
