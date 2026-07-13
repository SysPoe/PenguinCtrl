package media

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
)

func TestPCMRingWrapsWithoutLosingOrder(t *testing.T) {
	ring := newPCMRing(8)
	if n := ring.write([]byte{1, 2, 3, 4, 5, 6}); n != 6 {
		t.Fatalf("first write = %d", n)
	}
	first := make([]byte, 4)
	if n := ring.read(first); n != 4 || !bytes.Equal(first, []byte{1, 2, 3, 4}) {
		t.Fatalf("first read = %d %v", n, first)
	}
	if n := ring.write([]byte{7, 8, 9, 10, 11, 12}); n != 6 {
		t.Fatalf("wrapped write = %d", n)
	}
	remaining := make([]byte, 8)
	if n := ring.read(remaining); n != 8 || !bytes.Equal(remaining, []byte{5, 6, 7, 8, 9, 10, 11, 12}) {
		t.Fatalf("wrapped read = %d %v", n, remaining)
	}
}

func TestAudioGainBoostsAndSaturatesWithoutWrapping(t *testing.T) {
	player := &devicePlayer{ring: newPCMRing(16)}
	player.SetVolume(dbVolume(12, false))
	input := make([]byte, 4)
	binary.LittleEndian.PutUint16(input[0:], uint16(int16(1000)))
	binary.LittleEndian.PutUint16(input[2:], uint16(int16(20000)))
	player.ring.write(input)
	output := make([]byte, 4)
	player.readSamples(output, nil, 1)
	if got := int16(binary.LittleEndian.Uint16(output[0:])); got <= 1000 {
		t.Fatalf("boosted sample = %d, want > 1000", got)
	}
	if got := int16(binary.LittleEndian.Uint16(output[2:])); got != 32767 {
		t.Fatalf("saturated sample = %d, want 32767", got)
	}
}

func TestAudioCallbackNeverWaitsForDecoder(t *testing.T) {
	player := &devicePlayer{ring: newPCMRing(1024)}
	player.volume.Store(float64Bits(1))
	output := make([]byte, 512)
	done := make(chan struct{})
	go func() {
		player.readSamples(output, nil, 128)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("audio callback blocked waiting for decoder data")
	}
	if player.Underruns() != 1 {
		t.Fatalf("underruns = %d, want 1", player.Underruns())
	}
}

func TestEndpointMixerCombinesSourcesWithoutAllocation(t *testing.T) {
	first := &devicePlayer{ring: newPCMRing(16)}
	second := &devicePlayer{ring: newPCMRing(16)}
	first.volume.Store(float64Bits(1))
	second.volume.Store(float64Bits(1))
	for player, sample := range map[*devicePlayer]int16{first: 1000, second: 2000} {
		raw := make([]byte, 4)
		binary.LittleEndian.PutUint16(raw[0:], uint16(sample))
		binary.LittleEndian.PutUint16(raw[2:], uint16(sample))
		player.ring.write(raw)
		player.eof.Store(true)
	}
	mixer := &endpointMixer{}
	mixer.sources.Store([]*devicePlayer{first, second})
	output := make([]byte, 4)
	mixer.mix(output, nil, 1)
	if left, right := int16(binary.LittleEndian.Uint16(output)), int16(binary.LittleEndian.Uint16(output[2:])); left != 3000 || right != 3000 {
		t.Fatalf("mixed samples = %d/%d, want 3000/3000", left, right)
	}

	first.ring.write(make([]byte, 4))
	second.ring.write(make([]byte, 4))
	if allocations := testing.AllocsPerRun(100, func() { mixer.mix(output, nil, 1) }); allocations != 0 {
		t.Fatalf("mixer callback allocated %v times", allocations)
	}
}

func TestFallbackDevicePolicy(t *testing.T) {
	if _, ok := fallbackDeviceID(config.AudioRecoveryFailClosed, "backup"); ok {
		t.Fatal("fail-closed policy selected a fallback")
	}
	if id, ok := fallbackDeviceID(config.AudioRecoveryFollowDefault, "backup"); !ok || id != "" {
		t.Fatalf("follow-default fallback = %q, %v", id, ok)
	}
	if id, ok := fallbackDeviceID(config.AudioRecoveryNamedBackup, " backup "); !ok || id != "backup" {
		t.Fatalf("named fallback = %q, %v", id, ok)
	}
	if _, ok := fallbackDeviceID(config.AudioRecoveryNamedBackup, " "); ok {
		t.Fatal("empty named fallback was accepted")
	}
}

func TestPlayerRecoveryDelegatesTargetEndpoint(t *testing.T) {
	player := &devicePlayer{}
	called := ""
	player.SetRecoveryHandler(func(deviceID string) error {
		called = deviceID
		return nil
	})
	if !player.recoverTo("backup-device") || called != "backup-device" {
		t.Fatalf("recovery target = %q", called)
	}
}
