package config

import "testing"

func TestRemoteAssuranceSettingsNormalizeSafely(t *testing.T) {
	settings := Defaults()
	settings.RemoteSuccessPolicy = "unsafe"
	settings.RemoteTargets = []RemoteTarget{{Host: " console ", HealthPort: 70000, AckPort: -1}}
	normalized := normalizeRemote(settings.RemoteSettings)
	if normalized.RemoteSuccessPolicy != RemoteSuccessAll {
		t.Fatalf("success policy = %q", normalized.RemoteSuccessPolicy)
	}
	if target := normalized.RemoteTargets[0]; target.Host != "console" || target.HealthPort != 0 || target.AckPort != 0 {
		t.Fatalf("normalized target = %#v", target)
	}
}
