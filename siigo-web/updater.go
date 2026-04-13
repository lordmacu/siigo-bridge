package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Set at build time: go build -ldflags "-X main.Version=1.0.0"
var Version = "dev"

const (
	githubRepo     = "lordmacu/siigo-bridge"
	updateCheckURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
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

// checkAndUpdate checks GitHub for a newer release and applies it.
// Guarded by updateMu so concurrent triggers can't race on the .old/.update files.
func (s *Server) checkAndUpdate() {
	if !s.updateMu.TryLock() {
		s.db.AddLog("warn", "UPDATER", "Update already in progress, ignoring")
		return
	}
	defer s.updateMu.Unlock()

	if Version == "dev" {
		s.db.AddLog("warn", "UPDATER", "Dev build — refusing to self-replace")
		return
	}

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

	// Find the right asset for the current OS.
	var downloadURL string
	var expectedSize int64
	wantSuffix := ".exe"
	if runtime.GOOS != "windows" {
		wantSuffix = ""
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.Contains(name, "siigo-web") {
			continue
		}
		if wantSuffix != "" && !strings.HasSuffix(name, wantSuffix) {
			continue
		}
		downloadURL = asset.BrowserDownloadURL
		expectedSize = asset.Size
		break
	}

	if downloadURL == "" {
		s.db.AddLog("warn", "UPDATER", "No compatible asset found in release")
		return
	}

	s.db.AddLog("info", "UPDATER", "Downloading update...")
	exePath, err := os.Executable()
	if err != nil {
		s.db.AddLog("error", "UPDATER", "Cannot find executable path: "+err.Error())
		return
	}

	tmpPath := exePath + ".update"
	if err := downloadFile(downloadURL, tmpPath, expectedSize); err != nil {
		s.db.AddLog("error", "UPDATER", "Download failed: "+err.Error())
		os.Remove(tmpPath)
		return
	}

	s.db.AddLog("info", "UPDATER", fmt.Sprintf("Update downloaded. Replacing %s...", filepath.Base(exePath)))

	if runtime.GOOS == "windows" {
		// Windows can't delete a running exe but CAN rename it.
		oldPath := exePath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			s.db.AddLog("error", "UPDATER", "Cannot rename current exe: "+err.Error())
			os.Remove(tmpPath)
			return
		}
		if err := os.Rename(tmpPath, exePath); err != nil {
			os.Rename(oldPath, exePath) // rollback
			s.db.AddLog("error", "UPDATER", "Cannot place new exe: "+err.Error())
			return
		}
		// .old gets removed by cleanupOldExe() on next startup.
	} else {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			s.db.AddLog("warn", "UPDATER", "chmod failed: "+err.Error())
		}
		if err := os.Rename(tmpPath, exePath); err != nil {
			s.db.AddLog("error", "UPDATER", "Cannot replace exe: "+err.Error())
			return
		}
	}

	s.db.AddLog("info", "UPDATER", fmt.Sprintf("Updated to %s. Restarting...", latestVersion))

	// Best-effort Telegram notification with a short timeout so shutdown is not blocked.
	notifyDone := make(chan struct{})
	go func() {
		if s.bot != nil {
			s.bot.Send(fmt.Sprintf("🔄 <b>Update manual</b>\n\n%s → %s\nReiniciando...", Version, latestVersion))
		}
		close(notifyDone)
	}()
	select {
	case <-notifyDone:
	case <-time.After(3 * time.Second):
	}

	// Stop HTTP + sync loops before spawning the replacement so no requests or DB writes
	// are mid-flight when the new process starts.
	s.gracefulShutdown()
	if s.db != nil {
		s.db.Close()
	}

	restartProcess(exePath)
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
	if Version == "dev" {
		jsonError(w, "dev build cannot self-update", 400)
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
		// Small delay so the JSON response finishes flushing to the client.
		time.Sleep(500 * time.Millisecond)
		exePath, err := os.Executable()
		if err != nil {
			return
		}
		s.gracefulShutdown()
		if s.db != nil {
			s.db.Close()
		}
		restartProcess(exePath)
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

func downloadFile(url, dest string, expectedSize int64) error {
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

	written, err := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("size mismatch: got %d bytes, expected %d", written, expectedSize)
	}
	if written < 1024 {
		return fmt.Errorf("downloaded file suspiciously small (%d bytes)", written)
	}
	return nil
}
