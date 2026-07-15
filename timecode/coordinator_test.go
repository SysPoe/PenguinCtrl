package timecode

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestHoldPolicyLatchesDiscontinuityUntilAcknowledged(t *testing.T) {
	now := time.Unix(100, 0)
	coordinator := newCoordinator(Config{Source: SourceOSC, Policy: PolicyHold, FrameRate: 30, JumpTolerance: 100 * time.Millisecond}, func() time.Time { return now })
	var reported time.Duration
	coordinator.SetOnDiscontinuity(func(jump time.Duration) { reported = jump })
	if err := coordinator.Update(SourceOSC, 0, true); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := coordinator.Update(SourceOSC, 10*time.Second, true); err != nil {
		t.Fatal(err)
	}
	status := coordinator.Status()
	if !status.Held || status.State != StateDiscontinuity || reported < 8*time.Second {
		t.Fatalf("held status = %+v, jump = %v", status, reported)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if coordinator.WaitUntil(ctx, 2*time.Second) {
		t.Fatal("held timeline released a marker")
	}
	coordinator.Acknowledge(true)
	if got := coordinator.Position(); got != 10*time.Second {
		t.Fatalf("resynced position = %v", got)
	}
}

func TestLTCAndMTCAdaptersProduceTimelinePosition(t *testing.T) {
	ltc := New(Config{Source: SourceLTC, Policy: PolicyResync})
	if err := NewLTCAdapter(ltc).IngestFrame(1, 2, 3, 15, 30); err != nil {
		t.Fatal(err)
	}
	if got := ltc.Position(); got < time.Hour+2*time.Minute+3500*time.Millisecond || got > time.Hour+2*time.Minute+3510*time.Millisecond {
		t.Fatalf("LTC position = %v", got)
	}

	mtc := New(Config{Source: SourceMTC, Policy: PolicyResync})
	adapter := NewMTCAdapter(mtc)
	values := []byte{4, 0, 3, 0, 2, 0, 1, 6}
	for part, value := range values {
		if err := adapter.IngestQuarterFrame(byte(part<<4) | value); err != nil {
			t.Fatal(err)
		}
	}
	want := time.Hour + 2*time.Minute + 3*time.Second + time.Second*4/30
	if got := mtc.Position(); absDuration(got-want) > 2*time.Millisecond {
		t.Fatalf("MTC position = %v, want %v", got, want)
	}
}

func TestParseOSCTimecodeStringAndFloat(t *testing.T) {
	stringPacket := oscPacket("/cusus/timecode", ",s", append([]byte("01:02:03:15"), 0))
	position, err := ParseOSC(stringPacket, 30)
	if err != nil || absDuration(position-(time.Hour+2*time.Minute+3500*time.Millisecond)) > time.Millisecond {
		t.Fatalf("string OSC = %v, %v", position, err)
	}
	raw := make([]byte, 4)
	binary.BigEndian.PutUint32(raw, math.Float32bits(12.5))
	position, err = ParseOSC(oscPacket("/timecode", ",f", raw), 30)
	if err != nil || position != 12500*time.Millisecond {
		t.Fatalf("float OSC = %v, %v", position, err)
	}
}

func oscPacket(address, types string, argument []byte) []byte {
	pad := func(value []byte) []byte {
		value = append(value, 0)
		for len(value)%4 != 0 {
			value = append(value, 0)
		}
		return value
	}
	packet := append(pad([]byte(address)), pad([]byte(types))...)
	packet = append(packet, argument...)
	for len(packet)%4 != 0 {
		packet = append(packet, 0)
	}
	return packet
}
