package osc

import "testing"

func TestStringFramingRoundTripAndAlignment(t *testing.T) {
	packet, err := AppendString(nil, "/cue")
	if err != nil {
		t.Fatal(err)
	}
	packet, err = AppendString(packet, ",s")
	if err != nil {
		t.Fatal(err)
	}
	address, offset, err := ReadString(packet, 0)
	if err != nil || address != "/cue" || offset%alignment != 0 {
		t.Fatalf("address = %q, offset = %d, err = %v", address, offset, err)
	}
	tags, next, err := ReadString(packet, offset)
	if err != nil || tags != ",s" || next != len(packet) {
		t.Fatalf("tags = %q, next = %d, err = %v", tags, next, err)
	}
}

func TestStringFramingRejectsMalformedInput(t *testing.T) {
	if _, err := AppendString(nil, "bad\x00value"); err == nil {
		t.Fatal("embedded NUL was accepted")
	}
	if _, _, err := ReadString([]byte("unterminated"), 0); err == nil {
		t.Fatal("unterminated string was accepted")
	}
}
