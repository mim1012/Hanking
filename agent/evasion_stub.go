//go:build !windows || !amd64

package main

import (
	"context"
	"os/exec"
)

// evasionInit is a no-op on non-Windows/non-amd64 builds.
func evasionInit() {}

// hiddenCmdCtx is a no-op on non-Windows.
func hiddenCmdCtx(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// hiddenCmd is a no-op on non-Windows.
func hiddenCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
