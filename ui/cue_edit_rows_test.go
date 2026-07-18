package ui

import (
	"runtime"
	"testing"
)

func TestSameFilePathUsesPlatformCaseRules(t *testing.T) {
	if !sameFilePath("media/../Silence.wav", "Silence.wav") {
		t.Fatal("cleaned forms of the same path did not match")
	}

	wantCaseOnlyMatch := runtime.GOOS == "windows"
	if got := sameFilePath("Silence.wav", "silence.wav"); got != wantCaseOnlyMatch {
		t.Fatalf("case-only path match = %t, want %t on %s", got, wantCaseOnlyMatch, runtime.GOOS)
	}
}
