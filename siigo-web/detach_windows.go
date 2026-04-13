//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// Windows creation flags (avoid pulling in x/sys/windows).
const (
	detachedProcess        = 0x00000008
	createNewProcessGroup  = 0x00000200
)

// applyDetachedSysProcAttr makes the child fully independent of the parent console,
// so the child survives when the parent calls os.Exit.
func applyDetachedSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
}
