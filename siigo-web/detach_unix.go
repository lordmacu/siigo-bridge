//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// applyDetachedSysProcAttr puts the child in its own session so it survives
// the parent exiting.
func applyDetachedSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
