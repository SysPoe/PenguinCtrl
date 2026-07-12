package media

import (
	"errors"
	"strings"
	"testing"
)

func TestFFmpegCommandErrorPreservesProcessOutput(t *testing.T) {
	err := ffmpegCommandError("decoder", errors.New("exit status 1"), "Invalid data found when processing input\n")
	for _, want := range []string{"decoder", "exit status 1", "Invalid data found when processing input"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}
