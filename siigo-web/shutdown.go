package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

var shutdownOnce sync.Once

// gracefulShutdown stops sync loops and the HTTP server cleanly.
// Safe to call multiple times — only the first call has effect.
func (s *Server) gracefulShutdown() {
	shutdownOnce.Do(func() { s.doShutdown() })
}

func (s *Server) doShutdown() {
	log.Println("Shutdown signal received, stopping...")
	if s.db != nil {
		s.db.AddLog("info", "APP", "Server stopped (graceful shutdown)")
	}

	// Stop sync loops (detect + send)
	if s.stopCh != nil {
		select {
		case s.stopCh <- true:
		default:
		}
		select {
		case s.stopCh <- true:
		default:
		}
	}

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	// Kill the cloudflared child so the replacement process can claim its own tunnel
	// instead of leaving an orphaned one behind.
	tunnelMu.Lock()
	if tunnelProcess != nil && tunnelProcess.Process != nil {
		_ = tunnelProcess.Process.Kill()
		tunnelProcess = nil
	}
	tunnelMu.Unlock()
}

// restartProcess launches a detached copy of the given exe and exits the current process.
// The child survives the parent exiting (Windows: DETACHED_PROCESS; Unix: new session).
func restartProcess(exePath string) {
	cmd := exec.Command(exePath)
	applyDetachedSysProcAttr(cmd)
	// Do NOT share stdio — child must be independent of parent console.
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		log.Printf("[Restart] failed to spawn child: %v", err)
		return
	}
	// Release so the child isn't tracked by the exiting parent.
	_ = cmd.Process.Release()
}

// cleanupOldExe removes a stale `<exe>.old` left behind by a previous Windows update.
// Called at startup; the old exe is no longer running so deletion succeeds now.
func cleanupOldExe() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := exePath + ".old"
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.Remove(oldPath)
	}
	// Also remove any half-downloaded update file.
	_ = os.Remove(exePath + ".update")
}
