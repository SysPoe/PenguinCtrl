package processgroup

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestSupervisedCommandOutput(t *testing.T) {
	name, args := "sh", []string{"-c", "printf supervised"}
	if runtime.GOOS == "windows" {
		name, args = "cmd.exe", []string{"/d", "/c", "echo supervised"}
	}
	output, err := Output(CommandContext(context.Background(), name, args...))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(output)) != "supervised" {
		t.Fatalf("output = %q", output)
	}
}
