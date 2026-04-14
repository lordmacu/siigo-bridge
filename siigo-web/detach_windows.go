//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// Windows creation flags (avoid pulling in x/sys/windows).
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// applyDetachedSysProcAttr makes the child fully independent of the parent console,
// so the child survives when the parent calls os.Exit.
func applyDetachedSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
}

// applyHiddenSysProcAttr runs the child as a tracked child (parent can Kill it)
// but with no console window. Used for console apps like cloudflared.exe so they
// don't pop a black terminal when siigo-web.exe is built with -H windowsgui.
func applyHiddenSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow | createNewProcessGroup,
		HideWindow:    true,
	}
}
