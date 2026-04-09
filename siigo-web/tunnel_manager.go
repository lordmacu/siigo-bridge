package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TunnelConfig holds the cloudflare tunnel settings
type TunnelConfig struct {
	Enabled     bool   `json:"enabled"`
	TunnelID    string `json:"tunnel_id"`
	Hostname    string `json:"hostname"`
	Credentials string `json:"credentials_path"`
}

var tunnelProcess *exec.Cmd

const cloudflaredDownloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"

// cloudflaredPath returns the path to cloudflared.exe in the SiigoWeb directory
func cloudflaredPath() string {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "cloudflared.exe")
}

// ensureCloudflared downloads cloudflared.exe if not already present
func ensureCloudflared() error {
	path := cloudflaredPath()
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	// Download from official GitHub release
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

// handleTunnelStatus returns the current tunnel state
func (s *Server) handleTunnelStatus(w http.ResponseWriter, r *http.Request) {
	cloudflared := cloudflaredPath()
	_, statErr := os.Stat(cloudflared)
	exists := statErr == nil

	running := tunnelProcess != nil && tunnelProcess.Process != nil

	jsonResponse(w, map[string]interface{}{
		"cloudflared_available": exists,
		"cloudflared_path":      cloudflared,
		"running":               running,
		"hostname":              s.getTunnelHostname(),
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
	s.db.AddLog("info", "TUNNEL", "cloudflared.exe installed at "+cloudflaredPath())
	jsonResponse(w, map[string]string{"status": "installed", "path": cloudflaredPath()})
}

// getTunnelHostname reads the hostname from the tunnel config
func (s *Server) getTunnelHostname() string {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "cloudflared", "config.yml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- hostname:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- hostname:"))
		}
	}
	return ""
}

// handleTunnelStart starts the cloudflared tunnel
func (s *Server) handleTunnelStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	// Auto-download cloudflared if not present
	if err := ensureCloudflared(); err != nil {
		jsonError(w, "Cannot install cloudflared: "+err.Error(), 500)
		return
	}
	cloudflared := cloudflaredPath()

	if tunnelProcess != nil && tunnelProcess.Process != nil {
		jsonResponse(w, map[string]string{"status": "already_running"})
		return
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "cloudflared", "config.yml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		jsonError(w, "Tunnel config not found at "+configPath, 404)
		return
	}

	cmd := exec.Command(cloudflared, "tunnel", "--config", configPath, "run")
	logFile, _ := os.Create(filepath.Join(exeDir, "cloudflared", "tunnel.log"))
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		jsonError(w, "Failed to start tunnel: "+err.Error(), 500)
		return
	}

	tunnelProcess = cmd
	s.db.AddLog("info", "TUNNEL", "Cloudflare tunnel started")
	jsonResponse(w, map[string]string{"status": "started", "hostname": s.getTunnelHostname()})
}

// handleTunnelStop stops the cloudflared tunnel
func (s *Server) handleTunnelStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	if tunnelProcess == nil || tunnelProcess.Process == nil {
		jsonResponse(w, map[string]string{"status": "not_running"})
		return
	}

	tunnelProcess.Process.Kill()
	tunnelProcess = nil
	s.db.AddLog("info", "TUNNEL", "Cloudflare tunnel stopped")
	jsonResponse(w, map[string]string{"status": "stopped"})
}

// handleTunnelProvision: called by installer or manually. Downloads tunnel credentials
// from a central provisioning server and sets up the tunnel config.
// POST body: {"customer_id": "abc123", "token": "provision-token"}
func (s *Server) handleTunnelProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	var body struct {
		CustomerID      string `json:"customer_id"`
		ProvisionURL    string `json:"provision_url"`
		ProvisionToken  string `json:"provision_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid JSON", 400)
		return
	}

	if body.CustomerID == "" || body.ProvisionURL == "" {
		jsonError(w, "customer_id and provision_url required", 400)
		return
	}

	// Request tunnel credentials from the provisioning server
	req, _ := http.NewRequest("POST", body.ProvisionURL, strings.NewReader(fmt.Sprintf(`{"customer_id":"%s"}`, body.CustomerID)))
	req.Header.Set("Content-Type", "application/json")
	if body.ProvisionToken != "" {
		req.Header.Set("Authorization", "Bearer "+body.ProvisionToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		jsonError(w, "Provisioning request failed: "+err.Error(), 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		jsonError(w, fmt.Sprintf("Provisioning server returned %d: %s", resp.StatusCode, string(body)), 500)
		return
	}

	var provisionResult struct {
		TunnelID    string `json:"tunnel_id"`
		Hostname    string `json:"hostname"`
		Credentials string `json:"credentials"` // JSON credentials file content
	}
	if err := json.NewDecoder(resp.Body).Decode(&provisionResult); err != nil {
		jsonError(w, "Invalid provisioning response: "+err.Error(), 500)
		return
	}

	// Save credentials and config
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	tunnelDir := filepath.Join(exeDir, "cloudflared")
	os.MkdirAll(tunnelDir, 0755)

	credsPath := filepath.Join(tunnelDir, provisionResult.TunnelID+".json")
	if err := os.WriteFile(credsPath, []byte(provisionResult.Credentials), 0600); err != nil {
		jsonError(w, "Cannot write credentials: "+err.Error(), 500)
		return
	}

	configContent := fmt.Sprintf(`tunnel: %s
credentials-file: %s

ingress:
  - hostname: %s
    service: http://localhost:%s
  - service: http_status:404
`, provisionResult.TunnelID, credsPath, provisionResult.Hostname, s.cfg.Server.Port)

	configPath := filepath.Join(tunnelDir, "config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		jsonError(w, "Cannot write config: "+err.Error(), 500)
		return
	}

	s.db.AddLog("info", "TUNNEL", "Tunnel provisioned for "+provisionResult.Hostname)
	jsonResponse(w, map[string]interface{}{
		"status":   "provisioned",
		"hostname": provisionResult.Hostname,
		"tunnel_id": provisionResult.TunnelID,
	})
}
