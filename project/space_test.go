package project

import "testing"

func TestAvailableBytesForWritableDirectory(t *testing.T) {
	available, err := AvailableBytes(t.TempDir())
	if err != nil {
		t.Fatalf("AvailableBytes: %v", err)
	}
	if available == 0 {
		t.Fatal("AvailableBytes returned zero for the test volume")
	}
}
