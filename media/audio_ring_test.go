package media

import (
	"bytes"
	"testing"
	"time"
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
