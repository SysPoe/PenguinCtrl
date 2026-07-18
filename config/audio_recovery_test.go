package config

import "testing"

func TestAudioRecoveryPoliciesNormalizeAndResolvePerRoute(t *testing.T) {
	settings := Settings{
		AudioSettings: normalizeAudio(AudioSettings{
			PlaybackAudioDevice: " primary ", PlaybackAudioRecovery: AudioRecoveryNamedBackup, PlaybackBackupAudioDevice: " backup ",
			PreviewAudioDevice: " preview ", PreviewAudioRecovery: "invalid", PreviewBackupAudioDevice: " ignored ",
		}),
	}
	device, policy, backup := AudioRoute(settings, false)
	if device != "primary" || policy != AudioRecoveryNamedBackup || backup != "backup" {
		t.Fatalf("playback route = %q/%q/%q", device, policy, backup)
	}
	device, policy, backup = AudioRoute(settings, true)
	if device != "preview" || policy != AudioRecoveryFailClosed || backup != "ignored" {
		t.Fatalf("preview route = %q/%q/%q", device, policy, backup)
	}
}
