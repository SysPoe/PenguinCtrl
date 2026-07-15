package media

import "testing"

func TestAudioSystemMetricsDelegateToMixerRegistry(t *testing.T) {
	registry := newAudioMixerRegistry(nil)
	mixer := &endpointMixer{deviceID: "stage"}
	mixer.sources.Store([]*devicePlayer{{}})
	registry.mixers["stage"] = mixer
	system := &AudioSystem{mixers: registry}

	metrics := system.Metrics()
	if len(metrics) != 1 || metrics[0].EndpointID != "stage" || metrics[0].ActiveSources != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestClosedAudioSourceNeedsNoFailover(t *testing.T) {
	registry := newAudioMixerRegistry(nil)
	player := &devicePlayer{registry: registry, done: make(chan struct{})}
	registry.routes[player] = audioSourceRoute{}
	if err := player.Close(); err != nil {
		t.Fatal(err)
	}
	if !registry.failoverSource(player, "failed-device") {
		t.Fatal("closed source was treated as a failed failover")
	}
}
