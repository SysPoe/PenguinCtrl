package media

import (
	"context"
	"testing"
)

func TestDecoderAdmissionEnforcesSessionAndMemoryBudgets(t *testing.T) {
	admission := newDecoderAdmission()
	for range maxDecoderSessions {
		if !admission.acquire(context.Background(), context.Background(), 1) {
			t.Fatal("decoder inside budget was rejected")
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if admission.acquire(cancelled, context.Background(), 1) {
		t.Fatal("decoder above session budget was accepted")
	}
	for range maxDecoderSessions {
		admission.release(1)
	}
	if admission.acquire(cancelled, context.Background(), maxVideoBufferBytes+1) {
		t.Fatal("decoder above memory budget was accepted")
	}
}

func TestParseMediaInfoRejectsExcessiveBitrate(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","width":1920,"height":1080,"avg_frame_rate":"30/1","bit_rate":"500000001"}]}`
	if _, err := parseMediaInfo([]byte(raw)); err == nil {
		t.Fatal("excessive video bitrate was accepted")
	}
}
