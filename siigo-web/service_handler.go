package main

import (
	"net/http"
)

// handleServiceStatus returns current state of the Windows service (no UAC).
func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if !serviceSupported() {
		jsonResponse(w, map[string]interface{}{
			"supported": false,
			"state":     "not_supported",
		})
		return
	}
	state, display, err := getServiceStatus()
	resp := map[string]interface{}{
		"supported":    true,
		"state":        state,
		"display_name": display,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	jsonResponse(w, resp)
}

// handleServiceInstall launches install-service.bat elevated (UAC prompt once).
func (s *Server) handleServiceInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !serviceSupported() {
		jsonError(w, "only available on Windows", 400)
		return
	}
	if err := serviceInstall(); err != nil {
		s.db.AddLog("error", "SERVICE", "Install failed: "+err.Error())
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("info", "SERVICE", "Install requested — UAC prompt triggered")
	jsonResponse(w, map[string]interface{}{"status": "ok", "message": "UAC prompt shown. Accept to continue."})
}

// handleServiceUninstall launches uninstall-service.bat elevated.
func (s *Server) handleServiceUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !serviceSupported() {
		jsonError(w, "only available on Windows", 400)
		return
	}
	if err := serviceUninstall(); err != nil {
		s.db.AddLog("error", "SERVICE", "Uninstall failed: "+err.Error())
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("info", "SERVICE", "Uninstall requested")
	jsonResponse(w, map[string]interface{}{"status": "ok"})
}

// handleServiceRestart runs nssm restart elevated.
func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !serviceSupported() {
		jsonError(w, "only available on Windows", 400)
		return
	}
	if err := serviceRestart(); err != nil {
		s.db.AddLog("error", "SERVICE", "Restart failed: "+err.Error())
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("info", "SERVICE", "Restart requested")
	jsonResponse(w, map[string]interface{}{"status": "ok"})
}
