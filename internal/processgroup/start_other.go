//go:build !windows

package processgroup

import "os/exec"

func Start(cmd *exec.Cmd) error { return cmd.Start() }
