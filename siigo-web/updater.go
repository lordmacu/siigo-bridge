package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Set at build time: go build -ldflags "-X main.Version=1.0.0"
var Version = "dev"

const (
	githubRepo    = "lordmacu/siigo-bridge"
	updateCheckURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
	updateHour    = 2 // Check at 2 AM
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// startAutoUpdater runs a background loop that checks for updates at updateHour AM daily
func (s *Server) startAutoUpdater() {
	if Version == "dev" {
		log.Println("[Updater] Dev mode — auto-update disabled")
		return
	}
	log.Printf("[Updater] Current version: %s — checking daily at %d:00 AM", Version, updateHour)

	go func() {
		for {
			now := time.Now()
			// Calculate next check time (today or tomorrow at updateHour:00)
			next := time.Date(now.Year(), now.Month(), now.Day(), updateHour, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			waitDuration := time.Until(next)
			log.Printf("[Updater] Next check in %s (at %s)", waitDuration.Round(time.Minute), next.Format("2006-01-02 15:04"))

			select {
			case <-time.After(waitDuration):
				s.checkAndUpdate()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// checkAndUpdate checks GitHub for a newer release and applies it
func (s *Server) checkAndUpdate() {
	s.db.AddLog("info", "UPDATER", "Checking for updates...")

	release, err := getLatestRelease()
	if err != nil {
		s.db.AddLog("warn", "UPDATER", "Failed to check: "+err.Error())
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion == Version {
		s.db.AddLog("info", "UPDATER", fmt.Sprintf("Already on latest version (%s)", Version))
		return
	}

	s.db.AddLog("info", "UPDATER", fmt.Sprintf("New version available: %s → %s", Version, latestVersion))

	// Find the right asset (siigo-web.exe for Windows)
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(strings.ToLower(asset.Name), "siigo-web") &&
			strings.HasSuffix(strings.ToLower(asset.Name), ".exe") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		s.db.AddLog("warn", "UPDATER", "No compatible exe found in release assets")
		return
	}

	// Download to temp file
	s.db.AddLog("info", "UPDATER", "Downloading update...")
	exePath, err := os.Executable()
	if err != nil {
		s.db.AddLog("error", "UPDATER", "Cannot find executable path: "+err.Error())
		return
	}

	tmpPath := exePath + ".update"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		s.db.AddLog("error", "UPDATER", "Download failed: "+err.Error())
		os.Remove(tmpPath)
		return
	}

	// Replace exe and restart
	s.db.AddLog("info", "UPDATER", fmt.Sprintf("Update downloaded. Replacing %s...", filepath.Base(exePath)))

	if runtime.GOOS == "windows" {
		// Windows can't replace a running exe directly
		// Rename current exe to .old, move new to current, then restart
		oldPath := exePath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			s.db.AddLog("error", "UPDATER", "Cannot rename current exe: "+err.Error())
			os.Remove(tmpPath)
			return
		}
		if err := os.Rename(tmpPath, exePath); err != nil {
			// Rollback
			os.Rename(oldPath, exePath)
			s.db.AddLog("error", "UPDATER", "Cannot place new exe: "+err.Error())
			return
		}
		// Clean up old exe on next start (can't delete while running)
	} else {
		if err := os.Rename(tmpPath, exePath); err != nil {
			s.db.AddLog("error", "UPDATER", "Cannot replace exe: "+err.Error())
			return
		}
	}

	s.db.AddLog("info", "UPDATER", fmt.Sprintf("Updated to %s. Restarting...", latestVersion))

	// Notify via Telegram
	s.bot.Send(fmt.Sprintf("🔄 <b>Auto-update</b>\n\n%s → %s\nReiniciando...", Version, latestVersion))

	// Restart: launch new process and exit current
	cmd := exec.Command(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	os.Exit(0)
}

// handleCheckUpdate exposes manual update check via API
func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	release, err := getLatestRelease()
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"current_version": Version,
			"error":           err.Error(),
		})
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	updateAvailable := latestVersion != Version && Version != "dev"

	jsonResponse(w, map[string]interface{}{
		"current_version":  Version,
		"latest_version":   latestVersion,
		"update_available": updateAvailable,
		"release_name":     release.Name,
	})
}

// handleApplyUpdate triggers an immediate update
func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	jsonResponse(w, map[string]string{"status": "updating"})
	go s.checkAndUpdate()
}

// handleRestart restarts the application
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	jsonResponse(w, map[string]string{"status": "restarting"})
	s.db.AddLog("info", "APP", "Restart requested via API")

	go func() {
		time.Sleep(500 * time.Millisecond)
		exePath, err := os.Executable()
		if err != nil {
			return
		}
		cmd := exec.Command(exePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Start()
		os.Exit(0)
	}()
}

func getLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(updateCheckURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
