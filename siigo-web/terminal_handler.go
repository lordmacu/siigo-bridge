package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// terminalMsg is the JSON envelope exchanged over the WebSocket.
//   input: stdin from client (text or raw bytes)
//   resize: {cols, rows}
//   output: PTY stdout to client
//   exit: process exited
type terminalMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
}

// handleTerminalWS upgrades to WebSocket, spawns a PTY (PowerShell on Windows,
// bash elsewhere), and wires PTY <-> WebSocket both directions.
// Auth: JWT token in ?token= query param (browsers can't set WS headers).
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	info := s.getTokenInfo(token)
	if info == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	// Admin/root only — executing arbitrary shell commands is high privilege.
	if info.Role != "admin" && info.Role != "root" {
		http.Error(w, "admin only", 403)
		return
	}

	// Extra PIN gate: if terminal.pin_hash is set in config, require a matching ?pin= param.
	if hash := strings.TrimSpace(s.cfg.Terminal.PinHash); hash != "" {
		pin := r.URL.Query().Get("pin")
		if pin == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin)) != nil {
			s.db.AddLog("warn", "TERMINAL", "Rejected connection (bad pin) from "+info.Username)
			http.Error(w, "pin required", 403)
			return
		}
	}

	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Spawn PTY
	ptmx, err := pty.New()
	if err != nil {
		conn.WriteJSON(terminalMsg{Type: "exit", Data: "pty create failed: " + err.Error(), Code: -1})
		return
	}
	defer ptmx.Close()

	shell, args := pickShell()
	cmd := ptmx.Command(shell, args...)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		s.db.AddLog("error", "TERMINAL", "Start failed: "+err.Error())
		conn.WriteJSON(terminalMsg{Type: "exit", Data: "shell start failed: " + err.Error(), Code: -1})
		return
	}

	s.db.AddLog("info", "TERMINAL", "Session opened by "+info.Username)
	defer s.db.AddLog("info", "TERMINAL", "Session closed by "+info.Username)

	// Initial size
	_ = ptmx.Resize(120, 30)

	var wg sync.WaitGroup
	writeMu := sync.Mutex{}
	done := make(chan struct{})
	closeOnce := sync.Once{}
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	// PTY -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				werr := conn.WriteJSON(terminalMsg{Type: "output", Data: string(buf[:n])})
				writeMu.Unlock()
				if werr != nil {
					closeDone()
					return
				}
			}
			if err != nil {
				closeDone()
				return
			}
		}
	}()

	// WebSocket -> PTY (+ resize handling)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(60 * time.Minute))
			var msg terminalMsg
			if err := conn.ReadJSON(&msg); err != nil {
				closeDone()
				return
			}
			switch msg.Type {
			case "input":
				if _, err := ptmx.Write([]byte(msg.Data)); err != nil {
					closeDone()
					return
				}
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = ptmx.Resize(int(msg.Cols), int(msg.Rows))
				}
			case "ping":
				writeMu.Lock()
				_ = conn.WriteJSON(terminalMsg{Type: "pong"})
				writeMu.Unlock()
			}
		}
	}()

	// When one side closes, kill the shell so the other goroutine unblocks.
	go func() {
		<-done
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		ptmx.Close()
	}()

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = -1
	}
	writeMu.Lock()
	_ = conn.WriteJSON(terminalMsg{Type: "exit", Code: exitCode})
	writeMu.Unlock()
	closeDone()
	wg.Wait()
}

// installDirCompat returns the dir containing siigo-web.exe, portable across OS.
func installDirCompat() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// handleTerminalPin manages the extra PIN required to open the web terminal.
//   GET  -> { "set": true|false }       (reveals nothing else)
//   POST -> { "pin": "..." } sets it; "" clears it. Admin/root only.
func (s *Server) handleTerminalPin(w http.ResponseWriter, r *http.Request) {
	info := s.getTokenInfo(extractToken(r))
	if info == nil || (info.Role != "admin" && info.Role != "root") {
		jsonError(w, "admin only", 403)
		return
	}
	if r.Method == "GET" {
		jsonResponse(w, map[string]interface{}{"set": strings.TrimSpace(s.cfg.Terminal.PinHash) != ""})
		return
	}
	if r.Method != "POST" {
		jsonError(w, "POST or GET", 405)
		return
	}
	var body struct {
		Pin string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if body.Pin == "" {
		s.cfg.Terminal.PinHash = ""
		s.db.AddLog("info", "TERMINAL", "PIN cleared by "+info.Username)
	} else {
		if len(body.Pin) < 8 {
			jsonError(w, "pin must be at least 8 chars", 400)
			return
		}
		h, err := bcrypt.GenerateFromPassword([]byte(body.Pin), bcrypt.DefaultCost)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.cfg.Terminal.PinHash = string(h)
		s.db.AddLog("info", "TERMINAL", "PIN set by "+info.Username)
	}
	if err := s.cfg.Save("config.json"); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]interface{}{"status": "ok", "set": s.cfg.Terminal.PinHash != ""})
}

// pickShell returns an absolute-path shell and its args, robust across Windows
// versions. Avoids PATH lookup issues when running as LocalSystem service.
func pickShell() (string, []string) {
	if runtime.GOOS == "windows" {
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		candidates := []string{
			filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
			filepath.Join(windir, "System32", "cmd.exe"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				if strings.HasSuffix(strings.ToLower(p), "powershell.exe") {
					return p, []string{"-NoLogo"}
				}
				return p, nil
			}
		}
		return candidates[1], nil
	}
	return "/bin/bash", []string{"-i"}
}
