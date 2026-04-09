package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	tunnelProcess    *exec.Cmd
	currentTunnelURL string
	tunnelMu         sync.Mutex
)

const cloudflaredDownloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"

// cloudflaredPath returns the path to cloudflared.exe in the SiigoWeb directory
func cloudflaredPath() string {
	exePath, _ := os.Executable()
	return filepath.Join(filepath.Dir(exePath), "cloudflared.exe")
}

// ensureCloudflared downloads cloudflared.exe if not already present
func ensureCloudflared() error {
	path := cloudflaredPath()
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(cloudflaredDownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cannot create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(path)
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

// startQuickTunnel starts a cloudflare quick tunnel (random trycloudflare.com URL)
// and sends the URL via Telegram once detected.
func (s *Server) startQuickTunnel() {
	if err := ensureCloudflared(); err != nil {
		s.db.AddLog("warn", "TUNNEL", "Cannot install cloudflared: "+err.Error())
		return
	}

	tunnelMu.Lock()
	if tunnelProcess != nil && tunnelProcess.Process != nil {
		tunnelMu.Unlock()
		return // already running
	}
	tunnelMu.Unlock()

	port := s.cfg.Server.Port
	if port == "" {
		port = "3210"
	}

	exePath, _ := os.Executable()
	logDir := filepath.Join(filepath.Dir(exePath), "cloudflared")
	os.MkdirAll(logDir, 0755)

	// Create empty config to override the default ~/.cloudflared/config.yml
	emptyConfig := filepath.Join(logDir, "empty-config.yml")
	os.WriteFile(emptyConfig, []byte("# Empty config to force quick tunnel mode\n"), 0644)

	cmd := exec.Command(cloudflaredPath(),
		"--config", emptyConfig,
		"tunnel",
		"--url", "http://localhost:"+port,
		"--no-autoupdate",
	)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		s.db.AddLog("error", "TUNNEL", "Cannot create stderr pipe: "+err.Error())
		return
	}

	logFile, _ := os.Create(filepath.Join(logDir, "quick-tunnel.log"))
	cmd.Stdout = logFile

	if err := cmd.Start(); err != nil {
		s.db.AddLog("error", "TUNNEL", "Cannot start cloudflared: "+err.Error())
		return
	}

	tunnelMu.Lock()
	tunnelProcess = cmd
	currentTunnelURL = ""
	tunnelMu.Unlock()

	s.db.AddLog("info", "TUNNEL", "Quick tunnel starting, waiting for URL...")

	urlRegex := regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
	go func() {
		defer logFile.Close()
		scanner := bufio.NewScanner(stderrPipe)
		urlFound := false
		for scanner.Scan() {
			line := scanner.Text()
			logFile.WriteString(line + "\n")
			if !urlFound {
				if match := urlRegex.FindString(line); match != "" {
					urlFound = true
					tunnelMu.Lock()
					currentTunnelURL = match
					tunnelMu.Unlock()
					s.db.AddLog("info", "TUNNEL", "Quick tunnel active: "+match)
					s.notifyTunnelURL(match)
				}
			}
		}
	}()
}

// notifyTunnelURL sends the tunnel URL via Telegram AND pushes it to Finearom
func (s *Server) notifyTunnelURL(url string) {
	// Telegram notification
	if s.bot != nil && s.bot.IsEnabled() {
		msg := fmt.Sprintf(
			"🌐 <b>Tunnel activo</b>\n\n"+
				"URL publica: %s\n"+
				"Local: http://localhost:%s\n\n"+
				"<i>Esta URL cambia cada vez que se reinicia el programa.</i>",
			url, s.cfg.Server.Port,
		)
		s.bot.Send(msg)
	}

	// Push to Finearom (register the new URL so Finearom knows how to reach us)
	go s.pushTunnelURLToFinearom(url)
}

// pushTunnelURLToFinearom registers the current tunnel URL in Finearom.
// Finearom stores it keyed by the sync user email, so next time it needs
// to reach this instance it uses the latest URL.
func (s *Server) pushTunnelURLToFinearom(tunnelURL string) {
	base := strings.TrimRight(s.cfg.Finearom.BaseURL, "/")
	if base == "" || s.cfg.Finearom.Email == "" || s.cfg.Finearom.Password == "" {
		return
	}

	// Login first
	loginBody, _ := json.Marshal(map[string]string{
		"email":    s.cfg.Finearom.Email,
		"password": s.cfg.Finearom.Password,
	})
	client := &http.Client{Timeout: 15 * time.Second}
	loginResp, err := client.Post(base+"/siigo/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		s.db.AddLog("warn", "TUNNEL", "Finearom login failed: "+err.Error())
		return
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != 200 {
		body, _ := io.ReadAll(loginResp.Body)
		s.db.AddLog("warn", "TUNNEL", fmt.Sprintf("Finearom login returned %d: %s", loginResp.StatusCode, string(body)))
		return
	}

	var loginResult struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil || loginResult.Token == "" {
		s.db.AddLog("warn", "TUNNEL", "Finearom login response invalid")
		return
	}

	// Push URL
	payload, _ := json.Marshal(map[string]string{
		"tunnel_url": tunnelURL,
		"version":    Version,
	})
	req, _ := http.NewRequest("POST", base+"/siigo/tunnel", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResult.Token)

	resp, err := client.Do(req)
	if err != nil {
		s.db.AddLog("warn", "TUNNEL", "Push tunnel URL failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		s.db.AddLog("info", "TUNNEL", "Tunnel URL registered in Finearom: "+tunnelURL)
	} else {
		body, _ := io.ReadAll(resp.Body)
		s.db.AddLog("warn", "TUNNEL", fmt.Sprintf("Finearom tunnel register returned %d: %s", resp.StatusCode, string(body)))
	}
}

// GetCurrentTunnelURL returns the current tunnel URL (empty if not running)
func GetCurrentTunnelURL() string {
	tunnelMu.Lock()
	defer tunnelMu.Unlock()
	return currentTunnelURL
}

// startTunnelWatchdog runs every 5 minutes: checks if tunnel is alive and
// the URL is the same as last pushed. If changed, re-pushes to Finearom.
func (s *Server) startTunnelWatchdog() {
	lastPushed := ""
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				current := GetCurrentTunnelURL()

				// If tunnel died, restart it
				tunnelMu.Lock()
				alive := tunnelProcess != nil && tunnelProcess.Process != nil
				tunnelMu.Unlock()
				if !alive || current == "" {
					s.db.AddLog("warn", "TUNNEL", "Tunnel not alive, restarting...")
					s.startQuickTunnel()
					continue
				}

				// If URL changed since last push, push again
				if current != lastPushed {
					s.db.AddLog("info", "TUNNEL", "Tunnel URL changed, re-pushing to Finearom")
					s.pushTunnelURLToFinearom(current)
					lastPushed = current
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

// handleTunnelStatus returns the current tunnel state
func (s *Server) handleTunnelStatus(w http.ResponseWriter, r *http.Request) {
	path := cloudflaredPath()
	_, statErr := os.Stat(path)
	exists := statErr == nil

	tunnelMu.Lock()
	running := tunnelProcess != nil && tunnelProcess.Process != nil
	url := currentTunnelURL
	tunnelMu.Unlock()

	jsonResponse(w, map[string]interface{}{
		"cloudflared_available": exists,
		"cloudflared_path":      path,
		"running":               running,
		"public_url":            url,
	})
}

// handleTunnelInstall downloads cloudflared.exe if not present
func (s *Server) handleTunnelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	s.db.AddLog("info", "TUNNEL", "Downloading cloudflared.exe...")
	if err := ensureCloudflared(); err != nil {
		s.db.AddLog("error", "TUNNEL", "Failed to download cloudflared: "+err.Error())
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "installed", "path": cloudflaredPath()})
}

// handleTunnelStart (re)starts the quick tunnel
func (s *Server) handleTunnelStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	tunnelMu.Lock()
	if tunnelProcess != nil && tunnelProcess.Process != nil {
		tunnelProcess.Process.Kill()
		tunnelProcess = nil
		currentTunnelURL = ""
	}
	tunnelMu.Unlock()

	go s.startQuickTunnel()
	jsonResponse(w, map[string]string{"status": "starting", "message": "URL available in ~5 seconds"})
}

// handleTunnelStop kills the tunnel process
func (s *Server) handleTunnelStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	tunnelMu.Lock()
	if tunnelProcess == nil || tunnelProcess.Process == nil {
		tunnelMu.Unlock()
		jsonResponse(w, map[string]string{"status": "not_running"})
		return
	}
	tunnelProcess.Process.Kill()
	tunnelProcess = nil
	currentTunnelURL = ""
	tunnelMu.Unlock()

	s.db.AddLog("info", "TUNNEL", "Tunnel stopped")
	jsonResponse(w, map[string]string{"status": "stopped"})
}
