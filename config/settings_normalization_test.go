package config

import (
	"reflect"
	"testing"
)

func TestNormalizeMediaDefaults(t *testing.T) {
	settings := normalizeMedia(MediaSettings{})
	if settings.FFmpegPath != "ffmpeg" || settings.DefaultMediaOutput != "main" {
		t.Fatalf("media defaults = %+v", settings)
	}
	if settings.Variables == nil {
		t.Fatal("media variables map was not initialized")
	}
}

func TestNormalizeCacheClampsLimits(t *testing.T) {
	settings := normalizeCache(CacheSettings{CacheQuotaGB: maximumCacheQuotaGB + 1, CacheReserveGB: 0})
	if settings.CacheQuotaGB != maximumCacheQuotaGB || settings.CacheReserveGB != minimumCacheReserveGB {
		t.Fatalf("cache limits = %+v", settings)
	}
}

func TestNormalizePassesNormalizedMediaDefaultToOutputs(t *testing.T) {
	settings := normalize(Settings{})
	if settings.DefaultMediaOutput != "main" {
		t.Fatalf("default media output = %q, want main", settings.DefaultMediaOutput)
	}
	if len(settings.VideoOutputs) != 1 || settings.VideoOutputs[0].Stage != settings.DefaultMediaOutput {
		t.Fatalf("video outputs = %+v, want normalized default stage", settings.VideoOutputs)
	}
}

func TestNormalizeWiresEveryDomainNormalizer(t *testing.T) {
	input := Settings{
		MediaSettings:      MediaSettings{DefaultMediaOutput: "projection"},
		AudioSettings:      AudioSettings{PlaybackAudioDevice: " primary ", PlaybackAudioRecovery: "invalid"},
		OutputSettings:     OutputSettings{VideoOutputs: []VideoOutput{{Stage: " projection ", Scaling: "invalid"}}},
		RemoteSettings:     RemoteSettings{RemoteTargets: []RemoteTarget{{Host: " console ", OSCPort: -1}}, RemoteSuccessPolicy: "invalid"},
		CacheSettings:      CacheSettings{CacheQuotaGB: maximumCacheQuotaGB + 1, CacheReserveGB: 0},
		OperatorUISettings: OperatorUISettings{OperatorWindow: WindowPlacement{Width: 100, Height: 100}},
		TimecodeSettings:   TimecodeSettings{TimecodeSource: "invalid", TimecodeFrameRate: 12},
		RedundancySettings: RedundancySettings{RedundancyRole: "invalid", RedundancyNodeID: " node "},
	}
	got := normalize(clone(input))
	media := normalizeMedia(input.MediaSettings)
	want := Settings{
		MediaSettings:      media,
		AudioSettings:      normalizeAudio(input.AudioSettings),
		OutputSettings:     normalizeOutputs(input.OutputSettings, media.DefaultMediaOutput),
		RemoteSettings:     normalizeRemote(input.RemoteSettings),
		CacheSettings:      normalizeCache(input.CacheSettings),
		OperatorUISettings: normalizeOperatorUI(input.OperatorUISettings),
		TimecodeSettings:   normalizeTimecode(input.TimecodeSettings),
		RedundancySettings: normalizeRedundancy(input.RedundancySettings),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregate normalization =\n%+v\nwant domain composition\n%+v", got, want)
	}
}
