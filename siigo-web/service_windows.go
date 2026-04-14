//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serviceName = "SiigoBridge"

// getServiceStatus returns the current state: "running", "stopped", "not_installed", or "unknown".
// sc query is readable by non-admin users — no UAC needed.
func getServiceStatus() (state string, displayName string, err error) {
	cmd := exec.Command("sc", "query", serviceName)
	applyHiddenSysProcAttr(cmd)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if strings.Contains(text, "1060") || strings.Contains(text, "does not exist") {
			return "not_installed", "", nil
		}
		return "unknown", "", fmt.Errorf("sc query: %v: %s", err, text)
	}
	switch {
	case strings.Contains(text, "RUNNING"):
		return "running", serviceName, nil
	case strings.Contains(text, "STOPPED"):
		return "stopped", serviceName, nil
	case strings.Contains(text, "START_PENDING"):
		return "starting", serviceName, nil
	case strings.Contains(text, "STOP_PENDING"):
		return "stopping", serviceName, nil
	default:
		return "unknown", serviceName, nil
	}
}

// runElevated launches batPath via PowerShell Start-Process -Verb RunAs.
// Triggers Windows UAC — user must accept once. Bat script self-handles elevation after that.
// Returns immediately; the bat runs detached.
func runElevated(batPath string) error {
	if _, err := os.Stat(batPath); err != nil {
		return fmt.Errorf("bat not found: %s", batPath)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`Start-Process -FilePath '%s' -Verb RunAs -WindowStyle Normal`, batPath))
	applyHiddenSysProcAttr(cmd)
	return cmd.Start()
}

// installDir returns the directory containing siigo-web.exe.
func installDir() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}

// serviceInstall triggers install-service.bat elevated.
func serviceInstall() error {
	return runElevated(filepath.Join(installDir(), "install-service.bat"))
}

// serviceUninstall triggers uninstall-service.bat elevated.
func serviceUninstall() error {
	return runElevated(filepath.Join(installDir(), "uninstall-service.bat"))
}

// serviceRestart runs nssm restart elevated (or falls back to net stop/start).
func serviceRestart() error {
	nssm := filepath.Join(installDir(), "nssm", "nssm.exe")
	if _, err := os.Stat(nssm); err != nil {
		// fallback: sc stop + start via elevated cmd
		ps := fmt.Sprintf(`Start-Process -FilePath 'cmd.exe' -ArgumentList '/c sc stop %s & timeout /t 3 /nobreak & sc start %s' -Verb RunAs -WindowStyle Hidden`, serviceName, serviceName)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
		applyHiddenSysProcAttr(cmd)
		return cmd.Start()
	}
	ps := fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList 'restart','%s' -Verb RunAs -WindowStyle Hidden`, nssm, serviceName)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	applyHiddenSysProcAttr(cmd)
	return cmd.Start()
}

// serviceSupported returns true — we're on Windows.
func serviceSupported() bool { return true }
