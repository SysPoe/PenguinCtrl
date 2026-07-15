package processgroup

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// CommandContext is retained as a compatibility constructor for callers that
// pair commands with this package's supervised Start and output helpers.
// Deprecated: use exec.CommandContext when migrating an existing caller.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func Output(cmd *exec.Cmd) ([]byte, error) {
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := Start(cmd); err != nil {
		return nil, err
	}
	err := cmd.Wait()
	return stdout.Bytes(), err
}

func CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := Start(cmd); err != nil {
		return nil, err
	}
	err := cmd.Wait()
	return output.Bytes(), err
}
