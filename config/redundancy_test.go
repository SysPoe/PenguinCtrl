package config

import "testing"

func TestNormalizeRedundancyDefaultsAndExplicitRole(t *testing.T) {
	defaults := normalize(Settings{})
	if defaults.RedundancyRole != RedundancyOff || defaults.RedundancyNodeID == "" || defaults.RedundancyListenAddress == "" || defaults.RedundancyPeerAddress == "" {
		t.Fatalf("redundancy defaults = %+v", defaults)
	}
	configured := normalize(Settings{
		RedundancyRole: RedundancyStandby, RedundancyNodeID: " spare ",
		RedundancyListenAddress: " 127.0.0.1:9020 ", RedundancyPeerAddress: " 127.0.0.1:9021 ",
		RedundancySharedKey: " secret ", RedundancyInterlockPath: " shared/command.lock ",
	})
	if configured.RedundancyRole != RedundancyStandby || configured.RedundancyNodeID != "spare" || configured.RedundancyListenAddress != "127.0.0.1:9020" || configured.RedundancySharedKey != "secret" {
		t.Fatalf("redundancy configuration = %+v", configured)
	}
}
