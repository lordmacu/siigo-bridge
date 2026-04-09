package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// handleExec runs a shell command remotely. Requires:
// - Valid auth token (via authMiddleware)
// - PIN matching config.telegram.exec_pin
// - POST body: {"pin": "2337", "cmd": "ipconfig", "shell": "powershell"|"cmd", "timeout": 30}
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	var body struct {
		PIN     string `json:"pin"`
		Cmd     string `json:"cmd"`
		Shell   string `json:"shell"`
		Timeout int    `json:"timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid JSON: "+err.Error(), 400)
		return
	}

	if body.Cmd == "" {
		jsonError(w, "cmd required", 400)
		return
	}

	// PIN validation
	expectedPIN := s.cfg.Telegram.ExecPin
	if expectedPIN == "" {
		jsonError(w, "Exec PIN not configured. Set telegram.exec_pin in config", 403)
		return
	}
	if body.PIN != expectedPIN {
		s.db.AddLog("warn", "EXEC", "PIN incorrect attempt from API")
		jsonError(w, "Invalid PIN", 403)
		return
	}

	// Defaults
	if body.Timeout <= 0 {
		body.Timeout = 30
	}
	if body.Timeout > 300 {
		body.Timeout = 300 // max 5 min
	}
	if body.Shell == "" {
		if runtime.GOOS == "windows" {
			body.Shell = "cmd"
		} else {
			body.Shell = "bash"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(body.Timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch body.Shell {
	case "powershell", "ps":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", body.Cmd)
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd", "/C", body.Cmd)
	case "bash":
		cmd = exec.CommandContext(ctx, "bash", "-c", body.Cmd)
	default:
		jsonError(w, "Invalid shell. Use: cmd, powershell, bash", 400)
		return
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Log the execution
	logMsg := body.Cmd
	if len(logMsg) > 100 {
		logMsg = logMsg[:100] + "..."
	}
	s.db.AddLog("info", "EXEC", "Remote exec: "+logMsg+" (exit="+errMsg+")")

	jsonResponse(w, map[string]interface{}{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
		"duration":  duration.Milliseconds(),
		"error":     errMsg,
	})
}
