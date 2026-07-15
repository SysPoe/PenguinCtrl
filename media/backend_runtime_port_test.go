package media

import (
	"errors"
	"testing"
	"time"

	"github.com/syspoe/cusus/playback"
)

type runtimeBackendProbe struct {
	prewarmed []PlaybackRequest
	closed    bool
}

func (*runtimeBackendProbe) Open(PlaybackRequest) (PlaybackSession, error) { return nil, nil }

func (probe *runtimeBackendProbe) Prewarm(requests []PlaybackRequest) {
	probe.prewarmed = append([]PlaybackRequest(nil), requests...)
}

func (probe *runtimeBackendProbe) Close() { probe.closed = true }

func TestMediaRuntimeUsesSharedBackendLifecyclePort(t *testing.T) {
	probe := new(runtimeBackendProbe)
	runtime := &mediaRuntime{decoder: probe}
	runtime.prewarm([]playback.PreloadSpec{{Source: "cue.wav", ClipStartMs: 125}})
	if len(probe.prewarmed) != 1 || probe.prewarmed[0].Instance.Source != "cue.wav" || probe.prewarmed[0].Position.Milliseconds() != 125 {
		t.Fatalf("prewarm requests = %#v", probe.prewarmed)
	}
	runtime.close()
	if !probe.closed {
		t.Fatal("runtime lifecycle did not close decoder backend")
	}
}

type audioRecoveryProbe struct {
	target  string
	request audioRecoveryRequest
	err     error
}

func (probe *audioRecoveryProbe) recover(target string, request audioRecoveryRequest) (*recoveredAudioEndpoint, error) {
	probe.target, probe.request = target, request
	return nil, probe.err
}

func TestSessionDelegatesAudioEndpointRespawn(t *testing.T) {
	wantErr := errors.New("probe recovery")
	probe := &audioRecoveryProbe{err: wantErr}
	clock := NewPlaybackClock(275 * time.Millisecond)
	session := &ffmpegSession{
		backend: &FFmpegBackend{recovery: probe},
		request: PlaybackRequest{Instance: playback.Instance{ClipEndMs: 9_000, Preview: true}},
		path:    "cue.wav",
		ctx:     t.Context(),
		clock:   clock,
		audio:   new(devicePlayer),
		volume:  -6,
	}
	if err := session.recoverAudio("backup"); !errors.Is(err, wantErr) {
		t.Fatalf("recoverAudio error = %v, want %v", err, wantErr)
	}
	if probe.target != "backup" || probe.request.path != "cue.wav" || probe.request.position != clock.Position() || probe.request.clipEndMs != 9_000 || !probe.request.preview {
		t.Fatalf("recovery request = %#v, target = %q", probe.request, probe.target)
	}
	if probe.request.volumeDB != dbVolume(-6, false) || probe.request.onRecovery == nil {
		t.Fatalf("recovery volume/callback = %v, present=%t", probe.request.volumeDB, probe.request.onRecovery != nil)
	}
}

func TestAudioEndpointRecoveryRejectsUnavailableRoute(t *testing.T) {
	recovery := newFFmpegAudioEndpointRecovery(nil, nil)
	if _, err := recovery.recover("backup", audioRecoveryRequest{ctx: t.Context()}); err == nil {
		t.Fatal("unavailable audio route accepted recovery")
	}
}
