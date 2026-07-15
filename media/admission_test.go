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
