package main

import (
	"bytes"
	"context"
	"database/sql"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"siigo-common/api"
	"siigo-common/config"
	"siigo-common/isam"
	"siigo-common/parsers"
	"siigo-common/storage"
	"siigo-common/telegram"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/golang-jwt/jwt/v5"
)

//go:embed frontend/dist
var embeddedFrontend embed.FS

//go:embed siigobridge_tray.ico
var trayIconData []byte

// Auth credentials loaded from config.json (auth.username / auth.password)

type TokenInfo struct {
	Expiry      time.Time
	Username    string
	Role        string   // "root", "admin", "editor", "viewer"
	Permissions []string // allowed module paths
}

// rateLimiter tracks request counts per IP with sliding window
type rateLimiter struct {
	mu       gosync.Mutex
	requests map[string][]time.Time
	limit    int           // max requests per window
	window   time.Duration // time window
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove expired entries
	times := rl.requests[ip]
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}

	rl.requests[ip] = append(valid, now)
	return true
}

// cleanup removes expired entries periodically
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-rl.window)
	for ip, times := range rl.requests {
		valid := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, ip)
		} else {
			rl.requests[ip] = valid
		}
	}
}

type Server struct {
	db             *storage.DB
	cfg            *config.Config
	client         *api.Client
	bot            *telegram.Bot
	detecting      bool
	sending        bool
	paused         bool
	sendPaused     bool // auto-paused by circuit breaker
	sendFailCount   int    // consecutive send cycle failures
	setupPopulating string            // table currently being populated during setup ("" = idle)
	setupPopulated  map[string]bool   // tables that have been populated during this setup session (even if 0 records)
	stopCh          chan bool
	tokens         map[string]TokenInfo
	tokenMu        gosync.RWMutex
	startTime      time.Time
	apiLimiter     *rateLimiter
	loginLimiter   *rateLimiter
	syncRegistry   map[string]*SyncTableDef // ORM-based sync table definitions
	fileWatcher    *fileWatcher              // fsnotify watcher for ISAM file changes
	watcherCh      chan []string              // channel receiving changed table names from watcher
}

func main() {
	// In production, work from the directory where the .exe is located
	// (important when launched from shortcuts, Start Menu, etc.)
	// Skip in dev mode (Air puts the exe in a temp dir)
	if os.Getenv("AIR_ENV") == "" && os.Getenv("DEV") == "" {
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			// Only chdir if the exe dir has a config.json or looks like an install dir
			if _, statErr := os.Stat(filepath.Join(exeDir, "config.json")); statErr == nil {
				os.Chdir(exeDir)
			}
		}
	}

	db, err := storage.NewDB("siigo_web.db")
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	defer db.Close()

	cfg, err := config.Load("config.json")
	if err != nil {
		cfg = config.Default()
		cfg.Save("config.json")
		log.Println("Config not found, created default")
	}

	if cfg.Auth.Password == "change-me" {
		log.Println("[SECURITY] Default password detected — please change it from the web UI")
		db.AddLog("warning", "SECURITY", "Password por defecto activo. Cambielo desde Configuracion.")
	}

	bot := telegram.New(&cfg.Telegram)

	srv := &Server{
		db:         db,
		cfg:        cfg,
		bot:        bot,
		stopCh:     make(chan bool, 1),
		tokens:         make(map[string]TokenInfo),
		setupPopulated: make(map[string]bool),
		startTime:  time.Now(),
		apiLimiter:   newRateLimiter(120, time.Minute), // 120 req/min per IP
		loginLimiter: newRateLimiter(5, time.Minute),   // 5 login attempts/min per IP
	}

	// Restore setup state from DB (crash recovery)
	for _, t := range setupTablesList {
		if db.GetKV("setup_populated_"+t.Name) == "true" {
			srv.setupPopulated[t.Name] = true
		}
	}

	// Initialize ORM sync registry if data path is configured
	if srv.cfg.Siigo.DataPath != "" {
		srv.syncRegistry = initSyncTables(srv.cfg.Siigo.DataPath)
	}

	// Periodic cleanup
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			srv.apiLimiter.cleanup()
			srv.loginLimiter.cleanup()
		}
	}()
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			srv.db.CleanOldSyncStats(72) // Keep 3 days of stats
		}
	}()

	db.AddLog("info", "APP", "Web server started")

	if bot.IsEnabled() {
		srv.registerBotCommands()
		bot.StartPolling()
	} else {
		log.Println("[Telegram] Bot disabled — skipping polling")
	}

	// Start auto-updater (checks GitHub releases at 2 AM daily)
	srv.startAutoUpdater()

	// Auto-install cloudflared, start quick tunnel, and run watchdog
	go func() {
		time.Sleep(2 * time.Second) // wait for server to fully start
		srv.startQuickTunnel()
		srv.startTunnelWatchdog()
	}()

	if srv.cfg.SetupComplete {
		go srv.startSyncLoop()
	} else {
		db.AddLog("info", "APP", "Setup pending — sync loops disabled until wizard is completed")
	}

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// Serve React frontend — prefer disk (dev), fallback to embedded (production)
	frontendDir := "frontend/dist"
	var frontendHandler http.Handler
	if _, err := os.Stat(frontendDir); err == nil {
		// Development: serve from disk (supports Vite proxy / hot reload)
		diskFS := http.FileServer(http.Dir(frontendDir))
		frontendHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cleanPath := filepath.Clean(r.URL.Path)
			if strings.Contains(cleanPath, "..") {
				http.NotFound(w, r)
				return
			}
			fullPath := filepath.Join(frontendDir, cleanPath)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) && r.URL.Path != "/" {
				http.ServeFile(w, r, filepath.Join(frontendDir, "index.html"))
				return
			}
			diskFS.ServeHTTP(w, r)
		})
	} else if subFS, fsErr := fs.Sub(embeddedFrontend, "frontend/dist"); fsErr == nil {
		// Production: serve from embedded filesystem (single .exe)
		embFS := http.FS(subFS)
		embServer := http.FileServer(embFS)
		frontendHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			urlPath := r.URL.Path
			if strings.Contains(urlPath, "..") {
				http.NotFound(w, r)
				return
			}
			// Use forward slashes for embed FS (not filepath.Clean which uses backslash on Windows)
			cleanPath := strings.TrimPrefix(urlPath, "/")
			if cleanPath == "" {
				cleanPath = "index.html"
			}
			// Try to open the file; if not found, serve index.html (SPA routing)
			f, err := embFS.Open(cleanPath)
			if err != nil {
				// SPA fallback: serve index.html for client-side routing
				r.URL.Path = "/"
				embServer.ServeHTTP(w, r)
				return
			}
			f.Close()
			embServer.ServeHTTP(w, r)
		})
	}

	if frontendHandler != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			frontendHandler.ServeHTTP(w, r)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, "Frontend not available. Run 'npm run build' in frontend/")
		})
	}

	port := "3210"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	// Config-based port (env var overrides)
	if cfg.Server.Port != "" && os.Getenv("PORT") == "" {
		port = cfg.Server.Port
	}

	// Kill any existing instance on this port
	killExistingInstance(port)

	localURL := fmt.Sprintf("http://localhost:%s", port)
	lanURL := ""
	if ip := getLocalIP(); ip != "" {
		lanURL = fmt.Sprintf("http://%s:%s", ip, port)
	}
	log.Printf("Siigo Web running on %s", localURL)
	if lanURL != "" {
		log.Printf("  LAN: %s", lanURL)
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: corsMiddleware(mux),
	}

	// Graceful shutdown helper
	shutdownServer := func() {
		log.Println("Shutdown signal received, stopping...")
		db.AddLog("info", "APP", "Server stopped (graceful shutdown)")

		// Stop sync loops
		select {
		case srv.stopCh <- true:
		default:
		}
		select {
		case srv.stopCh <- true:
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	}

	// Handle OS signals (Ctrl+C, taskkill, etc.)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		<-sigCh
		shutdownServer()
		systray.Quit()
	}()

	// Start HTTP server in background
	go func() {
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Notify Telegram with tunnel URL if available (retry up to 30s since tunnel can be slow)
	go func() {
		var tunnelURL string
		for i := 0; i < 10; i++ {
			time.Sleep(3 * time.Second)
			tunnelURL = readTunnelURL()
			if tunnelURL != "" {
				break
			}
		}
		msg := fmt.Sprintf("🟢 <b>Server started</b>\n\n🖥 Local: %s", localURL)
		if lanURL != "" {
			msg += fmt.Sprintf("\n🏠 LAN: %s", lanURL)
		}
		if tunnelURL != "" {
			msg += fmt.Sprintf("\n🌍 Public: %s\n📄 Swagger: %s/api/v1/docs", tunnelURL, tunnelURL)
		}
		bot.Send(msg)
	}()

	// System tray (blocks main thread — this IS the main loop)
	systray.Run(func() {
		// onReady
		systray.SetIcon(trayIconData)
		systray.SetTitle("Siigo Web")
		systray.SetTooltip(fmt.Sprintf("Siigo Web — %s", localURL))

		mOpen := systray.AddMenuItem("Ver interfaz", "Abrir panel web en el navegador")
		systray.AddSeparator()
		mRestart := systray.AddMenuItem("Reiniciar programa", "Reiniciar el servidor")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Cerrar", "Cerrar Siigo Web")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(localURL)
				case <-mRestart.ClickedCh:
					log.Println("Restarting server...")
					db.AddLog("info", "APP", "Server restarted from tray")
					// Restart the executable
					exe, _ := os.Executable()
					cmd := exec.Command(exe)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					cmd.Start()
					// Shutdown current instance (new one will kill us via port check)
					shutdownServer()
					systray.Quit()
				case <-mQuit.ClickedCh:
					shutdownServer()
					systray.Quit()
				}
			}
		}()
	}, func() {
		// onExit
		log.Println("Tray exited, server stopped")
	})
}

// killExistingInstance checks if the port is already in use and kills the process occupying it.
func killExistingInstance(port string) {
	// Try to listen briefly to check if port is free
	ln, err := net.Listen("tcp", ":"+port)
	if err == nil {
		// Port is free, nothing to kill
		ln.Close()
		return
	}

	log.Printf("Port %s is in use, attempting to kill existing instance...", port)

	if runtime.GOOS == "windows" {
		// Use netstat to find the PID using this port
		out, err := exec.Command("cmd", "/c", "netstat", "-ano").Output()
		if err != nil {
			log.Printf("Could not run netstat: %v", err)
			return
		}

		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			// Look for LISTENING on our port
			if strings.Contains(line, ":"+port) && strings.Contains(line, "LISTENING") {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) >= 5 {
					pid := fields[len(fields)-1]
					if pid != "0" {
						log.Printf("Killing process PID %s on port %s", pid, port)
						killCmd := exec.Command("taskkill", "/F", "/PID", pid)
						if killErr := killCmd.Run(); killErr != nil {
							log.Printf("Could not kill PID %s: %v", pid, killErr)
						} else {
							log.Printf("Killed previous instance (PID %s)", pid)
							time.Sleep(500 * time.Millisecond) // Wait for port to be released
						}
					}
				}
				break
			}
		}
	} else {
		// Unix: use lsof or fuser
		out, err := exec.Command("lsof", "-ti", ":"+port).Output()
		if err == nil {
			pid := strings.TrimSpace(string(out))
			if pid != "" {
				log.Printf("Killing process PID %s on port %s", pid, port)
				killCmd := exec.Command("kill", "-9", pid)
				if killErr := killCmd.Run(); killErr != nil {
					log.Printf("Could not kill PID %s: %v", pid, killErr)
				} else {
					log.Printf("Killed previous instance (PID %s)", pid)
					time.Sleep(500 * time.Millisecond)
				}
			}
		}
	}
}

// getLocalIP returns the local LAN IP address (e.g. 192.168.x.x).
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}

// openBrowser opens the given URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Could not open browser: %v", err)
	}
}

// readTunnelURL reads the Cloudflare tunnel URL from the log file.
// Tries multiple paths because MSYS /tmp maps to user temp dir on Windows.
func readTunnelURL() string {
	paths := []string{
		os.Getenv("TEMP") + "\\cloudflared.log",
		os.Getenv("TMP") + "\\cloudflared.log",
		os.Getenv("USERPROFILE") + "\\AppData\\Local\\Temp\\cloudflared.log",
		"C:\\tmp\\cloudflared.log",
		"/tmp/cloudflared.log",
	}
	for _, p := range paths {
		if p == "\\cloudflared.log" || p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if idx := strings.Index(line, "https://"); idx >= 0 {
				end := strings.IndexAny(line[idx:], " \n\r")
				tunnelURL := line[idx:]
				if end > 0 {
					tunnelURL = line[idx : idx+end]
				}
				if strings.Contains(tunnelURL, "trycloudflare.com") {
					return tunnelURL
				}
			}
		}
	}
	return ""
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:3210":  true,
		"http://localhost:5173":  true, // Vite dev
		"http://127.0.0.1:3210": true,
		"http://127.0.0.1:5173": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] || strings.HasSuffix(origin, ".trycloudflare.com") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ==================== AUTH ====================

func (s *Server) generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) isValidToken(token string) bool {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	info, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(info.Expiry) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *Server) getTokenInfo(token string) *TokenInfo {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	info, ok := s.tokens[token]
	if !ok {
		return nil
	}
	return &info
}

func (s *Server) getUsernameFromRequest(r *http.Request) string {
	token := extractToken(r)
	info := s.getTokenInfo(token)
	if info != nil {
		return info.Username
	}
	return "unknown"
}

// AllModules lists all permission-controlled module keys
var AllModules = []string{
	"dashboard", "clients", "products", "cartera",
	"documentos",
	"field-mappings", "errors", "logs", "explorer", "config", "users",
}

// apiPathToModule maps API endpoint paths to their required module permission.
// Routes not listed here are accessible to any authenticated user.
var apiPathToModule = map[string]string{
	"/api/clients":              "clients",
	"/api/products":             "products",
	"/api/cartera":              "cartera",
	"/api/documentos":           "documentos",
	"/api/condiciones-pago":         "condiciones_pago",
	"/api/codigos-dane":             "codigos_dane",
	"/api/formulas":                "formulas",
	"/api/vendedores-areas":        "vendedores_areas",
	"/api/field-mappings":          "field-mappings",
	"/api/error-summary":        "errors",
	"/api/logs":                 "logs",
	"/api/export-logs":          "logs",
	"/api/query":                "explorer",
	"/api/config":               "config",
	"/api/telegram-config":      "config",
	"/api/telegram-test":        "config",
	"/api/public-api-config":    "config",
	"/api/webhook-config":       "config",
	"/api/webhook-test":         "config",
	"/api/clear-database":       "config",
	"/api/clear-logs":           "config",
	"/api/users":                "users",
	"/api/users/":               "users",
}

// hasModulePermission checks if a token has access to a specific module
func hasModulePermission(info *TokenInfo, module string) bool {
	if info.Role == "root" || info.Role == "admin" {
		return true
	}
	for _, p := range info.Permissions {
		if p == module {
			return true
		}
	}
	return false
}

// permMiddleware wraps authMiddleware and additionally checks module permissions
func (s *Server) permMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		module, needsPerm := apiPathToModule[path]
		if !needsPerm {
			next(w, r)
			return
		}
		token := extractToken(r)
		info := s.getTokenInfo(token)
		if info == nil {
			jsonError(w, "Unauthorized", 401)
			return
		}
		if !hasModulePermission(info, module) {
			jsonError(w, "No permission for this module", 403)
			return
		}
		next(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	// Rate limit login attempts by IP
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	if !s.loginLimiter.Allow(strings.TrimSpace(ip)) {
		w.Header().Set("Retry-After", "60")
		jsonError(w, "Too many attempts. Try again in 1 minute.", 429)
		s.bot.NotifyLoginFailed("rate-limited", ip)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request", 400)
		return
	}

	var tokenInfo TokenInfo

	// Check root user first (from config.json)
	if body.Username == s.cfg.Auth.Username && body.Password == s.cfg.Auth.Password {
		tokenInfo = TokenInfo{
			Expiry:      time.Now().Add(24 * time.Hour),
			Username:    body.Username,
			Role:        "root",
			Permissions: AllModules,
		}
	} else {
		// Check app_users table
		user, err := s.db.GetAppUser(body.Username)
		if err != nil || !s.db.CheckAppUserPassword(user, body.Password) || !user.Active {
			jsonError(w, "Invalid credentials", 401)
			return
		}
		perms := user.Permissions
		if user.Role == "admin" {
			perms = AllModules
		}
		tokenInfo = TokenInfo{
			Expiry:      time.Now().Add(24 * time.Hour),
			Username:    user.Username,
			Role:        user.Role,
			Permissions: perms,
		}
	}

	token := s.generateToken()
	s.tokenMu.Lock()
	s.tokens[token] = tokenInfo
	s.tokenMu.Unlock()

	jsonResponse(w, map[string]interface{}{
		"token":            token,
		"username":         tokenInfo.Username,
		"role":             tokenInfo.Role,
		"permissions":      tokenInfo.Permissions,
		"setup_complete":   s.cfg.SetupComplete,
		"default_password": s.cfg.Auth.Password == "change-me",
	})
}

func (s *Server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if !s.isValidToken(token) {
		jsonError(w, "Unauthorized", 401)
		return
	}
	info := s.getTokenInfo(token)
	if info == nil {
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	jsonResponse(w, map[string]interface{}{
		"status":         "ok",
		"username":       info.Username,
		"role":           info.Role,
		"permissions":    info.Permissions,
		"setup_complete": s.cfg.SetupComplete,
	})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	// Fallback: token in query param (for download links that can't set headers)
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if !s.isValidToken(token) {
			jsonError(w, "Unauthorized", 401)
			return
		}
		next(w, r)
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Public routes (no auth)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/check-auth", s.handleCheckAuth)
	mux.HandleFunc("/health", s.handleHealth)

	// Protected routes (permMiddleware checks module permissions where mapped)
	mux.HandleFunc("/api/stats", s.authMiddleware(s.handleStats))
	mux.HandleFunc("/api/config", s.permMiddleware(s.handleConfig))
	mux.HandleFunc("/api/validate-path", s.permMiddleware(s.handleValidatePath))
	mux.HandleFunc("/api/isam-info", s.authMiddleware(s.handleISAMInfo))
	mux.HandleFunc("/api/extfh-status", s.authMiddleware(s.handleExtfhStatus))
	mux.HandleFunc("/api/clients", s.permMiddleware(s.handleClients))
	mux.HandleFunc("/api/products", s.permMiddleware(s.handleProducts))
	mux.HandleFunc("/api/cartera", s.permMiddleware(s.handleCartera))
	mux.HandleFunc("/api/documentos", s.permMiddleware(s.handleDocumentos))
	mux.HandleFunc("/api/condiciones-pago", s.permMiddleware(s.handleGenericTable("condiciones_pago")))
	mux.HandleFunc("/api/codigos-dane", s.permMiddleware(s.handleGenericTable("codigos_dane")))
	mux.HandleFunc("/api/formulas", s.permMiddleware(s.handleGenericTable("formulas")))
	mux.HandleFunc("/api/vendedores-areas", s.permMiddleware(s.handleGenericTable("vendedores_areas")))

	// Relationship views
	mux.HandleFunc("/api/cliente-productos", s.authMiddleware(s.handleClienteProductos))
	mux.HandleFunc("/api/producto-receta", s.authMiddleware(s.handleProductoReceta))
	mux.HandleFunc("/api/cliente-producto-receta", s.authMiddleware(s.handleClienteProductoReceta))

	mux.HandleFunc("/api/sync-history", s.authMiddleware(s.handleSyncHistory))
	mux.HandleFunc("/api/logs", s.permMiddleware(s.handleLogs))
	mux.HandleFunc("/api/sync-now", s.authMiddleware(s.handleSyncNow))
	mux.HandleFunc("/api/pause", s.authMiddleware(s.handlePause))
	mux.HandleFunc("/api/resume", s.authMiddleware(s.handleResume))
	mux.HandleFunc("/api/sync-status", s.authMiddleware(s.handleSyncStatus))
	mux.HandleFunc("/api/send-resume", s.authMiddleware(s.handleSendResume))
	mux.HandleFunc("/api/retry-errors", s.authMiddleware(s.handleRetryErrors))
	mux.HandleFunc("/api/test-connection", s.authMiddleware(s.handleTestConnection))
	mux.HandleFunc("/api/clear-database", s.permMiddleware(s.handleClearDatabase))
	mux.HandleFunc("/api/clear-logs", s.permMiddleware(s.handleClearLogs))
	mux.HandleFunc("/api/refresh-cache", s.authMiddleware(s.handleRefreshCache))
	mux.HandleFunc("/api/field-mappings", s.permMiddleware(s.handleFieldMappings))
	mux.HandleFunc("/api/send-enabled", s.authMiddleware(s.handleSendEnabled))
	mux.HandleFunc("/api/global-send", s.permMiddleware(s.handleGlobalSend))
	mux.HandleFunc("/api/detect-enabled", s.authMiddleware(s.handleDetectEnabled))
	mux.HandleFunc("/api/error-summary", s.permMiddleware(s.handleErrorSummary))
	mux.HandleFunc("/api/export-history", s.authMiddleware(s.handleExportHistory))
	mux.HandleFunc("/api/export-logs", s.permMiddleware(s.handleExportLogs))
	mux.HandleFunc("/api/public-api-config", s.permMiddleware(s.handlePublicAPIConfig))
	mux.HandleFunc("/api/telegram-config", s.permMiddleware(s.handleTelegramConfig))
	mux.HandleFunc("/api/telegram-test", s.permMiddleware(s.handleTelegramTest))
	mux.HandleFunc("/api/query", s.permMiddleware(s.handleQuery))
	mux.HandleFunc("/api/allow-edit-delete", s.authMiddleware(s.handleAllowEditDelete))
	mux.HandleFunc("/api/record", s.authMiddleware(s.handleRecord))
	mux.HandleFunc("/api/users", s.permMiddleware(s.handleUsers))
	mux.HandleFunc("/api/users/", s.permMiddleware(s.handleUserByID))
	mux.HandleFunc("/api/audit-trail", s.authMiddleware(s.handleAuditTrail))
	mux.HandleFunc("/api/change-history", s.authMiddleware(s.handleChangeHistory))
	mux.HandleFunc("/api/sync-stats-history", s.authMiddleware(s.handleSyncStatsHistory))
	mux.HandleFunc("/api/backup", s.authMiddleware(s.handleBackup))
	mux.HandleFunc("/api/restore", s.authMiddleware(s.handleRestore))
	mux.HandleFunc("/api/events", s.authMiddleware(s.handleSSE))
	mux.HandleFunc("/api/bulk-action", s.authMiddleware(s.handleBulkAction))
	mux.HandleFunc("/api/webhook-config", s.permMiddleware(s.handleWebhookConfig))
	mux.HandleFunc("/api/webhook-test", s.permMiddleware(s.handleWebhookTest))
	mux.HandleFunc("/api/server-info", s.authMiddleware(s.handleServerInfo))
	mux.HandleFunc("/api/user-prefs", s.authMiddleware(s.handleUserPrefs))
	mux.HandleFunc("/api/check-update", s.authMiddleware(s.handleCheckUpdate))
	mux.HandleFunc("/api/apply-update", s.authMiddleware(s.handleApplyUpdate))
	mux.HandleFunc("/api/restart", s.authMiddleware(s.handleRestart))
	mux.HandleFunc("/api/exec", s.authMiddleware(s.handleExec))
	mux.HandleFunc("/api/tunnel/status", s.authMiddleware(s.handleTunnelStatus))
	mux.HandleFunc("/api/tunnel/install", s.authMiddleware(s.handleTunnelInstall))
	mux.HandleFunc("/api/tunnel/start", s.authMiddleware(s.handleTunnelStart))
	mux.HandleFunc("/api/tunnel/stop", s.authMiddleware(s.handleTunnelStop))

	// Cartera por cliente
	mux.HandleFunc("/api/cartera-cliente/", s.authMiddleware(s.handleCarteraCliente))

	// Ventas por cliente/producto
	mux.HandleFunc("/api/ventas/", s.authMiddleware(s.handleVentas))
	mux.HandleFunc("/api/recaudo/", s.authMiddleware(s.handleRecaudo))

	// Setup wizard
	mux.HandleFunc("/api/setup-status", s.authMiddleware(s.handleSetupStatus))
	mux.HandleFunc("/api/setup-populate", s.authMiddleware(s.handleSetupPopulate))
	mux.HandleFunc("/api/setup-complete", s.authMiddleware(s.handleSetupComplete))

	// Swagger docs
	mux.HandleFunc("/api/v1/docs", handleSwaggerUI)
	mux.HandleFunc("/api/v1/swagger.json", handleSwaggerJSON)
	mux.HandleFunc("/api/v1/postman", s.handlePostmanCollection)

	// Public API v1 (JWT auth)
	mux.HandleFunc("/api/v1/auth", s.handleV1Auth)
	mux.HandleFunc("/api/v1/stats", s.jwtMiddleware(s.handleV1Stats))
	// Register all data tables for v1 API
	for _, table := range odataTableOrder {
		mux.HandleFunc("/api/v1/"+table, s.jwtMiddleware(s.handleV1List(table)))
		mux.HandleFunc("/api/v1/"+table+"/", s.jwtMiddleware(s.handleV1Detail(table)))
	}

	// OData endpoints (JWT protected)
	mux.HandleFunc("/odata", s.jwtMiddleware(s.handleODataServiceDoc))
	mux.HandleFunc("/odata/", s.jwtMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/odata/")

		// $metadata
		if path == "$metadata" {
			s.handleODataMetadata(w, r)
			return
		}

		// Route to collection or entity
		for _, table := range odataTableOrder {
			if path == table || path == table+"/" {
				s.handleODataCollection(table)(w, r)
				return
			}
			if path == table+"/$count" {
				s.handleODataCollection(table)(w, r)
				return
			}
			if strings.HasPrefix(path, table+"/") || strings.HasPrefix(path, table+"(") {
				s.handleODataEntity(table)(w, r)
				return
			}
		}

		jsonError(w, "OData resource not found", 404)
	}))
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ==================== ORM SYNC TABLE REGISTRY ====================

// SyncTableDef describes how to map an ISAM ORM model to a SQLite sync table.
type SyncTableDef struct {
	Table    string       // SQLite table name (e.g. "clients")
	Model    *isam.Model  // ORM model (e.g. isam.TercerosAmpliados)
	Label    string       // Human-readable label for logs
	KeyCol   string       // SQLite key column name (e.g. "nit", "record_key")
	// KeyFunc generates the SQLite key from an ORM row. If nil, uses the ORM primary key.
	KeyFunc  func(r *isam.Row) string
	// ColMap maps SQLite column name → ORM field name.
	// BCD/float fields use the prefix "~" to indicate GetFloat (e.g. "~saldo_anterior").
	ColMap   map[string]string
	// BoolMap maps SQLite column → ORM field for boolean S/N → 0/1 conversion.
	BoolMap  map[string]string
	// ComputedCols generates additional SQLite columns from a row (e.g. saldo_final = prev + debit - credit).
	ComputedCols func(r *isam.Row) map[string]interface{}
	// PostDetect runs after diffGeneric completes for this table (e.g. enrich clients with audit trail data).
	PostDetect func(s *Server)
	// MultiYear: if true, read ALL year-based files (e.g. Z092024 + Z092025 + Z092026) instead of just the latest.
	MultiYear bool
}

// initSyncTables initializes the ORM models and returns the sync table registry.
// Must be called after config is loaded (needs DataPath).
func initSyncTables(dataPath string) map[string]*SyncTableDef {
	if dataPath == "" {
		log.Println("[WARN] initSyncTables: DataPath is empty, skipping ORM initialization")
		return nil
	}
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		log.Printf("[WARN] initSyncTables: DataPath %q does not exist, skipping ORM initialization", dataPath)
		return nil
	}

	// Find the latest year from Z04 (most common year-based file)
	_, latestYear := parsers.FindLatestZ04(dataPath)
	if latestYear == "" {
		latestYear = "2016" // fallback
	}

	// Connect all 24 ORM models and enable caching
	isam.ConnectAll(dataPath, latestYear)

	// Enable 30s read cache on all models (avoids re-reading ISAM files within a detect cycle)
	for _, m := range []*isam.Model{
		isam.Clients, isam.Products, isam.Movements, isam.Cartera,
		isam.Maestros, isam.PlanCuentas, isam.ActivosFijos, isam.Documentos,
		isam.TercerosAmpliados, isam.SaldosTerceros, isam.SaldosConsolidados,
		isam.CodigosDane, isam.Historial, isam.ActividadesICA, isam.ConceptosPILA,
		isam.LibrosAuxiliares, isam.TransaccionesDetalle, isam.PeriodosContables,
		isam.CondicionesPago, isam.MovimientosInventario, isam.SaldosInventario,
		isam.ClasificacionCuentas, isam.ActivosFijosDetalle, isam.AuditTrailTerceros,
		isam.Formulas, isam.DocsInventario, isam.VendedoresAreas,
	} {
		m.EnableCache(30 * time.Second)
	}

	registry := map[string]*SyncTableDef{
		"clients": {
			Table: "clients", Model: isam.TercerosAmpliados, Label: "Clients", KeyCol: "nit",
			KeyFunc: func(r *isam.Row) string {
				nit := strings.TrimSpace(r.Get("nit"))
				// Strip leading zeros to normalize NIT (0860048867 → 860048867)
				for len(nit) > 1 && nit[0] == '0' {
					nit = nit[1:]
				}
				return nit
			},
			ColMap: map[string]string{
				"nombre": "nombre", "empresa": "empresa", "tipo_persona": "tipo_persona",
				"direccion": "direccion", "email": "email",
				"dv": "dv",
			},
			PostDetect: func(s *Server) {
				// Enrich clients with audit trail data (Z11N: tipo_doc, rep. legal, latest changes)
				n, err := s.db.EnrichClientsFromAuditTrail()
				if err != nil {
					s.db.AddLog("warn", "CLIENTS", "Enrichment from audit trail failed: "+err.Error())
				} else if n > 0 {
					s.db.AddLog("info", "CLIENTS", fmt.Sprintf("Enriched %d clients from audit trail (Z11N)", n))
				}
			},
		},
		"products": {
			Table: "products", Model: isam.Products, Label: "Products", KeyCol: "code",
			KeyFunc: func(r *isam.Row) string { return r.Get("codigo") },
			ColMap: map[string]string{
				"nombre": "nombre", "nombre_corto": "nombre_corto", "grupo": "grupo",
				"referencia": "referencia", "empresa": "empresa",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				nombre := r.Get("nombre")
				codPlat := ""
				if idx := strings.Index(nombre, "-"); idx >= 0 {
					after := strings.TrimSpace(nombre[idx+1:])
					re := regexp.MustCompile(`^(\d+)`)
					if m := re.FindString(after); m != "" {
						codPlat = m
					}
				}
				return map[string]interface{}{
					"codigo_plataforma": codPlat,
				}
			},
		},
		"cartera": {
			Table: "cartera", Model: isam.Cartera, Label: "Cartera", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				e := strings.TrimSpace(r.Get("empresa"))
				numDoc := int(r.GetFloat("num_documento"))
				seq := strings.TrimSpace(r.Get("seq"))
				return fmt.Sprintf("%s-%s-%d-%s-%s", t, e, numDoc, seq, r.Hash()[:8])
			},
			ColMap: map[string]string{
				"tipo_registro": "tipo", "empresa": "empresa", "secuencia": "seq",
				"tipo_doc": "tipo_doc", "nit_tercero": "nit", "cuenta_contable": "cuenta",
				"fecha": "fecha", "codigo_producto": "codigo_producto",
				"descripcion": "descripcion", "tipo_mov": "dc",
				"~valor": "valor",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				codProd := strings.TrimSpace(r.Get("codigo_producto"))
				codPlat := ""
				if len(codProd) > 1 && codProd != "0000000" {
					codPlat = strings.TrimLeft(codProd[1:], "0")
				}
				// Build document reference: tipo-comprobante-numero (e.g. F-003-17248)
				numDoc := int(r.GetFloat("num_documento"))
				tipoReg := strings.TrimSpace(r.Get("tipo"))
				empresa := strings.TrimSpace(r.Get("empresa"))
				docRef := ""
				if numDoc > 0 {
					docRef = fmt.Sprintf("%s-%s-%d", tipoReg, empresa, numDoc)
				}
				return map[string]interface{}{
					"codigo_plataforma": codPlat,
					"num_documento":     numDoc,
					"documento_ref":     docRef,
				}
			},
		},
		"documentos": {
			Table: "documentos", Model: isam.Documentos, Label: "Documentos", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				c := strings.TrimSpace(r.Get("codigo"))
				s := strings.TrimSpace(r.Get("seq"))
				return t + "-" + c + "-" + s + "-" + r.Hash()[:8]
			},
			ColMap: map[string]string{
				"tipo_comprobante": "tipo", "codigo_comp": "codigo", "secuencia": "seq",
				"nit_tercero": "nit", "cuenta_contable": "cuenta", "producto_ref": "producto",
				"bodega": "bodega", "centro_costo": "cc", "fecha": "fecha",
				"descripcion": "descripcion", "tipo_mov": "dc", "referencia": "referencia",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				// Extract real NIT from cuenta: cuenta[1:11] stripped of leading zeros
				cuenta := r.Get("cuenta")
				nitReal := ""
				if len(cuenta) >= 11 {
					nitReal = strings.TrimLeft(cuenta[1:11], "0")
				}
				// BCD valor /100 for real pesos
				valor := r.GetFloat("valor") / 100
				return map[string]interface{}{
					"nit_cliente": nitReal,
					"valor":       valor,
				}
			},
		},
		"terceros_ampliados": {
			Table: "terceros_ampliados", Model: isam.TercerosAmpliados, Label: "Terceros Ampliados", KeyCol: "nit",
			KeyFunc: func(r *isam.Row) string {
				nit := strings.TrimSpace(r.Get("nit"))
				for len(nit) > 1 && nit[0] == '0' {
					nit = nit[1:]
				}
				return nit
			},
			ColMap: map[string]string{
				"nombre": "nombre", "empresa": "empresa", "tipo_persona": "tipo_persona",
				"direccion": "direccion", "email": "email",
				"dv": "dv",
			},
			PostDetect: func(s *Server) {
				// When terceros_ampliados changes, also enrich clients (same Z08A source)
				n, err := s.db.EnrichClientsFromAuditTrail()
				if err != nil {
					s.db.AddLog("warn", "CLIENTS", "Enrichment from audit trail failed: "+err.Error())
				} else if n > 0 {
					s.db.AddLog("info", "CLIENTS", fmt.Sprintf("Re-enriched %d clients from terceros_ampliados update", n))
				}
			},
		},
		"condiciones_pago": {
			Table: "condiciones_pago", Model: isam.CondicionesPago, Label: "Condiciones Pago", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				e := strings.TrimSpace(r.Get("empresa"))
				s := strings.TrimSpace(r.Get("seq"))
				n := strings.TrimSpace(r.Get("nit"))
				k := t + "-" + e + "-" + s + "-" + n
				if k == "---" { return r.Hash() }
				return k
			},
			ColMap: map[string]string{
				"tipo": "tipo", "empresa": "empresa", "secuencia": "seq",
				"tipo_doc": "tipo_doc", "fecha": "fecha", "nit": "nit",
				"tipo_secundario": "tipo_sec", "~valor": "valor", "fecha_registro": "fecha_reg",
			},
		},
		"codigos_dane": {
			Table: "codigos_dane", Model: isam.CodigosDane, Label: "Codigos DANE", KeyCol: "codigo",
			KeyFunc: func(r *isam.Row) string { return r.Get("codigo") },
			ColMap: map[string]string{
				"nombre": "nombre",
			},
		},
		"audit_trail_terceros": {
			Table: "audit_trail_terceros", Model: isam.AuditTrailTerceros, Label: "Audit Trail Terceros", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				n := strings.TrimSpace(r.Get("nit"))
				t := strings.TrimSpace(r.Get("timestamp"))
				k := n + "-" + t
				if k == "-" { return r.Hash() }
				return k
			},
			ColMap: map[string]string{
				"fecha_cambio": "fecha_cambio", "nit_tercero": "nit",
				"timestamp": "timestamp", "usuario": "usuario",
				"fecha_periodo": "fecha_periodo", "tipo_doc": "tipo_doc",
				"nombre": "nombre", "nit_representante": "nit_rep",
				"nombre_representante": "nom_rep",
				"direccion": "direccion", "email": "email",
			},
			PostDetect: func(s *Server) {
				// When audit trail changes, re-enrich clients
				n, err := s.db.EnrichClientsFromAuditTrail()
				if err != nil {
					s.db.AddLog("warn", "CLIENTS", "Enrichment from audit trail failed: "+err.Error())
				} else if n > 0 {
					s.db.AddLog("info", "CLIENTS", fmt.Sprintf("Re-enriched %d clients from audit trail (Z11N update)", n))
				}
			},
		},
		"formulas": {
			Table: "formulas", Model: isam.Formulas, Label: "Formulas", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				emp := strings.TrimSpace(r.Get("empresa"))
				prod := strings.TrimSpace(r.Get("codigo_producto"))
				ing := strings.TrimSpace(r.Get("codigo_ingrediente"))
				k := emp + "-" + prod + "-" + ing
				if k == "--" { return "" }
				return k
			},
			ColMap: map[string]string{
				"empresa": "empresa", "grupo_producto": "grupo_producto",
				"codigo_producto": "codigo_producto", "grupo_ingrediente": "grupo_ingrediente",
				"codigo_ingrediente": "codigo_ingrediente",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				raw := strings.TrimSpace(r.Get("porcentaje"))
				val, _ := strconv.ParseFloat(raw, 64)
				return map[string]interface{}{"porcentaje": val / 1000.0}
			},
		},
		"vendedores_areas": {
			Table: "vendedores_areas", Model: isam.VendedoresAreas, Label: "Vendedores/Areas", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				c := strings.TrimSpace(r.Get("codigo"))
				k := t + "-" + c
				if k == "-" {
					return ""
				}
				return k
			},
			ColMap: map[string]string{
				"tipo": "tipo", "codigo": "codigo", "nombre": "nombre",
				"nombre_corto": "nombre_corto", "ciudad": "ciudad",
				"nit": "nit", "direccion": "direccion", "email": "email",
			},
		},
		"notas_documentos": {
			Table: "notas_documentos", Model: isam.Movements, Label: "Notas Documentos", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				c := strings.TrimSpace(r.Get("codigo"))
				n := strings.TrimSpace(r.Get("num_doc"))
				k := t + "-" + c + "-" + n
				if k == "--" {
					return ""
				}
				return k
			},
			ColMap: map[string]string{
				"tipo": "tipo", "codigo_doc": "codigo", "num_documento": "num_doc",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				texto := strings.TrimSpace(r.Get("texto"))
				cols := map[string]interface{}{
					"texto": texto,
				}
				// Parse embedded fields from free-form text
				if lote := extractField(texto, `LOTE:(\S+)`); lote != "" {
					cols["lote"] = lote
				}
				if oc := extractField(texto, `ORDEN DE COMPRA:([^/]+)`); oc != "" {
					cols["orden_compra"] = strings.TrimSpace(oc)
				}
				if fecha := extractField(texto, `FECHA DESPACHO:([^/]+)`); fecha != "" {
					cols["fecha_despacho"] = strings.TrimSpace(fecha)
				}
				if empaque := extractField(texto, `EMPAQUE\s+([^/]+)`); empaque != "" {
					cols["empaque"] = strings.TrimSpace(empaque)
				}
				if obs := extractField(texto, `OBSERVACIONES:(.+)`); obs != "" {
					cols["observaciones"] = strings.TrimSpace(obs)
				}
				return cols
			},
		},
		"facturas_electronicas": {
			Table: "facturas_electronicas", Model: isam.FacturasElectronicas, Label: "Facturas Electronicas", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				e := strings.TrimSpace(r.Get("empresa"))
				s := strings.TrimSpace(r.Get("seq"))
				k := t + "-" + e + "-" + s
				if k == "--" {
					return ""
				}
				return k
			},
			ColMap: map[string]string{
				"tipo": "tipo", "empresa": "empresa", "secuencia": "seq",
				"nit_tercero": "nit", "fecha": "fecha",
				"descripcion": "descripcion", "tipo_mov": "dc",
				"vendedor": "vendedor",
			},
			ComputedCols: func(r *isam.Row) map[string]interface{} {
				valor := r.GetFloat("valor") / 100
				return map[string]interface{}{
					"valor": valor,
				}
			},
		},
		"detalle_movimientos": {
			Table: "detalle_movimientos", Model: isam.DetalleMovimientos, Label: "Detalle Movimientos", KeyCol: "record_key",
			KeyFunc: func(r *isam.Row) string {
				t := strings.TrimSpace(r.Get("tipo"))
				e := strings.TrimSpace(r.Get("empresa"))
				p := strings.TrimSpace(r.Get("codigo_producto"))
				l := strings.TrimSpace(r.Get("linea"))
				tc := strings.TrimSpace(r.Get("tipo_comprobante"))
				nc := strings.TrimSpace(r.Get("num_comprobante"))
				k := t + "-" + e + "-" + p + "-" + l + "-" + tc + "-" + nc
				if k == "-----" {
					return ""
				}
				return k
			},
			ColMap: map[string]string{
				"tipo": "tipo", "empresa": "empresa",
				"codigo_producto": "codigo_producto", "linea": "linea",
				"tipo_comprobante": "tipo_comprobante", "num_comprobante": "num_comprobante",
				"bodega": "bodega", "fecha": "fecha",
				"nombre": "nombre", "tipo_mov": "dc", "valor": "valor",
			},
		},
	}

	return registry
}

// extractField extracts the first capture group from text using a regex pattern.
func extractField(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// fmtIsamDate converts YYYYMMDD → YYYY-MM-DD if valid, else returns raw.
func fmtIsamDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 8 {
		y, m, d := raw[:4], raw[4:6], raw[6:8]
		if y >= "1900" && y <= "2099" {
			return y + "-" + m + "-" + d
		}
	}
	return raw
}

// diffGeneric reads an ISAM file via the ORM model and upserts to SQLite.
func (s *Server) diffGeneric(def *SyncTableDef) {
	if !def.Model.Exists() {
		s.db.AddLog("warn", def.Table, "File does not exist: "+def.Model.Table.Path)
		return
	}

	// For year-based tables that need all years (e.g. cartera), use AllMultiYear
	var rows []*isam.Row
	var err error
	if def.MultiYear && s.cfg.Siigo.DataPath != "" {
		rows, err = def.Model.AllMultiYear(s.cfg.Siigo.DataPath, "2025")
		if err != nil {
			s.db.AddLog("error", def.Model.FileName(), "Error reading multi-year: "+err.Error())
			return
		}
		s.db.AddLog("info", def.Table, fmt.Sprintf("Read %d rows from ISAM (all years)", len(rows)))
	} else {
		rows, err = def.Model.All()
		if err != nil {
			s.db.AddLog("error", def.Model.FileName(), "Error reading: "+err.Error())
			return
		}
		s.db.AddLog("info", def.Table, fmt.Sprintf("Read %d rows from ISAM", len(rows)))
	}

	// Pre-build a set of date field names from the ORM schema for auto-conversion
	dateFields := map[string]bool{}
	for _, f := range def.Model.Table.Fields {
		if f.Type == isam.FieldDate {
			dateFields[f.Name] = true
		}
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(rows))

	// Collect actual record data for webhook
	var addedRecords []map[string]interface{}
	var editedRecords []map[string]interface{}

	for _, r := range rows {
		key := strings.TrimSpace(def.KeyFunc(r))
		if key == "" {
			continue
		}
		currentKeys[key] = true

		// Build column values from ColMap
		cols := make(map[string]interface{}, len(def.ColMap)+len(def.BoolMap)+3)
		for sqlCol, ormField := range def.ColMap {
			if strings.HasPrefix(sqlCol, "~") {
				// Float/BCD field
				realCol := sqlCol[1:]
				cols[realCol] = r.GetFloat(ormField)
			} else if dateFields[ormField] {
				// Date field — convert YYYYMMDD → YYYY-MM-DD
				cols[sqlCol] = fmtIsamDate(r.Get(ormField))
			} else {
				cols[sqlCol] = strings.TrimSpace(r.Get(ormField))
			}
		}

		// Boolean S/N → 0/1 conversion
		for sqlCol, ormField := range def.BoolMap {
			v := strings.TrimSpace(r.Get(ormField))
			if v == "S" || v == "s" {
				cols[sqlCol] = 1
			} else {
				cols[sqlCol] = 0
			}
		}

		// Computed columns (e.g. saldo_final)
		if def.ComputedCols != nil {
			for k, v := range def.ComputedCols(r) {
				cols[k] = v
			}
		}

		action := s.db.UpsertGeneric(def.Table, def.KeyCol, key, r.Hash(), cols)
		switch action {
		case "add":
			adds++
			// Include key in the record data
			rec := make(map[string]interface{}, len(cols)+1)
			rec[def.KeyCol] = key
			for k, v := range cols {
				rec[k] = v
			}
			addedRecords = append(addedRecords, rec)
		case "edit":
			edits++
			rec := make(map[string]interface{}, len(cols)+1)
			rec[def.KeyCol] = key
			for k, v := range cols {
				rec[k] = v
			}
			editedRecords = append(editedRecords, rec)
		}
	}

	deletedKeys := s.db.MarkDeletedGenericKeys(def.Table, def.KeyCol, currentKeys)
	deletes := len(deletedKeys)

	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[%s] %s: %d new, %d updated, %d deleted (total: %d)",
			def.Label, def.Model.FileName(), adds, edits, deletes, len(rows)))

		// Only dispatch webhook and immediate send when NOT in setup wizard
		if s.cfg.SetupComplete && s.setupPopulating == "" {
			// Apply field mapping filter to webhook records (only send enabled/mapped fields)
			applyMapping := func(records []map[string]interface{}) []map[string]interface{} {
				mapped := make([]map[string]interface{}, len(records))
				for i, rec := range records {
					mapped[i] = s.cfg.ApplyFieldMapping(def.Table, rec)
				}
				return mapped
			}

			// Webhook: send actual changed records so Laravel can process them
			webhookData := map[string]interface{}{
				"table": def.Table,
				"label": def.Label,
			}
			if len(addedRecords) > 0 {
				webhookData["added"] = applyMapping(addedRecords)
			}
			if len(editedRecords) > 0 {
				webhookData["edited"] = applyMapping(editedRecords)
			}
			if len(deletedKeys) > 0 {
				webhookData["deleted"] = deletedKeys
			}
			s.webhookDispatch("table_changed", webhookData)

			// Immediate send: if sending is enabled for this table, send now
			if s.cfg.IsSendEnabled(def.Table) && !s.sendPaused {
				go s.sendTableNow(def.Table)
			}
		}
	}
	s.bot.NotifyChangesDetected(def.Label, adds, edits, deletes)

	// Run post-detect enrichment if defined (e.g. enrich clients with audit trail)
	if def.PostDetect != nil {
		def.PostDetect(s)
	}
}

// ==================== SYNC LOOP ====================

func (s *Server) startSyncLoop() {
	s.paused = false
	s.stopCh = make(chan bool, 2)
	s.db.AddLog("info", "SYNC", "Loops started (detect + send independent)")

	go s.startDetectLoop()
	s.startSendLoop()
}

// startDetectLoop uses fsnotify to watch ISAM files and only processes tables whose files changed.
// Falls back to full-scan polling if the watcher cannot be initialized.
func (s *Server) startDetectLoop() {
	// Run initial full scan on startup
	s.doDetectCycle()

	// Initialize watcher if registry and data path are available
	if s.syncRegistry != nil && s.cfg.Siigo.DataPath != "" {
		s.watcherCh = make(chan []string, 32)
		fileMap := buildFileToTablesMap(s.syncRegistry)
		fw, err := newFileWatcher(s.cfg.Siigo.DataPath, fileMap, 800*time.Millisecond, func(tables []string) {
			// Non-blocking send to channel
			select {
			case s.watcherCh <- tables:
			default:
				log.Printf("[WARN] Watcher event dropped for %v (detect already queued)", tables)
			}
		})
		if err != nil {
			s.db.AddLog("warn", "DETECT", fmt.Sprintf("File watcher failed, falling back to polling: %v", err))
			s.startDetectLoopPolling()
			return
		}
		s.fileWatcher = fw
		s.db.AddLog("info", "DETECT", fmt.Sprintf("File watcher active on %s (%d ISAM files mapped)", s.cfg.Siigo.DataPath, len(fileMap)))

		// Wait for watcher events
		for {
			select {
			case tables := <-s.watcherCh:
				if !s.paused && !s.detecting && s.cfg.SetupComplete {
					s.doDetectTables(tables)
				}
			case <-s.stopCh:
				s.fileWatcher.Stop()
				s.db.AddLog("info", "DETECT", "Detect loop stopped (watcher)")
				return
			}
		}
	} else {
		s.db.AddLog("warn", "DETECT", "No sync registry or data path — falling back to polling")
		s.startDetectLoopPolling()
	}
}

// startDetectLoopPolling is the legacy polling fallback when fsnotify is unavailable.
func (s *Server) startDetectLoopPolling() {
	interval := time.Duration(s.cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !s.paused && !s.detecting && s.cfg.SetupComplete {
				s.doDetectCycle()
			}
		case <-s.stopCh:
			s.db.AddLog("info", "DETECT", "Detect loop stopped (polling)")
			return
		}
	}
}

// startSendLoop sends pending records from SQLite to the API independently
func (s *Server) startSendLoop() {
	sendInterval := time.Duration(s.cfg.Sync.SendIntervalSeconds) * time.Second
	if sendInterval < 10*time.Second {
		sendInterval = 30 * time.Second
	}

	// Wait a few seconds before first send to let detect populate data
	time.Sleep(5 * time.Second)
	s.doSendCycle()

	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !s.paused && !s.sendPaused && !s.sending {
				s.doSendCycle()
			}
		case <-s.stopCh:
			s.db.AddLog("info", "SEND", "Send loop stopped")
			return
		}
	}
}

// doDetectTables processes only the specified tables (triggered by file watcher).
func (s *Server) doDetectTables(tables []string) {
	s.detecting = true
	defer func() { s.detecting = false }()

	if s.syncRegistry == nil && s.cfg.Siigo.DataPath != "" {
		s.syncRegistry = initSyncTables(s.cfg.Siigo.DataPath)
	}

	s.db.AddLog("info", "DETECT", fmt.Sprintf("--- File change detected → scanning %d table(s): %s ---", len(tables), strings.Join(tables, ", ")))

	// Temporary: Telegram notification for watcher testing
	if s.bot != nil {
		s.bot.Send(fmt.Sprintf("📂 <b>File watcher triggered</b>\n\nChanged tables: %s", strings.Join(tables, ", ")))
	}

	scanned := 0
	for _, table := range tables {
		if !s.cfg.SetupComplete || s.paused {
			s.db.AddLog("info", "DETECT", "Detection aborted (setup incomplete or paused)")
			return
		}
		if !s.cfg.IsDetectEnabled(table) {
			continue
		}
		scanned++
		if def, ok := s.syncRegistry[table]; ok {
			s.diffGeneric(def)
		} else if fn := s.getDiffFunc(table); fn != nil {
			fn()
		}
	}

	s.db.AddLog("info", "DETECT", fmt.Sprintf("--- Watcher scan completed --- %d table(s) processed", scanned))

	// Update stats and notify
	stats := s.db.GetStats()
	for _, table := range config.AllSyncTables() {
		total, _ := stats[table+"_total"].(int)
		pending, _ := stats[table+"_pending"].(int)
		synced, _ := stats[table+"_synced"].(int)
		errors, _ := stats[table+"_errors"].(int)
		s.db.RecordSyncStats(table, total, pending, synced, errors)
	}
	statsJSON, _ := json.Marshal(stats)
	sseNotify("sync_complete", string(statsJSON))
}

func (s *Server) doDetectCycle() {
	s.detecting = true
	defer func() { s.detecting = false }()

	s.db.AddLog("info", "DETECT", "--- Full detection cycle started --- Reading all ISAM files...")

	// Re-initialize registry if not yet done (e.g. setup wizard just completed)
	if s.syncRegistry == nil && s.cfg.Siigo.DataPath != "" {
		s.syncRegistry = initSyncTables(s.cfg.Siigo.DataPath)
	}

	// Detection order (matches config.AllSyncTables)
	detectOrder := []string{
		"clients", "products", "cartera",
		"documentos",
		"condiciones_pago", "codigos_dane",
		"audit_trail_terceros",
		"formulas",
		"vendedores_areas",
		"notas_documentos", "facturas_electronicas", "detalle_movimientos",
		"cartera_cxc", "ventas_productos",
	}

	enabledCount := 0
	for _, table := range detectOrder {
		// Abort if setup was reset (e.g. user cleared the database)
		if !s.cfg.SetupComplete || s.paused {
			s.db.AddLog("info", "DETECT", "Detection aborted (setup incomplete or paused)")
			return
		}
		if !s.cfg.IsDetectEnabled(table) {
			continue
		}
		enabledCount++
		s.db.AddLog("info", "DETECT", fmt.Sprintf("Scanning table: %s...", table))

		if def, ok := s.syncRegistry[table]; ok {
			s.diffGeneric(def)
		} else if fn := s.getDiffFunc(table); fn != nil {
			fn() // fallback to legacy diff
		}
	}

	s.db.AddLog("info", "DETECT", fmt.Sprintf("--- Detection cycle completed --- %d tables scanned", enabledCount))

	// Record sync stats for dashboard charts
	stats := s.db.GetStats()
	for _, table := range config.AllSyncTables() {
		total, _ := stats[table+"_total"].(int)
		pending, _ := stats[table+"_pending"].(int)
		synced, _ := stats[table+"_synced"].(int)
		errors, _ := stats[table+"_errors"].(int)
		s.db.RecordSyncStats(table, total, pending, synced, errors)
	}

	// SSE notification
	statsJSON, _ := json.Marshal(stats)
	sseNotify("sync_complete", string(statsJSON))
}

// maxSendFailures is the legacy constant; use s.cfg.Sync.GetCircuitBreakerThreshold() instead

func (s *Server) doSendCycle() {
	s.sending = true
	defer func() { s.sending = false }()

	// Skip entire cycle if no table has sending enabled
	anyEnabled := false
	tables := config.AllSyncTables()
	for _, t := range tables {
		if s.cfg.IsSendEnabled(t) {
			anyEnabled = true
			break
		}
	}
	if !anyEnabled {
		return
	}

	if !s.ensureLogin() {
		s.sendFailCount++
		s.checkSendCircuitBreaker("Login fallido")
		return
	}

	s.db.AddLog("info", "SEND", "--- Send cycle started --- Sending pending records to API...")

	totalSent, totalErrors := 0, 0
	for _, t := range tables {
		sent, errs := s.sendPending(t)
		totalSent += sent
		totalErrors += errs
	}

	// Automatic retry: requeue error records that haven't exceeded max retries
	s.retryFailedRecords()

	// Circuit breaker: if ALL records failed and none succeeded, count as failure
	if totalErrors > 0 && totalSent == 0 {
		s.sendFailCount++
		s.checkSendCircuitBreaker(fmt.Sprintf("%d errores, 0 enviados", totalErrors))
	} else if totalSent > 0 {
		// At least some succeeded — reset counter
		if s.sendFailCount > 0 {
			s.db.AddLog("info", "SEND", fmt.Sprintf("Send recovered (consecutive failures reset from %d to 0)", s.sendFailCount))
		}
		s.sendFailCount = 0
	}

	s.db.AddLog("info", "SEND", fmt.Sprintf("--- Send cycle completed --- Sent: %d, Errors: %d", totalSent, totalErrors))

	// SSE notification
	sseNotify("send_complete", fmt.Sprintf(`{"sent":%d,"errors":%d}`, totalSent, totalErrors))

	// Webhook notification
	s.webhookDispatch("send_complete", map[string]int{"sent": totalSent, "errors": totalErrors})
}

// sendTableNow immediately sends pending records for a specific table.
// Triggered by the file watcher when changes are detected in real time.
func (s *Server) sendTableNow(tableName string) {
	if !s.ensureLogin() {
		s.db.AddLog("warn", "SEND", fmt.Sprintf("Immediate send for %s skipped: login failed", tableName))
		return
	}
	sent, errs := s.sendPending(tableName)
	if sent > 0 || errs > 0 {
		s.db.AddLog("info", "SEND", fmt.Sprintf("[Immediate] %s: %d sent, %d errors", tableName, sent, errs))
		sseNotify("send_complete", fmt.Sprintf(`{"sent":%d,"errors":%d,"table":"%s","immediate":true}`, sent, errs, tableName))
		s.webhookDispatch("table_sent", map[string]interface{}{
			"table":  tableName,
			"sent":   sent,
			"errors": errs,
		})
	}
}

func (s *Server) checkSendCircuitBreaker(reason string) {
	threshold := s.cfg.Sync.GetCircuitBreakerThreshold()
	if s.sendFailCount >= threshold && !s.sendPaused {
		s.sendPaused = true
		msg := fmt.Sprintf("🔴 <b>Send auto-paused</b>\n\n%d consecutive failures.\nLast: %s\n\nCheck the destination server and reactivate send with /send-resume or from the web.",
			s.sendFailCount, reason)
		s.bot.Send(msg)
		s.db.AddLog("error", "SEND", fmt.Sprintf("Circuit breaker activated: %d consecutive failures (%s). Send auto-paused.", s.sendFailCount, reason))
		s.webhookDispatch("send_paused", map[string]interface{}{"reason": reason, "fail_count": s.sendFailCount})
	} else if s.sendFailCount > 0 && !s.sendPaused {
		s.db.AddLog("warn", "SEND", fmt.Sprintf("Send failure %d/%d (%s)", s.sendFailCount, threshold, reason))
	}
}

func (s *Server) ensureLogin() bool {
	if s.client != nil && s.client.IsAuthenticated() {
		return true
	}
	client := api.NewClient(s.cfg.Finearom.BaseURL, s.cfg.Finearom.Email, s.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		s.db.AddLog("error", "API", "Login failed: "+err.Error())
		s.bot.NotifyLoginFailed(s.cfg.Finearom.BaseURL, err.Error())
		return false
	}
	s.client = client
	return true
}

// ==================== SETUP WIZARD ====================

type setupTableDef struct {
	Name  string
	Label string
}

var setupTablesList = []setupTableDef{
	{"clients", "Clientes (Z08A)"},
	{"products", "Productos (Z04)"},
	{"cartera", "Cartera (Z09)"},
	{"documentos", "Documentos (Z11)"},
	{"condiciones_pago", "Condiciones de Pago (Z05)"},
	{"codigos_dane", "Codigos DANE"},
	{"formulas", "Formulas/Recetas (Z06 tipo R)"},
	{"vendedores_areas", "Vendedores/Areas (Z06A)"},
	{"notas_documentos", "Notas Documentos (Z49)"},
	{"facturas_electronicas", "Facturas Electronicas (Z09ELE)"},
	{"detalle_movimientos", "Detalle Movimientos (Z17)"},
	{"cartera_cxc", "Cartera CxC (Z07)"},
	{"ventas_productos", "Ventas Productos (Z09)"},
	{"recaudo", "Recaudo (Z09)"},
}

func (s *Server) getDiffFunc(table string) func() {
	// Try ORM registry first
	if def, ok := s.syncRegistry[table]; ok {
		return func() { s.diffGeneric(def) }
	}
	// Legacy fallback (should not be needed — all 27 tables are in the registry)
	switch table {
	case "clients":
		return s.diffClientes
	case "products":
		return s.diffProductos
	case "cartera":
		return s.diffCartera
	case "documentos":
		return s.diffDocumentos
	case "condiciones_pago":
		return s.diffCondicionesPago
	case "codigos_dane":
		return s.diffCodigosDane
	case "audit_trail_terceros":
		return s.diffAuditTrailTerceros
	case "cartera_cxc":
		return s.diffCarteraCxC
	case "ventas_productos":
		return s.diffVentasProductos
	case "recaudo":
		return s.diffRecaudo
	}
	return nil
}

// diffCarteraCxC reads Z07 (current year) cuenta 1305 and upserts to cartera_cxc table.
func (s *Server) diffCarteraCxC() {
	dataPath := s.cfg.Siigo.DataPath
	if dataPath == "" {
		return
	}
	currentYear := time.Now().Format("2006")
	filePath := filepath.Join(dataPath, "Z07"+currentYear)
	recs, _, err := isam.ReadFileV2All(filePath)
	if err != nil {
		s.db.AddLog("warn", "cartera_cxc", "Error reading Z07: "+err.Error())
		return
	}

	conn := s.db.GetConn()
	today := time.Now().Format("2006-01-02")
	adds, edits := 0, 0
	currentKeys := make(map[string]bool)
	var addedRecords, editedRecords []map[string]interface{}

	for _, rec := range recs {
		if len(rec) < 120 {
			continue
		}
		cuenta := strings.TrimSpace(string(rec[7:20]))
		if !strings.HasPrefix(cuenta, "0001305050000") {
			continue
		}
		tipoComp := strings.TrimSpace(string(rec[20:24]))
		recNit := strings.TrimLeft(strings.TrimSpace(string(rec[41:54])), "0")
		numDoc := int(parsers.DecodePacked(rec[24:30], 0))
		fechaRaw := string(rec[33:41])
		fecha := fechaRaw[:4] + "-" + fechaRaw[4:6] + "-" + fechaRaw[6:8]
		venceRaw := string(rec[67:75])
		vence := venceRaw[:4] + "-" + venceRaw[4:6] + "-" + venceRaw[6:8]
		saldo := parsers.DecodePacked(rec[110:118], 2)

		dias := 0
		if tv, err := time.Parse("2006-01-02", vence); err == nil {
			if tt, err := time.Parse("2006-01-02", today); err == nil {
				dias = int(tv.Sub(tt).Hours() / 24)
			}
		}

		docRef := fmt.Sprintf("%s-%s-%011d-001", tipoComp[:1], tipoComp[1:], numDoc)

		hashStr := fmt.Sprintf("%s|%s|%s|%s|%.2f", recNit, tipoComp, fecha, vence, saldo)
		h := sha256.Sum256([]byte(hashStr))
		hash := hex.EncodeToString(h[:8])

		key := fmt.Sprintf("%s-%s-%d", recNit, tipoComp, numDoc)
		currentKeys[key] = true

		var nombre string
		conn.QueryRow("SELECT nombre FROM clients WHERE nit = ?", recNit).Scan(&nombre)

		rec := map[string]interface{}{
			"nit": recNit, "nombre_cliente": nombre,
			"tipo_comprobante": tipoComp, "num_documento": numDoc,
			"documento_ref": docRef, "fecha": fecha,
			"fecha_vencimiento": vence, "dias": dias, "saldo": saldo,
		}

		action := s.db.UpsertGeneric("cartera_cxc", "record_key", key, hash, rec)
		if action == "add" {
			adds++
			addedRecords = append(addedRecords, rec)
		} else if action == "edit" {
			edits++
			editedRecords = append(editedRecords, rec)
		}
	}

	deletedKeys := s.db.MarkDeletedGenericKeys("cartera_cxc", "record_key", currentKeys)
	deletes := len(deletedKeys)
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Cartera CxC] Z07%s: %d new, %d updated, %d deleted (total: %d)", currentYear, adds, edits, deletes, len(currentKeys)))

	// Webhook dispatch
	if (adds > 0 || edits > 0 || deletes > 0) && s.cfg.SetupComplete && s.setupPopulating == "" {
		webhookData := map[string]interface{}{
			"table": "cartera_cxc",
			"label": "Cartera CxC",
		}
		if len(addedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(addedRecords))
			for i, r := range addedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("cartera_cxc", r)
			}
			webhookData["added"] = mapped
		}
		if len(editedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(editedRecords))
			for i, r := range editedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("cartera_cxc", r)
			}
			webhookData["edited"] = mapped
		}
		if deletes > 0 {
			webhookData["deleted"] = deletedKeys
		}
		s.webhookDispatch("table_changed", webhookData)

		nits := uniqueNITsFromRecords(append(addedRecords, editedRecords...))
		s.dispatchReportWebhooks("cartera_cxc", nits)

		if s.cfg.IsSendEnabled("cartera_cxc") && !s.sendPaused {
			go s.sendTableNow("cartera_cxc")
		}
	}
}

// diffVentasProductos reads Z09 (current year) tipo F cuenta 412 and upserts to ventas_productos table.
func (s *Server) diffVentasProductos() {
	dataPath := s.cfg.Siigo.DataPath
	if dataPath == "" {
		return
	}
	currentYear := time.Now().Format("2006")
	filePath := filepath.Join(dataPath, "Z09"+currentYear)
	recs, _, err := isam.ReadFileV2All(filePath)
	if err != nil {
		s.db.AddLog("warn", "ventas_productos", "Error reading Z09: "+err.Error())
		return
	}

	conn := s.db.GetConn()
	adds, edits := 0, 0
	currentKeys := make(map[string]bool)
	var addedRecords, editedRecords []map[string]interface{}

	// Pass 1: Build price map from F records (last known sale price per product)
	// Read BOTH current year AND previous year for price references
	priceMap := make(map[string]float64) // codProd → last price
	prevYear := strconv.Itoa(func() int { y, _ := strconv.Atoi(currentYear); return y - 1 }())
	for _, yearFile := range []string{
		filepath.Join(dataPath, "Z09"+prevYear),
		filePath, // current year (overwrites prev year prices = most recent)
	} {
		priceRecs, _, err := isam.ReadFileV2All(yearFile)
		if err != nil {
			continue
		}
		for _, rec := range priceRecs {
			if len(rec) < 500 || rec[0] != 'F' {
				continue
			}
			cuenta := strings.TrimSpace(string(rec[29:42]))
			if !strings.Contains(cuenta, "41") {
				continue
			}
			codProd := strings.TrimSpace(string(rec[67:74]))
			if codProd == "" || codProd == "0000000" {
				continue
			}
			precio := parsers.DecodePacked(rec[553:561], 3)
			if precio > 0 {
				priceMap[codProd] = precio
			}
		}
	}

	// Pass 2.5: Build cancellation set from J records (notas crédito)
	// J records cancel a specific F record with the same NIT+product+total.
	// We track how many times each combination is cancelled.
	type cancelKey struct {
		nit, prod string
		total     float64
	}
	cancelled := make(map[cancelKey]int)
	for _, rec := range recs {
		if len(rec) < 560 || rec[0] != 'J' {
			continue
		}
		cuenta := strings.TrimSpace(string(rec[29:42]))
		if !strings.Contains(cuenta, "41") {
			continue
		}
		codProd := strings.TrimSpace(string(rec[67:74]))
		if codProd == "" || codProd == "0000000" {
			continue
		}
		recNit := strings.TrimLeft(strings.TrimSpace(string(rec[16:29])), "0")
		total := parsers.DecodePacked(rec[145:153], 3)
		ck := cancelKey{recNit, codProd, math.Round(total*100) / 100}
		cancelled[ck]++
	}

	// Track which NIT+product+month combos have F records
	facturado := make(map[string]bool)
	seen := make(map[string]bool)       // dedup: same sale appearing as two invoices
	lineCount := make(map[string]int)   // count lines per base key (same doc+product can have multiple lines)

	// Pass 2: Process F records (facturas with sale price)
	for _, rec := range recs {
		if len(rec) < 500 || rec[0] != 'F' {
			continue
		}
		cuenta := strings.TrimSpace(string(rec[29:42]))
		if !strings.Contains(cuenta, "41") {
			continue
		}
		codProd := strings.TrimSpace(string(rec[67:74]))
		if codProd == "" || codProd == "0000000" {
			continue
		}

		recNit := strings.TrimLeft(strings.TrimSpace(string(rec[16:29])), "0")
		empresa := strings.TrimSpace(string(rec[1:4]))
		fechaRaw := string(rec[42:50])
		fecha := fechaRaw[:4] + "-" + fechaRaw[4:6] + "-" + fechaRaw[6:8]
		mes := fechaRaw[:4] + "-" + fechaRaw[4:6]
		desc := strings.TrimSpace(string(rec[93:133]))
		numDoc := int(parsers.DecodePacked(rec[4:10], 0))
		total := parsers.DecodePacked(rec[145:153], 3)
		cantidad := parsers.DecodePacked(rec[334:342], 1)
		precio := 0.0
		if cantidad > 0 {
			precio = math.Round(total/cantidad*100) / 100
		} else {
			precio = parsers.DecodePacked(rec[553:561], 3)
			if precio > 0 {
				cantidad = math.Round(total / precio)
			}
		}

		cuentaShort := cuenta
		if len(cuenta) > 4 {
			cuentaShort = cuenta[4:]
		}

		codProdFull := codProd

		// Skip F records cancelled by a J (nota crédito) with same NIT+product+total
		ck := cancelKey{recNit, codProdFull, math.Round(total*100) / 100}
		if cancelled[ck] > 0 {
			cancelled[ck]--
			continue
		}

		// Dedup: only for international clients (NIT 444444xxx) who generate
		// two invoices per sale (national + export)
		if strings.HasPrefix(recNit, "444444") {
			dedupKey := fmt.Sprintf("%s|%s|%s|%.2f", recNit, codProdFull, fecha, total)
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true
		}

		hashStr := fmt.Sprintf("%s|%s|%s|%d|%.2f|%.2f", recNit, codProdFull, fecha, numDoc, total, precio)
		h := sha256.Sum256([]byte(hashStr))
		hash := hex.EncodeToString(h[:8])

		baseKey := fmt.Sprintf("%s-%s-%s-%d", recNit, codProdFull, fecha, numDoc)
		lineCount[baseKey]++
		key := baseKey
		if lineCount[baseKey] > 1 {
			key = fmt.Sprintf("%s-%d", baseKey, lineCount[baseKey])
		}
		currentKeys[key] = true
		facturado[recNit+"|"+codProd+"|"+mes] = true

		var nombre string
		conn.QueryRow("SELECT nombre FROM clients WHERE nit = ?", recNit).Scan(&nombre)

		// Look up OC and lote from notas_documentos (Z49)
		numDocPadded := fmt.Sprintf("%011d", numDoc)
		var ordenCompra, lote string
		conn.QueryRow(`SELECT COALESCE(orden_compra, ''), COALESCE(lote, '') FROM notas_documentos
			WHERE num_documento = ? AND orden_compra != '' LIMIT 1`, numDocPadded).Scan(&ordenCompra, &lote)
		if ordenCompra == "" {
			// Try from texto field: "ORDEN DE COMPRA XXXXX"
			var texto string
			conn.QueryRow(`SELECT texto FROM notas_documentos
				WHERE num_documento = ? AND texto LIKE '%ORDEN DE COMPRA%' LIMIT 1`, numDocPadded).Scan(&texto)
			if texto != "" {
				re := regexp.MustCompile(`ORDEN DE COMPRA[:\s]+(\S+)`)
				if m := re.FindStringSubmatch(texto); len(m) > 1 {
					ordenCompra = m[1]
				}
				re2 := regexp.MustCompile(`LOTE[:\s]+(\S+)`)
				if m := re2.FindStringSubmatch(texto); len(m) > 1 {
					lote = m[1]
				}
			}
		}

		rec := map[string]interface{}{
			"nit": recNit, "nombre_cliente": nombre,
			"empresa": empresa, "cuenta": cuentaShort, "linea": cuentaShort,
			"codigo_producto": codProdFull, "descripcion": desc,
			"fecha": fecha, "mes": mes, "num_documento": numDoc,
			"orden_compra": ordenCompra, "lote": lote,
			"total_venta": total, "precio_unitario": precio, "cantidad": cantidad,
		}

		action := s.db.UpsertGeneric("ventas_productos", "record_key", key, hash, rec)
		if action == "add" {
			adds++
			addedRecords = append(addedRecords, rec)
		} else if action == "edit" {
			edits++
			editedRecords = append(editedRecords, rec)
		}
	}

	deletedKeys := s.db.MarkDeletedGenericKeys("ventas_productos", "record_key", currentKeys)
	deletes := len(deletedKeys)
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Ventas Productos] Z09%s: %d new, %d updated, %d deleted (total: %d)", currentYear, adds, edits, deletes, len(currentKeys)))

	// Webhook dispatch
	if (adds > 0 || edits > 0 || deletes > 0) && s.cfg.SetupComplete && s.setupPopulating == "" {
		webhookData := map[string]interface{}{
			"table": "ventas_productos",
			"label": "Ventas Productos",
		}
		if len(addedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(addedRecords))
			for i, r := range addedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("ventas_productos", r)
			}
			webhookData["added"] = mapped
		}
		if len(editedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(editedRecords))
			for i, r := range editedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("ventas_productos", r)
			}
			webhookData["edited"] = mapped
		}
		if deletes > 0 {
			webhookData["deleted"] = deletedKeys
		}
		s.webhookDispatch("table_changed", webhookData)

		// Also dispatch processed report webhooks per affected NIT
		nits := uniqueNITsFromRecords(append(addedRecords, editedRecords...))
		s.dispatchReportWebhooks("ventas_productos", nits)

		if s.cfg.IsSendEnabled("ventas_productos") && !s.sendPaused {
			go s.sendTableNow("ventas_productos")
		}
	}
}

// diffRecaudo reads Z09 (current year) tipo R cuenta 1305 and upserts to recaudo table.
// valor_cancelado = CxC payment × (CxC + reteFuente) / CxC to match Siigo's Excel report.
func (s *Server) diffRecaudo() {
	dataPath := s.cfg.Siigo.DataPath
	if dataPath == "" {
		return
	}
	currentYear := time.Now().Format("2006")
	filePath := filepath.Join(dataPath, "Z09"+currentYear)
	recs, _, err := isam.ReadFileV2All(filePath)
	if err != nil {
		s.db.AddLog("warn", "recaudo", "Error reading Z09: "+err.Error())
		return
	}

	// Build factura ratio map from F records (current + 3 prior years)
	type facturaRatio struct {
		cxc          float64
		clientRetes  float64 // all 1355 EXCEPT 1355951 (autorretencion)
	}
	facturaMap := make(map[string]*facturaRatio)
	prevYear := strconv.Itoa(func() int { y, _ := strconv.Atoi(currentYear); return y - 1 }())
	prevYear2 := strconv.Itoa(func() int { y, _ := strconv.Atoi(currentYear); return y - 2 }())
	prevYear3 := strconv.Itoa(func() int { y, _ := strconv.Atoi(currentYear); return y - 3 }())
	for _, yearFile := range []string{
		filepath.Join(dataPath, "Z09"+prevYear3),
		filepath.Join(dataPath, "Z09"+prevYear2),
		filepath.Join(dataPath, "Z09"+prevYear),
		filePath,
	} {
		fRecs, _, err := isam.ReadFileV2All(yearFile)
		if err != nil {
			continue
		}
		for _, rec := range fRecs {
			if len(rec) < 153 || rec[0] != 'F' {
				continue
			}
			fNit := strings.TrimLeft(strings.TrimSpace(string(rec[16:29])), "0")
			fDoc := int(parsers.DecodePacked(rec[4:10], 0))
			fCuenta := strings.TrimSpace(string(rec[29:42]))
			fDC := string(rec[143:144])
			fValor := parsers.DecodePacked(rec[145:153], 3)
			key := fmt.Sprintf("%s-%d", fNit, fDoc)
			if _, ok := facturaMap[key]; !ok {
				facturaMap[key] = &facturaRatio{}
			}
			if strings.Contains(fCuenta, "1305") && fDC == "D" {
				facturaMap[key].cxc += fValor
			}
			if fDC == "D" && strings.Contains(fCuenta, "1355") && !strings.Contains(fCuenta, "1355951") {
				facturaMap[key].clientRetes += fValor
			}
		}
	}

	conn := s.db.GetConn()
	adds, edits := 0, 0
	currentKeys := make(map[string]bool)
	lineCount := make(map[string]int)
	var addedRecords, editedRecords []map[string]interface{}

	reFactura := regexp.MustCompile(`(?:PG|ABONO|IMPTO[^F]*)\s*F\d+\s*-\s*(\d+)`)
	reDescuento := regexp.MustCompile(`F\d+\s*-\s*(\d+)`)

	// Build pronto pago discount map per recibo+factura (cuenta 5305351)
	descuentoPP := make(map[string]float64) // "recibo-factura" → discount amount
	for _, rec := range recs {
		if len(rec) < 153 || rec[0] != 'R' {
			continue
		}
		cuenta := strings.TrimSpace(string(rec[29:42]))
		if !strings.Contains(cuenta, "5305351") {
			continue
		}
		dc := string(rec[143:144])
		if dc != "D" {
			continue
		}
		numRecibo := int(parsers.DecodePacked(rec[4:10], 0))
		valor := parsers.DecodePacked(rec[145:153], 3)
		desc := strings.TrimSpace(string(rec[93:133]))
		numFac := 0
		if m := reDescuento.FindStringSubmatch(desc); len(m) > 1 {
			fmt.Sscanf(m[1], "%d", &numFac)
		}
		key := fmt.Sprintf("%d-%d", numRecibo, numFac)
		descuentoPP[key] += valor
	}

	for _, rec := range recs {
		if len(rec) < 153 || rec[0] != 'R' {
			continue
		}
		cuenta := strings.TrimSpace(string(rec[29:42]))
		if !strings.Contains(cuenta, "1305") {
			continue
		}
		dc := string(rec[143:144])
		if dc != "C" {
			continue
		}

		recNit := strings.TrimLeft(strings.TrimSpace(string(rec[16:29])), "0")
		fechaRaw := string(rec[42:50])
		fecha := fechaRaw[:4] + "-" + fechaRaw[4:6] + "-" + fechaRaw[6:8]
		mes := fechaRaw[:4] + "-" + fechaRaw[4:6]
		numRecibo := int(parsers.DecodePacked(rec[4:10], 0))
		valorCxC := parsers.DecodePacked(rec[145:153], 3)
		desc := strings.TrimSpace(string(rec[93:133]))

		// Parse factura number from description
		numFactura := 0
		if m := reFactura.FindStringSubmatch(desc); len(m) > 1 {
			fmt.Sscanf(m[1], "%d", &numFactura)
		}
		// Skip anticipos without factura reference (DEUDORES NACIONALES)
		if numFactura == 0 {
			continue
		}

		// Apply factura ratio: valor_cancelado = CxC × (CxC + clientRetes) / CxC - descuento_pronto_pago
		// clientRetes = all 1355 retentions EXCEPT autorretencion (1355951)
		valorCancelado := valorCxC
		if numFactura > 0 {
			fKey := fmt.Sprintf("%s-%d", recNit, numFactura)
			if fr, ok := facturaMap[fKey]; ok && fr.cxc > 0 && fr.clientRetes > 0 {
				valorCancelado = math.Round(valorCxC*(fr.cxc+fr.clientRetes)/fr.cxc*100) / 100
			}
		}
		// Subtract pronto pago discount if exists
		ppKey := fmt.Sprintf("%d-%d", numRecibo, numFactura)
		if dto, ok := descuentoPP[ppKey]; ok && dto > 0 {
			valorCancelado -= dto
		}

		// Determine tipo_pago from description
		tipoPago := "PG"
		if strings.HasPrefix(desc, "ABONO") {
			tipoPago = "ABONO"
		}

		// Look up fecha_vencimiento from cartera_cxc (Z07)
		var fechaVto string
		if numFactura > 0 {
			conn.QueryRow("SELECT COALESCE(fecha_vencimiento, '') FROM cartera_cxc WHERE num_documento = ? AND nit = ? LIMIT 1",
				numFactura, recNit).Scan(&fechaVto)
		}

		// Calculate dias
		dias := 0
		if fechaVto != "" {
			tRecibo, e1 := time.Parse("2006-01-02", fecha)
			tVto, e2 := time.Parse("2006-01-02", fechaVto)
			if e1 == nil && e2 == nil {
				dias = int(tRecibo.Sub(tVto).Hours() / 24)
			}
		}

		// Look up client name
		var nombre string
		conn.QueryRow("SELECT COALESCE(nombre, '') FROM clients WHERE nit = ?", recNit).Scan(&nombre)

		hashStr := fmt.Sprintf("%s|%d|%d|%.2f", recNit, numRecibo, numFactura, valorCancelado)
		h := sha256.Sum256([]byte(hashStr))
		hash := hex.EncodeToString(h[:8])

		baseKey := fmt.Sprintf("%s-%d-%d", recNit, numRecibo, numFactura)
		lineCount[baseKey]++
		key := baseKey
		if lineCount[baseKey] > 1 {
			key = fmt.Sprintf("%s-%d", baseKey, lineCount[baseKey])
		}
		currentKeys[key] = true

		rec := map[string]interface{}{
			"nit": recNit, "nombre_cliente": nombre,
			"num_recibo": numRecibo, "fecha_recibo": fecha, "mes": mes,
			"num_factura": numFactura, "fecha_vencimiento": fechaVto,
			"dias": dias, "valor_cancelado": valorCancelado,
			"tipo_pago": tipoPago,
			"vendedor_codigo": "", "vendedor_nombre": "",
			"descripcion": desc,
		}

		action := s.db.UpsertGeneric("recaudo", "record_key", key, hash, rec)
		if action == "add" {
			adds++
			addedRecords = append(addedRecords, rec)
		} else if action == "edit" {
			edits++
			editedRecords = append(editedRecords, rec)
		}
	}

	deletedKeys := s.db.MarkDeletedGenericKeys("recaudo", "record_key", currentKeys)
	deletes := len(deletedKeys)
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Recaudo] Z09%s: %d new, %d updated, %d deleted (total: %d)", currentYear, adds, edits, deletes, len(currentKeys)))

	// Webhook dispatch
	if (adds > 0 || edits > 0 || deletes > 0) && s.cfg.SetupComplete && s.setupPopulating == "" {
		webhookData := map[string]interface{}{
			"table": "recaudo",
			"label": "Recaudo",
		}
		if len(addedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(addedRecords))
			for i, r := range addedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("recaudo", r)
			}
			webhookData["added"] = mapped
		}
		if len(editedRecords) > 0 {
			mapped := make([]map[string]interface{}, len(editedRecords))
			for i, r := range editedRecords {
				mapped[i] = s.cfg.ApplyFieldMapping("recaudo", r)
			}
			webhookData["edited"] = mapped
		}
		if deletes > 0 {
			webhookData["deleted"] = deletedKeys
		}
		s.webhookDispatch("table_changed", webhookData)

		nits := uniqueNITsFromRecords(append(addedRecords, editedRecords...))
		s.dispatchReportWebhooks("recaudo", nits)

		if s.cfg.IsSendEnabled("recaudo") && !s.sendPaused {
			go s.sendTableNow("recaudo")
		}
	}
}

// handleRecaudo serves GET /api/recaudo/{nit} or /api/recaudo/all
// Filters: desde, hasta, fecha_desde, fecha_hasta, factura, recibo, tipo_pago, vencido, dias_min, dias_max, min_valor, max_valor, orden, agrupar
func (s *Server) handleRecaudo(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "GET only", 405)
		return
	}

	nit := strings.TrimPrefix(r.URL.Path, "/api/recaudo/")
	if nit == "" {
		jsonError(w, "NIT required. Use /api/recaudo/{nit} or /api/recaudo/all", 400)
		return
	}

	q := r.URL.Query()
	desde := q.Get("desde")
	hasta := q.Get("hasta")
	if desde == "" {
		desde = time.Now().Format("2006") + "-01"
	}
	if hasta == "" {
		hasta = time.Now().Format("2006-01")
	}

	conn := s.db.GetConn()

	// Use exact date range if provided, otherwise month range
	var query string
	args := []interface{}{}
	fechaDesde := q.Get("fecha_desde")
	fechaHasta := q.Get("fecha_hasta")
	if fechaDesde != "" || fechaHasta != "" {
		query = `SELECT nit, nombre_cliente, num_recibo, fecha_recibo, mes,
			num_factura, fecha_vencimiento, dias, valor_cancelado, tipo_pago,
			vendedor_codigo, vendedor_nombre, descripcion
			FROM recaudo WHERE 1=1`
		if fechaDesde != "" {
			query += " AND fecha_recibo >= ?"
			args = append(args, fechaDesde)
		}
		if fechaHasta != "" {
			query += " AND fecha_recibo <= ?"
			args = append(args, fechaHasta)
		}
	} else {
		query = `SELECT nit, nombre_cliente, num_recibo, fecha_recibo, mes,
			num_factura, fecha_vencimiento, dias, valor_cancelado, tipo_pago,
			vendedor_codigo, vendedor_nombre, descripcion
			FROM recaudo WHERE mes >= ? AND mes <= ?`
		args = append(args, desde, hasta)
	}

	// NIT filter
	if nit != "all" {
		query += " AND nit = ?"
		args = append(args, nit)
	}

	// Factura filter
	if factura := q.Get("factura"); factura != "" {
		query += " AND num_factura = ?"
		args = append(args, factura)
	}

	// Recibo filter
	if recibo := q.Get("recibo"); recibo != "" {
		query += " AND num_recibo = ?"
		args = append(args, recibo)
	}

	// Tipo pago filter (PG or ABONO)
	if tipoPago := q.Get("tipo_pago"); tipoPago != "" {
		query += " AND tipo_pago = ?"
		args = append(args, strings.ToUpper(tipoPago))
	}

	// Vencido filter (dias > 0 means paid after due date)
	if q.Get("vencido") == "true" {
		query += " AND dias > 0"
	} else if q.Get("vencido") == "false" {
		query += " AND dias <= 0"
	}

	// Dias range filter
	if diasMin := q.Get("dias_min"); diasMin != "" {
		query += " AND dias >= ?"
		args = append(args, diasMin)
	}
	if diasMax := q.Get("dias_max"); diasMax != "" {
		query += " AND dias <= ?"
		args = append(args, diasMax)
	}

	// Valor range filter
	if minValor := q.Get("min_valor"); minValor != "" {
		query += " AND valor_cancelado >= ?"
		args = append(args, minValor)
	}
	if maxValor := q.Get("max_valor"); maxValor != "" {
		query += " AND valor_cancelado <= ?"
		args = append(args, maxValor)
	}

	// Cliente name search
	if cliente := q.Get("cliente"); cliente != "" {
		query += " AND nombre_cliente LIKE ?"
		args = append(args, "%"+cliente+"%")
	}

	// Order by
	orden := q.Get("orden")
	switch orden {
	case "valor_desc":
		query += " ORDER BY valor_cancelado DESC"
	case "valor_asc":
		query += " ORDER BY valor_cancelado ASC"
	case "dias_desc":
		query += " ORDER BY dias DESC"
	case "dias_asc":
		query += " ORDER BY dias ASC"
	case "cliente":
		query += " ORDER BY nombre_cliente, fecha_recibo"
	case "factura":
		query += " ORDER BY num_factura, fecha_recibo"
	default:
		query += " ORDER BY fecha_recibo, num_recibo, nit"
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type recaudoItem struct {
		NIT              string  `json:"nit"`
		NombreCliente    string  `json:"nombre_cliente"`
		NumRecibo        int     `json:"num_recibo"`
		FechaRecibo      string  `json:"fecha_recibo"`
		Mes              string  `json:"mes"`
		NumFactura       int     `json:"num_factura"`
		FechaVencimiento string  `json:"fecha_vencimiento"`
		Dias             int     `json:"dias"`
		ValorCancelado   float64 `json:"valor_cancelado"`
		TipoPago         string  `json:"tipo_pago"`
		VendedorCodigo   string  `json:"vendedor_codigo"`
		VendedorNombre   string  `json:"vendedor_nombre"`
		Descripcion      string  `json:"descripcion"`
	}

	var items []recaudoItem
	totalValor := 0.0
	clientes := make(map[string]bool)

	for rows.Next() {
		var it recaudoItem
		rows.Scan(&it.NIT, &it.NombreCliente, &it.NumRecibo, &it.FechaRecibo, &it.Mes,
			&it.NumFactura, &it.FechaVencimiento, &it.Dias, &it.ValorCancelado, &it.TipoPago,
			&it.VendedorCodigo, &it.VendedorNombre, &it.Descripcion)
		items = append(items, it)
		totalValor += it.ValorCancelado
		clientes[it.NIT] = true
	}

	// Group by NIT if requested
	if q.Get("agrupar") == "nit" {
		type clienteResumen struct {
			NIT           string  `json:"nit"`
			NombreCliente string  `json:"nombre_cliente"`
			TotalValor    float64 `json:"total_valor"`
			TotalPagos    int     `json:"total_pagos"`
			DiasPromedio  float64 `json:"dias_promedio"`
		}
		grouped := make(map[string]*clienteResumen)
		order := []string{}
		for _, it := range items {
			if _, ok := grouped[it.NIT]; !ok {
				grouped[it.NIT] = &clienteResumen{NIT: it.NIT, NombreCliente: it.NombreCliente}
				order = append(order, it.NIT)
			}
			g := grouped[it.NIT]
			g.TotalValor += it.ValorCancelado
			g.TotalPagos++
			g.DiasPromedio += float64(it.Dias)
		}
		var resumen []clienteResumen
		for _, nit := range order {
			g := grouped[nit]
			if g.TotalPagos > 0 {
				g.DiasPromedio = math.Round(g.DiasPromedio / float64(g.TotalPagos))
			}
			g.TotalValor = math.Round(g.TotalValor*100) / 100
			resumen = append(resumen, *g)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"desde":           desde,
			"hasta":           hasta,
			"total_registros": len(items),
			"total_clientes":  len(clientes),
			"total_valor":     math.Round(totalValor*100) / 100,
			"agrupado_por":    "nit",
			"resumen":         resumen,
		})
		return
	}

	// Group by mes if requested
	if q.Get("agrupar") == "mes" {
		type mesResumen struct {
			Mes        string  `json:"mes"`
			TotalValor float64 `json:"total_valor"`
			TotalPagos int     `json:"total_pagos"`
			Clientes   int     `json:"clientes"`
		}
		grouped := make(map[string]*mesResumen)
		mesClientes := make(map[string]map[string]bool)
		order := []string{}
		for _, it := range items {
			if _, ok := grouped[it.Mes]; !ok {
				grouped[it.Mes] = &mesResumen{Mes: it.Mes}
				mesClientes[it.Mes] = make(map[string]bool)
				order = append(order, it.Mes)
			}
			grouped[it.Mes].TotalValor += it.ValorCancelado
			grouped[it.Mes].TotalPagos++
			mesClientes[it.Mes][it.NIT] = true
		}
		var resumen []mesResumen
		for _, mes := range order {
			g := grouped[mes]
			g.TotalValor = math.Round(g.TotalValor*100) / 100
			g.Clientes = len(mesClientes[mes])
			resumen = append(resumen, *g)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"desde":           desde,
			"hasta":           hasta,
			"total_registros": len(items),
			"total_clientes":  len(clientes),
			"total_valor":     math.Round(totalValor*100) / 100,
			"agrupado_por":    "mes",
			"resumen":         resumen,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"desde":           desde,
		"hasta":           hasta,
		"total_registros": len(items),
		"total_clientes":  len(clientes),
		"total_valor":     math.Round(totalValor*100) / 100,
		"recaudo":         items,
	})
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "GET only", 405)
		return
	}

	type tableStatus struct {
		Name   string `json:"name"`
		Label  string `json:"label"`
		Total  int    `json:"total"`
		Status string `json:"status"`
	}

	tables := make([]tableStatus, 0, len(setupTablesList))
	for _, t := range setupTablesList {
		var count int
		s.db.QueryCount(t.Name, &count)
		status := "pending"
		if s.setupPopulating == t.Name {
			status = "populating"
		} else if count > 0 || s.setupPopulated[t.Name] {
			status = "done"
		}
		tables = append(tables, tableStatus{
			Name:   t.Name,
			Label:  t.Label,
			Total:  count,
			Status: status,
		})
	}

	jsonResponse(w, map[string]interface{}{
		"setup_complete": s.cfg.SetupComplete,
		"tables":         tables,
		"populating":     s.setupPopulating,
	})
}

func (s *Server) handleSetupPopulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	if s.cfg.SetupComplete {
		jsonError(w, "Setup already completed", 400)
		return
	}

	if s.setupPopulating != "" {
		jsonError(w, "Ya hay una tabla en proceso: "+s.setupPopulating, 409)
		return
	}

	var req struct {
		Table string `json:"table"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON invalido", 400)
		return
	}

	diffFn := s.getDiffFunc(req.Table)
	if diffFn == nil {
		jsonError(w, "Tabla desconocida: "+req.Table, 400)
		return
	}

	s.setupPopulating = req.Table
	s.db.AddLog("info", "SETUP", "Poblando tabla: "+req.Table)

	go func() {
		defer func() {
			s.setupPopulating = ""
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("Panic poblando tabla %s: %v", req.Table, r)
				s.db.AddLog("error", "SETUP", errMsg)
				data, _ := json.Marshal(map[string]interface{}{"table": req.Table, "error": errMsg})
				sseNotify("setup_table_error", string(data))
				return
			}
		}()
		diffFn()
		var count int
		s.db.QueryCount(req.Table, &count)
		s.setupPopulated[req.Table] = true
		s.db.SetKV("setup_populated_"+req.Table, "true")
		data, _ := json.Marshal(map[string]interface{}{"table": req.Table, "count": count})
		sseNotify("setup_table_done", string(data))
		s.db.AddLog("info", "SETUP", fmt.Sprintf("Table %s populated: %d records", req.Table, count))
	}()

	jsonResponse(w, map[string]interface{}{"status": "started", "table": req.Table})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	if s.setupPopulating != "" {
		jsonError(w, "Hay una tabla en proceso: "+s.setupPopulating, 409)
		return
	}

	s.cfg.SetupComplete = true
	if err := s.cfg.Save("config.json"); err != nil {
		jsonError(w, "Error guardando config: "+err.Error(), 500)
		return
	}

	// Final enrichment: fill clients with audit trail data now that all tables are populated
	n, err := s.db.EnrichClientsFromAuditTrail()
	if err != nil {
		s.db.AddLog("warn", "SETUP", "Client enrichment from audit trail failed: "+err.Error())
	} else if n > 0 {
		s.db.AddLog("info", "SETUP", fmt.Sprintf("Enriched %d clients from audit trail (Z11N) during setup", n))
	}

	s.db.AddLog("info", "SETUP", "Setup wizard completed — sync loops starting in 3s")
	go func() {
		time.Sleep(3 * time.Second)
		s.startSyncLoop()
	}()
	sseNotify("setup_complete", "{}")

	jsonResponse(w, map[string]string{"status": "ok"})
}

// ==================== DIFF ====================

func (s *Server) diffClientes() {
	clientes, _, err := parsers.ParseTercerosAmpliados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z08A", "Error parsing: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(clientes))

	for _, t := range clientes {
		nit := strings.TrimSpace(t.Nit)
		if nit == "" {
			continue
		}
		currentKeys[nit] = true
		action := s.db.UpsertClient(nit, t.Name, t.PersonType, t.Company, t.Address, t.Email, t.LegalRep, t.Hash)
		switch action {
		case "add":
			adds++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Clients] + New: NIT=%s, Name=%s", nit, t.Name))
		case "edit":
			edits++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Clients] ~ Updated: NIT=%s, Name=%s", nit, t.Name))
		}
	}

	deletes := s.db.MarkDeletedClients(currentKeys)
	if deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Clients] - Deleted: %d records no longer in ISAM", deletes))
	}
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Clients] Summary: %d new, %d updated, %d deleted (ISAM total: %d)", adds, edits, deletes, len(clientes)))
	s.bot.NotifyChangesDetected("Clientes", adds, edits, deletes)
}

func (s *Server) diffProductos() {
	productos, year, err := parsers.ParseInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z04", "Error parsing inventory: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(productos))

	for _, p := range productos {
		key := p.Code
		if key == "" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertProduct(key, p.Name, p.ShortName, p.Group, p.Reference, p.Company, p.Hash)
		switch action {
		case "add":
			adds++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Products] + New: Code=%s, Name=%s, Group=%s", key, p.Name, p.Group))
		case "edit":
			edits++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Products] ~ Updated: Code=%s, Name=%s", key, p.Name))
		}
	}

	deletes := s.db.MarkDeletedProducts(currentKeys)
	if deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Products] - Deleted: %d records no longer in ISAM", deletes))
	}
	source := "Z04" + year
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Products] Summary (%s): %d new, %d updated, %d deleted (ISAM total: %d)", source, adds, edits, deletes, len(productos)))
	s.bot.NotifyChangesDetected("Productos", adds, edits, deletes)
}

func (s *Server) diffMovimientos() {
	movimientos, err := parsers.ParseMovimientos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z49", "Error parsing: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(movimientos))

	for _, m := range movimientos {
		key := m.VoucherType + "-" + m.DocNumber
		if key == "-" {
			key = m.Hash
		}
		currentKeys[key] = true

		desc := m.Description
		if m.Description2 != "" {
			if desc != "" {
				desc += " | " + m.Description2
			} else {
				desc = m.Description2
			}
		}
		// Z49 only has tipo, numero, nombre, descripcion. No fecha/cuenta/valor/tipoMov.
		action := s.db.UpsertMovement(key, m.VoucherType, m.Company, m.DocNumber, "", m.ThirdPartyName, "", desc, "", "", m.Hash)
		switch action {
		case "add":
			adds++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Movements] + New: %s Doc=%s, ThirdParty=%s", m.VoucherType, m.DocNumber, m.ThirdPartyName))
		case "edit":
			edits++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Movements] ~ Updated: %s Doc=%s, ThirdParty=%s", m.VoucherType, m.DocNumber, m.ThirdPartyName))
		}
	}

	deletes := s.db.MarkDeletedMovements(currentKeys)
	if deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Movements] - Deleted: %d records", deletes))
	}
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Movements] Summary: %d new, %d updated, %d deleted (ISAM total: %d)", adds, edits, deletes, len(movimientos)))
	s.bot.NotifyChangesDetected("Movimientos", adds, edits, deletes)
}

func (s *Server) diffCartera() {
	cartera, year, err := parsers.ParseCarteraLatest(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z09", "Error parsing cartera: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(cartera))
	file := "Z09" + year

	for _, c := range cartera {
		key := file + "-" + c.RecordType + "-" + c.Company + "-" + c.Sequence
		currentKeys[key] = true

		fecha := c.Date
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		action := s.db.UpsertCartera(key, c.RecordType, c.Company, c.Sequence, c.DocType, c.ThirdPartyNit, c.LedgerAccount, fecha, c.Description, c.MovType, c.Hash)
		switch action {
		case "add":
			adds++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Cartera] + New: NIT=%s, Account=%s, Date=%s, %s", c.ThirdPartyNit, c.LedgerAccount, fecha, c.Description))
		case "edit":
			edits++
			s.db.AddLog("info", "DETECT", fmt.Sprintf("[Cartera] ~ Updated: NIT=%s, Account=%s, Date=%s", c.ThirdPartyNit, c.LedgerAccount, fecha))
		}
	}

	deletes := s.db.MarkDeletedCartera(currentKeys)
	if deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Cartera] - Deleted: %d records", deletes))
	}
	s.db.AddLog("info", "DETECT", fmt.Sprintf("[Cartera] Summary (%s): %d new, %d updated, %d deleted (ISAM total: %d)", file, adds, edits, deletes, len(cartera)))
	s.bot.NotifyChangesDetected("Cartera ("+file+")", adds, edits, deletes)
}


func (s *Server) diffActivosFijos() {
	activos, year, err := parsers.ParseActivosFijos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z27", "Error parsing fixed assets: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(activos))

	for _, a := range activos {
		if a.Code == "" {
			continue
		}
		currentKeys[a.Code] = true
		action := s.db.UpsertActivoFijo(a.Code, a.Name, a.Company, a.ResponsibleNit, a.AcquisitionDate, a.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActivosFijos(currentKeys)
	source := "Z27" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Fixed Assets] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(activos)))
	}
	s.bot.NotifyChangesDetected("Activos Fijos", adds, edits, deletes)
}


func (s *Server) diffSaldosConsolidados() {
	saldos, year, err := parsers.ParseSaldosConsolidados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z28", "Error parsing consolidated balances: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(saldos))

	for _, sc := range saldos {
		if sc.LedgerAccount == "" {
			continue
		}
		currentKeys[sc.LedgerAccount] = true
		action := s.db.UpsertSaldoConsolidado(sc.LedgerAccount, sc.Company, sc.Hash, sc.PrevBalance, sc.Debit, sc.Credit, sc.FinalBalance)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedSaldosConsolidados(currentKeys)
	source := "Z28" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Consolidated Balances] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(saldos)))
	}
	s.bot.NotifyChangesDetected("Saldos Consolidados", adds, edits, deletes)
}

func (s *Server) diffDocumentos() {
	docs, year, err := parsers.ParseDocumentos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z11", "Error parsing documents: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(docs))

	for _, d := range docs {
		key := d.VoucherType + "-" + d.VoucherCode + "-" + d.Sequence + "-" + d.Hash[:8]
		currentKeys[key] = true
		fecha := d.Date
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		action := s.db.UpsertDocumento(key, d.VoucherType, d.VoucherCode, d.Sequence, d.ThirdPartyNit, d.LedgerAccount, d.ProductoRef, d.Warehouse, d.CostCenter, fecha, d.Description, d.MovType, d.Reference, d.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedDocumentos(currentKeys)
	source := "Z11" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Documents] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(docs)))
	}
	s.bot.NotifyChangesDetected("Documentos", adds, edits, deletes)
}

func (s *Server) diffTercerosAmpliados() {
	terceros, year, err := parsers.ParseTercerosAmpliados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z08A", "Error parsing extended third parties: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(terceros))

	for _, t := range terceros {
		if t.Nit == "" {
			continue
		}
		currentKeys[t.Nit] = true
		action := s.db.UpsertTerceroAmpliado(t.Nit, t.Name, t.Company, t.PersonType, t.LegalRep, t.Address, t.Email, t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedTercerosAmpliados(currentKeys)
	source := "Z08A" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Extended Clients] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(terceros)))
	}
	s.bot.NotifyChangesDetected("Terceros Ampliados", adds, edits, deletes)
}

func (s *Server) diffTransaccionesDetalle() {
	items, err := parsers.ParseTransaccionesDetalle(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z07T", "Error parsing transaction details: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, t := range items {
		key := t.VoucherType + "-" + t.Company + "-" + t.Sequence + "-" + t.LedgerAccount
		if key == "---" {
			key = t.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertTransaccionDetalle(key, t.VoucherType, t.Company, t.Sequence, t.ThirdPartyNit, t.LedgerAccount, t.DocDate, t.DueDate, t.MovType, t.Reference, t.Hash, t.Amount)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedTransaccionesDetalle(currentKeys)
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Transaction Details] Z07T: %d new, %d updated, %d deleted (total: %d)", adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffPeriodosContables() {
	items, year, err := parsers.ParsePeriodos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z26", "Error parsing accounting periods: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, p := range items {
		key := p.Company + "-" + p.PeriodNumber
		if key == "-" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertPeriodoContable(key, p.Company, p.PeriodNumber, p.StartDate, p.EndDate, p.Status, p.Hash, p.Balance1, p.Balance2, p.Balance3)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedPeriodosContables(currentKeys)
	source := "Z26" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Accounting Periods] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffCondicionesPago() {
	items, year, err := parsers.ParseCondicionesPago(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z05", "Error parsing payment conditions: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, c := range items {
		key := c.RecType + "-" + c.Company + "-" + c.Sequence + "-" + c.NIT
		if key == "---" {
			key = c.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertCondicionPago(key, c.RecType, c.Company, c.Sequence, c.DocType, c.Date, c.NIT, c.SecondaryType, c.RegDate, c.Hash, c.Amount)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedCondicionesPago(currentKeys)
	source := "Z05" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Payment Conditions] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}


func (s *Server) diffCodigosDane() {
	items, err := parsers.ParseDane(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZDANE", "Error parsing DANE codes: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, d := range items {
		if d.Code == "" {
			continue
		}
		currentKeys[d.Code] = true
		action := s.db.UpsertCodigoDane(d.Code, d.Name, d.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedCodigosDane(currentKeys)
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[DANE Codes] ZDANE: %d new, %d updated, %d deleted (total: %d)", adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffActividadesICA() {
	items, err := parsers.ParseICA(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZICA", "Error parsing ICA activities: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, a := range items {
		if a.Code == "" {
			continue
		}
		currentKeys[a.Code] = true
		action := s.db.UpsertActividadICA(a.Code, a.Name, a.Rate, a.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActividadesICA(currentKeys)
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[ICA Activities] ZICA: %d new, %d updated, %d deleted (total: %d)", adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffConceptosPILA() {
	items, err := parsers.ParsePILA(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZPILA", "Error parsing PILA concepts: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, p := range items {
		key := p.RecType + "-" + p.Fund + "-" + p.Concept
		if key == "--" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertConceptoPILA(key, p.RecType, p.Fund, p.Concept, p.Flags, p.BaseType, p.CalcBase, p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedConceptosPILA(currentKeys)
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[PILA Concepts] ZPILA: %d new, %d updated, %d deleted (total: %d)", adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffActivosFijosDetalle() {
	items, year, err := parsers.ParseActivosFijosDetalle(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z27D", "Error parsing fixed asset details: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, a := range items {
		key := a.Group + "-" + a.Sequence
		if key == "-" {
			key = a.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertActivoFijoDetalle(key, a.Group, a.Sequence, a.Name, a.ResponsibleNit, a.Code, a.Date, a.Hash, a.PurchaseValue)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActivosFijosDetalle(currentKeys)
	source := "Z27D" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Fixed Asset Details] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffAuditTrailTerceros() {
	items, year, err := parsers.ParseAuditTrailTerceros(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z11N", "Error parsing third-party audit trail: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, at := range items {
		key := at.ThirdPartyNit + "-" + at.Timestamp
		if key == "-" {
			key = at.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertAuditTrailTercero(key, at.ChangeDate, at.ThirdPartyNit, at.Timestamp, at.User, at.PeriodDate, at.DocType, at.Name, at.RepNit, at.RepName, at.Address, at.Email, at.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedAuditTrailTerceros(currentKeys)
	source := "Z11N" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Audit Trail] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffClasificacionCuentas() {
	items, year, err := parsers.ParseClasificacionCuentas(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z279CP", "Error parsing account classification: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, c := range items {
		if c.AccountCode == "" {
			continue
		}
		currentKeys[c.AccountCode] = true
		action := s.db.UpsertClasificacionCuenta(c.AccountCode, c.GroupCode, c.DetailCode, c.Description, c.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedClasificacionCuentas(currentKeys)
	source := "Z279CP" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Account Classification] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffMovimientosInventario() {
	items, year, err := parsers.ParseMovimientosInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z16", "Error parsing inventory movements: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, m := range items {
		if m.RecordKey == "" {
			continue
		}
		currentKeys[m.RecordKey] = true
		action := s.db.UpsertMovimientoInventario(m.RecordKey, m.Company, m.Group, m.ProductCode, m.VoucherType, m.VoucherCode, m.Sequence, m.DocType, m.Date, m.Quantity, m.Amount, m.MovType, m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedMovimientosInventario(currentKeys)
	source := "Z16" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Inventory Movements] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}

func (s *Server) diffSaldosInventario() {
	items, year, err := parsers.ParseSaldosInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z15", "Error parsing inventory balances: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, si := range items {
		if si.RecordKey == "" {
			continue
		}
		currentKeys[si.RecordKey] = true
		action := s.db.UpsertSaldoInventario(si.RecordKey, si.Company, si.Group, si.ProductCode, si.Hash, si.InitBalance, si.Entries, si.Withdrawals, si.FinalBalance)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedSaldosInventario(currentKeys)
	source := "Z15" + year
	if adds > 0 || edits > 0 || deletes > 0 {
		s.db.AddLog("info", "DETECT", fmt.Sprintf("[Inventory Balances] %s: %d new, %d updated, %d deleted (total: %d)", source, adds, edits, deletes, len(items)))
	}
}


// ==================== SEND PENDING ====================

func (s *Server) sendPending(tableName string) (int, int) {
	if !s.cfg.IsSendEnabled(tableName) {
		return 0, 0
	}

	var pending []storage.PendingRecord
	switch tableName {
	case "clients":
		pending = s.db.GetPendingClients()
	case "products":
		pending = s.db.GetPendingProducts()
	case "movements":
		pending = s.db.GetPendingMovements()
	case "cartera":
		pending = s.db.GetPendingCartera()
	case "movimientos_inventario":
		pending = s.db.GetPendingMovimientosInventario()
	case "saldos_inventario":
		pending = s.db.GetPendingSaldosInventario()
	case "activos_fijos_detalle":
		pending = s.db.GetPendingActivosFijosDetalle()
	case "audit_trail_terceros":
		pending = s.db.GetPendingAuditTrailTerceros()
	default:
		// Generic fallback for all other tables
		pending = s.db.GetPendingGeneric(tableName)
	}

	if len(pending) == 0 {
		return 0, 0
	}

	s.db.AddLog("info", "SEND", fmt.Sprintf("[%s] Sending %d pending records...", tableName, len(pending)))

	batchSize := s.cfg.Sync.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	batchDelay := time.Duration(s.cfg.Sync.BatchDelayMs) * time.Millisecond

	sent, errors := 0, 0
	lastErr := ""
	for i, rec := range pending {
		// Delay between batches (not before the first record)
		if i > 0 && i%batchSize == 0 && batchDelay > 0 {
			s.db.AddLog("info", "SEND", fmt.Sprintf("[%s] Batch %d done (%d sent), waiting %dms...", tableName, i/batchSize, sent, s.cfg.Sync.BatchDelayMs))
			time.Sleep(batchDelay)
		}

		filteredData := s.cfg.ApplyFieldMapping(tableName, rec.Data)
		err := s.client.Sync(tableName, rec.SyncAction, rec.Key, filteredData)
		dataJSON, _ := json.Marshal(rec.Data)
		dataStr := string(dataJSON)

		if err != nil {
			s.db.MarkSyncError(tableName, rec.ID, err.Error())
			s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "error", err.Error())
			s.db.AddLog("error", "SEND", fmt.Sprintf("[%s] Failed key=%s (%s): %s", tableName, rec.Key, rec.SyncAction, err.Error()))
			lastErr = err.Error()
			errors++
			continue
		}

		s.db.MarkSynced(tableName, rec.ID)
		s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "sent", "")
		s.db.AddLog("info", "SEND", fmt.Sprintf("[%s] Sent OK: key=%s, action=%s", tableName, rec.Key, rec.SyncAction))
		sent++
	}

	s.db.RemoveDeletedSynced(tableName)
	s.db.AddLog("info", "SEND", fmt.Sprintf("[%s] Summary: %d sent, %d errors (of %d pending)", tableName, sent, errors, len(pending)))
	if errors > 0 {
		s.bot.NotifySyncErrors(tableName, errors, lastErr)
	}
	return sent, errors
}

// ==================== AUTO RETRY ====================

func (s *Server) retryFailedRecords() {
	maxRetries := s.cfg.Sync.MaxRetries
	if maxRetries <= 0 {
		return // retry disabled
	}
	baseDelay := time.Duration(s.cfg.Sync.RetryDelaySeconds) * time.Second
	if baseDelay < 5*time.Second {
		baseDelay = 5 * time.Second
	}

	tables := []string{"clients", "products", "movements", "cartera", "movimientos_inventario", "saldos_inventario", "activos_fijos_detalle"}
	totalRequeued := 0
	maxRetryCount := 0

	for _, table := range tables {
		rc := s.db.GetMaxRetryCount(table, maxRetries)
		if rc > maxRetryCount {
			maxRetryCount = rc
		}
		n := s.db.RequeueRetryableErrors(table, maxRetries)
		if n > 0 {
			totalRequeued += n
			s.db.AddLog("info", "RETRY", fmt.Sprintf("[%s] %d records auto-retried (attempt %d/%d)", table, n, rc+1, maxRetries))
		}
	}

	if totalRequeued == 0 {
		return
	}

	// Exponential backoff: baseDelay * 2^retryCount (capped at 5 min)
	delay := baseDelay
	for i := 0; i < maxRetryCount; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}

	s.db.AddLog("info", "RETRY", fmt.Sprintf("Exponential backoff: waiting %ds before retrying %d records...", int(delay.Seconds()), totalRequeued))
	time.Sleep(delay)

	for _, table := range tables {
		s.sendPending(table)
	}
}

// ==================== ISAM CACHE ====================

// refreshCache triggers a diff for the given table, re-reading ISAM data into SQLite.
func (s *Server) refreshCache(which string) {
	if def, ok := s.syncRegistry[which]; ok {
		s.diffGeneric(def)
	} else if fn := s.getDiffFunc(which); fn != nil {
		fn()
	}
}

// ==================== HTTP HANDLERS ====================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime).Truncate(time.Second).String()
	dbOK := "ok"
	if _, _, _, err := s.db.QueryReadOnly("SELECT 1", 1, 0); err != nil {
		dbOK = "error"
	}
	status := "ok"
	if dbOK != "ok" {
		status = "degraded"
	}
	jsonResponse(w, map[string]interface{}{
		"status":    status,
		"version":   Version,
		"uptime":    uptime,
		"db":        dbOK,
		"detecting": s.detecting,
		"sending":   s.sending,
		"paused":    s.paused,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, s.db.GetStats())
}

func (s *Server) handleValidatePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		jsonResponse(w, map[string]interface{}{"valid": false, "error": "Ruta vacia"})
		return
	}

	// Check if directory exists
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		jsonResponse(w, map[string]interface{}{"valid": false, "error": "La carpeta no existe"})
		return
	}

	// Check for known ISAM files
	knownFiles := []string{"Z17", "Z06", "Z49", "ZDANE", "ZICA", "ZPILA", "Z06A", "Z279CP", "Z03", "Z08", "Z09", "Z25", "Z28", "Z27", "Z04"}
	var found []string
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		name := strings.ToUpper(e.Name())
		for _, kf := range knownFiles {
			if strings.HasPrefix(name, kf) {
				found = append(found, e.Name())
				break
			}
		}
	}

	if len(found) == 0 {
		jsonResponse(w, map[string]interface{}{
			"valid": false,
			"error": "No se encontraron archivos ISAM de Siigo en esta carpeta",
			"files": found,
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"valid":   true,
		"message": fmt.Sprintf("Carpeta valida: %d archivos ISAM encontrados", len(found)),
		"files":   found,
		"count":   len(found),
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			DataPath          string `json:"data_path"`
			Port              string `json:"port"`
			BaseURL           string `json:"base_url"`
			Email             string `json:"email"`
			Password          string `json:"password"`
			Interval          int    `json:"interval"`
			SendInterval      int    `json:"send_interval"`
			BatchSize         int    `json:"batch_size"`
			BatchDelayMs      int    `json:"batch_delay_ms"`
			MaxRetries        int    `json:"max_retries"`
			RetryDelaySeconds int    `json:"retry_delay_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		s.cfg.Siigo.DataPath = body.DataPath
		if body.Port != "" {
			s.cfg.Server.Port = body.Port
		}
		s.cfg.Finearom.BaseURL = body.BaseURL
		s.cfg.Finearom.Email = body.Email
		// Only update password if it's not the masked value (don't overwrite real password with asterisks)
		if body.Password != "" && !strings.Contains(body.Password, "***") {
			s.cfg.Finearom.Password = body.Password
		}
		s.cfg.Sync.IntervalSeconds = body.Interval
		if body.SendInterval > 0 {
			s.cfg.Sync.SendIntervalSeconds = body.SendInterval
		}
		if body.BatchSize > 0 {
			s.cfg.Sync.BatchSize = body.BatchSize
		}
		if body.BatchDelayMs >= 0 {
			s.cfg.Sync.BatchDelayMs = body.BatchDelayMs
		}
		if body.MaxRetries >= 0 {
			s.cfg.Sync.MaxRetries = body.MaxRetries
		}
		if body.RetryDelaySeconds >= 0 {
			s.cfg.Sync.RetryDelaySeconds = body.RetryDelaySeconds
		}
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", "Configuration saved")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	// Return config WITHOUT secrets
	jsonResponse(w, map[string]interface{}{
		"siigo":  s.cfg.Siigo,
		"server": map[string]interface{}{"port": s.cfg.Server.Port},
		"finearom": map[string]interface{}{
			"base_url": s.cfg.Finearom.BaseURL,
			"email":    s.cfg.Finearom.Email,
			"password": maskSecret(s.cfg.Finearom.Password),
		},
		"sync": s.cfg.Sync,
	})
}

func (s *Server) handleGlobalSend(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		s.cfg.GlobalSendEnabled = body.Enabled
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", fmt.Sprintf("Global send to Finearom: %v", body.Enabled))
		jsonResponse(w, map[string]interface{}{"status": "ok", "enabled": body.Enabled})
		return
	}
	jsonResponse(w, map[string]interface{}{"enabled": s.cfg.GlobalSendEnabled})
}

func (s *Server) handleAllowEditDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		s.cfg.AllowEditDelete = body.Enabled
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", fmt.Sprintf("Record edit/delete: %v", body.Enabled))
		jsonResponse(w, map[string]interface{}{"status": "ok", "enabled": body.Enabled})
		return
	}
	jsonResponse(w, map[string]interface{}{"enabled": s.cfg.AllowEditDelete})
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowEditDelete {
		jsonError(w, "Record edit/delete is not enabled", 403)
		return
	}

	table := r.URL.Query().Get("table")
	idStr := r.URL.Query().Get("id")
	if table == "" || idStr == "" {
		jsonError(w, "table e id son requeridos", 400)
		return
	}
	id := int64(0)
	fmt.Sscanf(idStr, "%d", &id)
	if id == 0 {
		jsonError(w, "id invalido", 400)
		return
	}

	switch r.Method {
	case "GET":
		record, err := s.db.GetRecordByID(table, id)
		if err != nil {
			jsonError(w, err.Error(), 404)
			return
		}
		jsonResponse(w, record)

	case "PUT":
		var body struct {
			Fields map[string]interface{} `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		// Prevent updating internal fields
		for _, blocked := range []string{"id", "hash", "sync_status", "sync_action", "sync_error", "updated_at", "synced_at"} {
			delete(body.Fields, blocked)
		}
		if err := s.db.UpdateRecord(table, id, body.Fields); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "EDIT", fmt.Sprintf("Record %d edited in %s", id, table))
		s.db.AddAudit(s.getUsernameFromRequest(r), "edit_record", table, fmt.Sprintf("%d", id), "")
		jsonResponse(w, map[string]string{"status": "ok"})

	case "DELETE":
		if err := s.db.DeleteRecord(table, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "DELETE", fmt.Sprintf("Record %d deleted from %s", id, table))
		s.db.AddAudit(s.getUsernameFromRequest(r), "delete_record", table, fmt.Sprintf("%d", id), "")
		jsonResponse(w, map[string]string{"status": "ok"})

	default:
		jsonError(w, "Metodo no soportado", 405)
	}
}

func maskSecret(s string) string {
	if len(s) <= 3 {
		return "***"
	}
	return s[:2] + strings.Repeat("*", len(s)-3) + s[len(s)-1:]
}

type ISAMPreview struct {
	File       string `json:"file"`
	RecordSize int    `json:"record_size"`
	Records    int    `json:"records"`
	NumKeys    int    `json:"num_keys"`
	HasIndex   bool   `json:"has_index"`
	UsedEXTFH  bool   `json:"used_extfh"`
	Format     int    `json:"format"`
	ModTime    string `json:"mod_time"`
}

func (s *Server) handleISAMInfo(w http.ResponseWriter, r *http.Request) {
	files := []string{"Z17", "Z06", "Z49"}
	var result []ISAMPreview
	for _, f := range files {
		path := filepath.Join(s.cfg.Siigo.DataPath, f)
		records, meta, err := isam.ReadIsamFileWithMeta(path)
		if err != nil {
			result = append(result, ISAMPreview{File: f, Records: -1})
			continue
		}
		modTime, _ := isam.GetModTime(path)
		t := time.Unix(0, modTime)
		result = append(result, ISAMPreview{
			File:       f,
			RecordSize: meta.RecSize,
			Records:    len(records),
			NumKeys:    meta.NumKeys,
			HasIndex:   meta.HasIndex,
			UsedEXTFH:  meta.UsedEXTFH,
			Format:     meta.Format,
			ModTime:    t.Format("2006-01-02 15:04:05"),
		})
	}
	jsonResponse(w, result)
}

func (s *Server) handleExtfhStatus(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{
		"available": isam.ExtfhAvailable(),
		"dll_path":  isam.ExtfhDLLPath(),
	})
}

func getQueryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n := 0
	fmt.Sscanf(v, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetClients(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetProducts(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleMovements(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetMovements(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleCartera(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetCarteraRecords(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}


func (s *Server) handleActivosFijos(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetActivosFijos(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}


func (s *Server) handleSaldosConsolidados(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetSaldosConsolidados(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleDocumentos(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetDocumentos(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleTercerosAmpliados(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetTercerosAmpliados(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleMovimientosInventario(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetMovimientosInventario(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleSaldosInventario(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetSaldosInventario(limit, offset, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleActivosFijosDetalle(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	records, total, _ := s.db.GetActivosFijosDetalle(page, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

func (s *Server) handleAuditTrailTerceros(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	records, total, _ := s.db.GetAuditTrailTerceros(page, search)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

// Relationship endpoints

// handleClienteProductos returns products associated with a client (via documentos).
// ?nit=12345 to filter by client, ?search=xxx to search, ?page=N for pagination.
func (s *Server) handleClienteProductos(w http.ResponseWriter, r *http.Request) {
	nit := r.URL.Query().Get("nit")
	search := r.URL.Query().Get("search")
	page := getQueryInt(r, "page", 1)
	limit := 50
	offset := (page - 1) * limit

	query := "SELECT * FROM v_cliente_productos WHERE 1=1"
	countQ := "SELECT COUNT(*) FROM v_cliente_productos WHERE 1=1"
	var args []interface{}
	if nit != "" {
		clause := " AND CAST(CAST(nit AS INTEGER) AS TEXT) = CAST(CAST(? AS INTEGER) AS TEXT)"
		query += clause
		countQ += clause
		args = append(args, nit)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		clause := " AND (LOWER(nombre_cliente) LIKE ? OR LOWER(nombre_producto) LIKE ? OR codigo_producto LIKE ?)"
		query += clause
		countQ += clause
		args = append(args, like, like, like)
	}

	var total int
	row := s.db.GetConn().QueryRow(countQ, args...)
	row.Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := s.db.GetConn().Query(query+" ORDER BY nombre_cliente, nombre_producto LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	records := scanRows(rows)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total})
}

// handleProductoReceta returns the recipe/formula for a product.
// ?codigo=5109105 returns all ingredients for that product.
func (s *Server) handleProductoReceta(w http.ResponseWriter, r *http.Request) {
	codigo := r.URL.Query().Get("codigo")
	if codigo == "" {
		jsonError(w, "Parametro 'codigo' requerido", 400)
		return
	}
	rows, err := s.db.GetConn().Query(
		"SELECT * FROM v_producto_receta WHERE codigo_producto = ? ORDER BY grupo_ingrediente, codigo_ingrediente", codigo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	records := scanRows(rows)
	jsonResponse(w, map[string]interface{}{"data": records, "producto": codigo})
}

// handleClienteProductoReceta returns the full chain: client → products → recipe ingredients.
// ?nit=12345 required.
func (s *Server) handleClienteProductoReceta(w http.ResponseWriter, r *http.Request) {
	nit := r.URL.Query().Get("nit")
	if nit == "" {
		jsonError(w, "Parametro 'nit' requerido", 400)
		return
	}
	page := getQueryInt(r, "page", 1)
	limit := 100
	offset := (page - 1) * limit

	where := "CAST(CAST(nit AS INTEGER) AS TEXT) = CAST(CAST(? AS INTEGER) AS TEXT)"
	var total int
	s.db.GetConn().QueryRow("SELECT COUNT(*) FROM v_cliente_producto_receta WHERE "+where, nit).Scan(&total)

	rows, err := s.db.GetConn().Query(
		"SELECT * FROM v_cliente_producto_receta WHERE "+where+" ORDER BY nombre_producto, tipo_ingrediente LIMIT ? OFFSET ?",
		nit, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	records := scanRows(rows)
	jsonResponse(w, map[string]interface{}{"data": records, "total": total, "nit": nit})
}

// scanRows converts sql.Rows into a slice of maps.
func scanRows(rows *sql.Rows) []map[string]interface{} {
	defer rows.Close()
	cols, _ := rows.Columns()
	var records []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		records = append(records, row)
	}
	return records
}

// handleGenericTable returns a handler that queries any table with pagination and search.
func (s *Server) handleGenericTable(tableName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := getQueryInt(r, "page", 1)
		search := r.URL.Query().Get("search")
		limit := 50
		offset := (page - 1) * limit
		records, total := s.db.GetGenericTable(tableName, limit, offset, search)
		jsonResponse(w, map[string]interface{}{"data": records, "total": total})
	}
}

func (s *Server) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	status := r.URL.Query().Get("status")
	limit := 50
	offset := (page - 1) * limit

	var records []storage.SyncRecord
	var total int
	var err error

	if search != "" || dateFrom != "" || dateTo != "" || status != "" {
		records, total, err = s.db.SearchSyncHistory(table, search, dateFrom, dateTo, status, limit, offset)
	} else {
		records, total, err = s.db.GetSyncHistory(table, limit, offset)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]interface{}{"records": records, "total": total})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	limit := 100
	offset := (page - 1) * limit
	level := r.URL.Query().Get("level")
	source := r.URL.Query().Get("source")
	search := r.URL.Query().Get("search")
	logs, total, err := s.db.GetLogsFiltered(limit, offset, level, source, search)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]interface{}{"logs": logs, "total": total})
}

func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if s.detecting || s.sending {
		jsonResponse(w, map[string]string{"message": "Sync already in progress"})
		return
	}
	go func() {
		s.doDetectCycle()
		s.doSendCycle()
	}()
	jsonResponse(w, map[string]string{"message": "Manual sync started (detect + send)"})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	s.paused = true
	s.db.AddLog("info", "SYNC", "Sync paused by user")
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	s.paused = false
	s.db.AddLog("info", "SYNC", "Sync resumed by user")
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{
		"detecting":        s.detecting,
		"sending":          s.sending,
		"syncing":          s.detecting || s.sending,
		"paused":           s.paused,
		"send_paused":      s.sendPaused,
		"send_fail_count":  s.sendFailCount,
		"detect_interval":  s.cfg.Sync.IntervalSeconds,
		"send_interval":    s.cfg.Sync.SendIntervalSeconds,
		"send_enabled":     s.cfg.SendEnabled,
		"detect_enabled":   s.cfg.DetectEnabled,
		"watcher_active":   s.fileWatcher != nil,
	})
}

func (s *Server) handleSendResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	wasPaused := s.sendPaused
	s.sendPaused = false
	s.sendFailCount = 0
	if wasPaused {
		s.db.AddLog("info", "SEND", "Send reactivated from web (circuit breaker reset)")
		s.bot.Send("▶️ <b>Send reactivated</b>\n\nReactivador from the web. Failure counter reset.")
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleRetryErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	var body struct {
		Table string `json:"table"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	n := s.db.RetryErrors(body.Table)
	s.db.AddLog("info", "SYNC", fmt.Sprintf("Retrying %d error records in %s", n, body.Table))
	jsonResponse(w, map[string]interface{}{"status": "ok", "count": n})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if s.cfg.Finearom.BaseURL == "" || s.cfg.Finearom.Email == "" || s.cfg.Finearom.Password == "" {
		jsonResponse(w, map[string]string{"status": "error", "message": "Credenciales incompletas: configure URL, email y password en Configuración"})
		return
	}
	client := api.NewClient(s.cfg.Finearom.BaseURL, s.cfg.Finearom.Email, s.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		jsonResponse(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	s.client = client
	s.db.AddLog("info", "API", "Connection successful with "+s.cfg.Finearom.BaseURL)
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleClearDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	// 1. Stop sync loops so they don't repopulate tables
	s.paused = true
	s.cfg.SetupComplete = false // This will abort any running doDetectCycle

	// 2. Wait for any in-progress detection/send cycle to finish (max 10s)
	for i := 0; i < 100; i++ {
		if !s.detecting && !s.sending {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 3. Reset setup state
	s.setupPopulated = make(map[string]bool)
	s.setupPopulating = ""
	s.db.DeleteKVPrefix("setup_populated_")
	if err := s.cfg.Save("config.json"); err != nil {
		s.db.AddLog("warn", "CONFIG", "Error resetting setup_complete: "+err.Error())
	}

	// 4. Clear all data tables (retry up to 3 times if DB is locked)
	var clearErr error
	for attempt := 0; attempt < 3; attempt++ {
		clearErr = s.db.ClearAll()
		if clearErr == nil {
			break
		}
		s.db.AddLog("warn", "APP", fmt.Sprintf("ClearAll attempt %d failed: %s", attempt+1, clearErr.Error()))
		time.Sleep(500 * time.Millisecond)
	}
	if clearErr != nil {
		jsonError(w, "Error clearing database: "+clearErr.Error(), 500)
		return
	}
	s.db.AddLog("warning", "APP", "Database cleared by user")
	s.db.AddAudit(s.getUsernameFromRequest(r), "clear_database", "", "", "All data tables cleared")
	s.bot.NotifyDBCleared()
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if err := s.db.ClearLogs(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("info", "APP", "Logs cleared by user")
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleRefreshCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	var body struct {
		Which string `json:"which"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.refreshCache(body.Which)
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleFieldMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body map[string][]config.FieldMap
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido: "+err.Error(), 400)
			return
		}
		s.cfg.FieldMappings = body
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", "Field mapping updated")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}

	s.cfg.EnsureFieldMappings()
	jsonResponse(w, s.cfg.FieldMappings)
}

func (s *Server) handleSendEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body map[string]bool
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido: "+err.Error(), 400)
			return
		}
		s.cfg.SendEnabled = body
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", "Per-module send config updated")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}

	s.cfg.EnsureFieldMappings() // also ensures SendEnabled
	jsonResponse(w, s.cfg.SendEnabled)
}

func (s *Server) handleDetectEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body map[string]bool
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido: "+err.Error(), 400)
			return
		}
		s.cfg.DetectEnabled = body
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", "Per-module detection config updated")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}

	s.cfg.EnsureFieldMappings()
	jsonResponse(w, s.cfg.DetectEnabled)
}

func (s *Server) handleErrorSummary(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, s.db.GetErrorSummary())
}

func (s *Server) handleExportHistory(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	data, err := s.db.ExportSyncHistoryCSV(table)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=sync_history.csv")
	w.Write(data)
}

func (s *Server) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	data, err := s.db.ExportLogsCSV()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=logs.csv")
	w.Write(data)
}

func (s *Server) handlePublicAPIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			Enabled     *bool  `json:"enabled"`
			JwtRequired *bool  `json:"jwt_required"`
			ApiKey      string `json:"api_key"`
			JwtSecret   string `json:"jwt_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		if body.Enabled != nil {
			s.cfg.PublicAPI.Enabled = *body.Enabled
		}
		if body.JwtRequired != nil {
			s.cfg.PublicAPI.JwtRequired = *body.JwtRequired
		}
		if body.ApiKey != "" {
			s.cfg.PublicAPI.ApiKey = body.ApiKey
		}
		if body.JwtSecret != "" {
			s.cfg.PublicAPI.JwtSecret = body.JwtSecret
		}
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", fmt.Sprintf("API publica: enabled=%v, jwt_required=%v", s.cfg.PublicAPI.Enabled, s.cfg.PublicAPI.JwtRequired))
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	// GET: return current public API config (hide secrets)
	jsonResponse(w, map[string]interface{}{
		"enabled":      s.cfg.PublicAPI.Enabled,
		"jwt_required": s.cfg.PublicAPI.JwtRequired,
		"api_key":      maskSecret(s.cfg.PublicAPI.ApiKey),
	})
}

func (s *Server) handleTelegramConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			Enabled            *bool  `json:"enabled"`
			BotToken           string `json:"bot_token"`
			ChatID             int64  `json:"chat_id"`
			ExecPin            string `json:"exec_pin"`
			NotifyServerStart  *bool  `json:"notify_server_start"`
			NotifySyncComplete *bool  `json:"notify_sync_complete"`
			NotifySyncErrors   *bool  `json:"notify_sync_errors"`
			NotifyLoginFailed  *bool  `json:"notify_login_failed"`
			NotifyChanges      *bool  `json:"notify_changes"`
			NotifyDBCleared    *bool  `json:"notify_db_cleared"`
			NotifyMaxRetries   *bool  `json:"notify_max_retries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		if body.Enabled != nil {
			s.cfg.Telegram.Enabled = *body.Enabled
		}
		if body.BotToken != "" {
			s.cfg.Telegram.BotToken = body.BotToken
		}
		if body.ChatID != 0 {
			s.cfg.Telegram.ChatID = body.ChatID
		}
		if body.ExecPin != "" {
			s.cfg.Telegram.ExecPin = body.ExecPin
		}
		if body.NotifyServerStart != nil {
			s.cfg.Telegram.NotifyServerStart = body.NotifyServerStart
		}
		if body.NotifySyncComplete != nil {
			s.cfg.Telegram.NotifySyncComplete = body.NotifySyncComplete
		}
		if body.NotifySyncErrors != nil {
			s.cfg.Telegram.NotifySyncErrors = body.NotifySyncErrors
		}
		if body.NotifyLoginFailed != nil {
			s.cfg.Telegram.NotifyLoginFailed = body.NotifyLoginFailed
		}
		if body.NotifyChanges != nil {
			s.cfg.Telegram.NotifyChanges = body.NotifyChanges
		}
		if body.NotifyDBCleared != nil {
			s.cfg.Telegram.NotifyDBCleared = body.NotifyDBCleared
		}
		if body.NotifyMaxRetries != nil {
			s.cfg.Telegram.NotifyMaxRetries = body.NotifyMaxRetries
		}
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// Stop old bot polling, recreate with new config, re-register commands
		s.bot.StopPolling()
		s.bot = telegram.New(&s.cfg.Telegram)
		s.registerBotCommands()
		if s.bot.IsEnabled() {
			s.bot.StartPolling()
			s.db.AddLog("info", "CONFIG", "Telegram bot activated and polling restarted")
		} else {
			s.db.AddLog("info", "CONFIG", "Telegram bot deactivated")
		}
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	jsonResponse(w, map[string]interface{}{
		"enabled":               s.cfg.Telegram.Enabled,
		"bot_token":             maskSecret(s.cfg.Telegram.BotToken),
		"chat_id":               s.cfg.Telegram.ChatID,
		"has_exec_pin":          s.cfg.Telegram.ExecPin != "",
		"notify_server_start":   s.cfg.Telegram.IsNotifyEnabled("server_start"),
		"notify_sync_complete":  s.cfg.Telegram.IsNotifyEnabled("sync_complete"),
		"notify_sync_errors":    s.cfg.Telegram.IsNotifyEnabled("sync_errors"),
		"notify_login_failed":   s.cfg.Telegram.IsNotifyEnabled("login_failed"),
		"notify_changes":        s.cfg.Telegram.IsNotifyEnabled("changes"),
		"notify_db_cleared":     s.cfg.Telegram.IsNotifyEnabled("db_cleared"),
		"notify_max_retries":    s.cfg.Telegram.IsNotifyEnabled("max_retries"),
	})
}

func (s *Server) handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !s.bot.IsEnabled() {
		jsonError(w, "Telegram not configured or disabled", 400)
		return
	}
	s.bot.Send("✅ <b>Test successful</b>\n\nTelegram notifications are working.")
	jsonResponse(w, map[string]string{"status": "ok"})
}

// ==================== SQL EXPLORER (read-only) ====================

var allowedSQLPrefixes = []string{"SELECT ", "PRAGMA ", "EXPLAIN "}

func isReadOnlySQL(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	for _, prefix := range allowedSQLPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	var req struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON", 400)
		return
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		jsonError(w, "Query vacia", 400)
		return
	}

	if !isReadOnlySQL(query) {
		jsonError(w, "Solo se permiten consultas SELECT, PRAGMA o EXPLAIN", 403)
		return
	}

	// Enforce limits
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	cols, data, total, err := s.db.QueryReadOnly(query, req.Limit, req.Offset)
	if err != nil {
		jsonError(w, "Error en query: "+err.Error(), 400)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"columns": cols,
		"data":    data,
		"total":   total,
		"limit":   req.Limit,
		"offset":  req.Offset,
	})
}

// ==================== PUBLIC API v1 (JWT) ====================

func (s *Server) jwtMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.PublicAPI.Enabled {
			jsonError(w, "API deshabilitada", 403)
			return
		}

		// Rate limiting by IP
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = strings.Split(fwd, ",")[0]
		}
		if !s.apiLimiter.Allow(strings.TrimSpace(ip)) {
			w.Header().Set("Retry-After", "60")
			jsonError(w, "Rate limit exceeded", 429)
			return
		}

		// Skip JWT validation when jwt_required is false (test mode)
		if !s.cfg.PublicAPI.JwtRequired {
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			jsonError(w, "Token requerido", 401)
			return
		}
		tokenStr := auth[7:]

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("metodo de firma inesperado")
			}
			return []byte(s.cfg.PublicAPI.JwtSecret), nil
		})
		if err != nil || !token.Valid {
			jsonError(w, "Token invalido o expirado", 401)
			return
		}

		next(w, r)
	}
}

func (s *Server) handleV1Auth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !s.cfg.PublicAPI.Enabled {
		jsonError(w, "API deshabilitada", 403)
		return
	}

	var body struct {
		ApiKey   string `json:"api_key"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "JSON invalido", 400)
		return
	}

	var authMethod string
	var authUser string

	if body.ApiKey != "" {
		// Method 1: API key
		if body.ApiKey != s.cfg.PublicAPI.ApiKey {
			jsonError(w, "API key invalida", 401)
			return
		}
		authMethod = "api_key"
		authUser = "api_key"
	} else if body.Username != "" && body.Password != "" {
		// Method 2: Username/password (root + app_users)
		authenticated := false
		role := ""
		if body.Username == s.cfg.Auth.Username && body.Password == s.cfg.Auth.Password {
			authenticated = true
			role = "root"
		} else {
			user, err := s.db.GetAppUser(body.Username)
			if err == nil && s.db.CheckAppUserPassword(user, body.Password) && user.Active {
				authenticated = true
				role = user.Role
			}
		}
		if !authenticated {
			jsonError(w, "Invalid credentials", 401)
			return
		}
		authMethod = "credentials"
		authUser = body.Username
		_ = role // available for claims if needed
	} else {
		jsonError(w, "Enviar api_key o username+password", 400)
		return
	}

	// Generate JWT valid for 24 hours
	claims := jwt.MapClaims{
		"iss":    "siigo-sync",
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
		"method": authMethod,
		"user":   authUser,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.PublicAPI.JwtSecret))
	if err != nil {
		jsonError(w, "Error generando token", 500)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"token":      tokenStr,
		"expires_in": 86400,
		"method":     authMethod,
		"user":       authUser,
	})
}

func (s *Server) handleV1Stats(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, s.db.GetStats())
}

func (s *Server) handleV1List(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}

		params := storage.APIQueryParams{
			Search:     r.URL.Query().Get("search"),
			SyncStatus: r.URL.Query().Get("sync_status"),
			Since:      r.URL.Query().Get("since"),
			Limit:      limit,
			Offset:     (page - 1) * limit,
		}

		result := s.db.APIGetRecords(table, params)
		jsonResponse(w, result)
	}
}

func (s *Server) handleV1Detail(table string) http.HandlerFunc {
	prefix := "/api/v1/" + table + "/"
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, prefix)
		if key == "" {
			jsonError(w, "Key requerida", 400)
			return
		}
		record := s.db.APIGetRecord(table, key)
		if record == nil {
			jsonError(w, "No encontrado", 404)
			return
		}
		jsonResponse(w, record)
	}
}

// ==================== OData ENDPOINTS ====================

// OData table metadata definitions
var odataTables = map[string]struct {
	EntityType string
	KeyProp    string
}{
	"clients":           {EntityType: "Client", KeyProp: "nit"},
	"products":          {EntityType: "Product", KeyProp: "code"},
	"cartera":           {EntityType: "Cartera", KeyProp: "record_key"},
	"documentos":        {EntityType: "Documento", KeyProp: "record_key"},
	"condiciones_pago":  {EntityType: "CondicionPago", KeyProp: "record_key"},
	"codigos_dane":      {EntityType: "CodigoDane", KeyProp: "codigo"},
	"formulas":          {EntityType: "Formula", KeyProp: "record_key"},
	"vendedores_areas":  {EntityType: "VendedorArea", KeyProp: "record_key"},
}

// OData ordered table list (for consistent output, v1 API, and OData routing)
var odataTableOrder = []string{
	"clients", "products", "cartera",
	"documentos", "condiciones_pago",
	"codigos_dane",
	"formulas",
	"vendedores_areas",
	"notas_documentos", "facturas_electronicas", "detalle_movimientos",
}

// OData relationships for Power BI auto-detection
type odataRelation struct {
	FromTable string // e.g. "movements"
	FromProp  string // e.g. "nit_tercero"
	ToTable   string // e.g. "clients"
	ToProp    string // e.g. "nit"
	NavName   string // navigation property name
}

var odataRelations = []odataRelation{
	{"cartera", "nit_tercero", "clients", "nit", "Client"},
	{"documentos", "nit_tercero", "clients", "nit", "Client"},
	{"documentos", "nit_cliente", "clients", "nit", "ClienteFactura"},
	{"condiciones_pago", "nit", "clients", "nit", "Client"},
	{"formulas", "codigo_producto", "products", "code", "Producto"},
	{"formulas", "codigo_ingrediente", "products", "code", "Ingrediente"},
	{"vendedores_areas", "nit", "clients", "nit", "Client"},
}

func (s *Server) handleODataServiceDoc(w http.ResponseWriter, r *http.Request) {
	var sets []map[string]string
	for _, table := range odataTableOrder {
		sets = append(sets, map[string]string{"name": table, "url": table, "kind": "EntitySet"})
	}
	doc := map[string]interface{}{
		"@odata.context": s.odataBaseURL(r) + "/$metadata",
		"value":          sets,
	}
	w.Header().Set("Content-Type", "application/json;odata.metadata=minimal")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleODataMetadata(w http.ResponseWriter, r *http.Request) {
	// Return CSDL XML metadata
	w.Header().Set("Content-Type", "application/xml")
	xml := `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="SiigoSync" xmlns="http://docs.oasis-open.org/odata/ns/edm">
`
	// Generate entity types from actual table columns
	for _, table := range odataTableOrder {
		meta := odataTables[table]
		cols := s.db.GetTableColumns(table)
		xml += fmt.Sprintf("      <EntityType Name=\"%s\">\n", meta.EntityType)
		xml += fmt.Sprintf("        <Key><PropertyRef Name=\"%s\"/></Key>\n", meta.KeyProp)
		for _, col := range cols {
			edm := "Edm.String"
			if col == "id" || col == "retry_count" {
				edm = "Edm.Int64"
			} else if col == "updated_at" || col == "synced_at" {
				edm = "Edm.DateTimeOffset"
			} else if strings.HasPrefix(col, "saldo") || col == "debito" || col == "credito" || col == "valor_compra" {
				edm = "Edm.Decimal"
			}
			xml += fmt.Sprintf("        <Property Name=\"%s\" Type=\"%s\"/>\n", col, edm)
		}
		// Add navigation properties for this table
		for _, rel := range odataRelations {
			if rel.FromTable == table {
				targetMeta := odataTables[rel.ToTable]
				xml += fmt.Sprintf("        <NavigationProperty Name=\"%s\" Type=\"SiigoSync.%s\" Nullable=\"true\">\n", rel.NavName, targetMeta.EntityType)
				xml += fmt.Sprintf("          <ReferentialConstraint Property=\"%s\" ReferencedProperty=\"%s\"/>\n", rel.FromProp, rel.ToProp)
				xml += "        </NavigationProperty>\n"
			}
		}
		xml += "      </EntityType>\n"
	}

	xml += "      <EntityContainer Name=\"SiigoSyncContainer\">\n"
	for _, table := range odataTableOrder {
		meta := odataTables[table]
		xml += fmt.Sprintf("        <EntitySet Name=\"%s\" EntityType=\"SiigoSync.%s\">\n", table, meta.EntityType)
		// Add NavigationPropertyBinding for relationships
		for _, rel := range odataRelations {
			if rel.FromTable == table {
				xml += fmt.Sprintf("          <NavigationPropertyBinding Path=\"%s\" Target=\"%s\"/>\n", rel.NavName, rel.ToTable)
			}
		}
		xml += "        </EntitySet>\n"
	}
	xml += "      </EntityContainer>\n"
	xml += "    </Schema>\n  </edmx:DataServices>\n</edmx:Edmx>"

	w.Write([]byte(xml))
}

func (s *Server) handleODataCollection(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Check for /$count suffix
		if strings.HasSuffix(r.URL.Path, "/$count") {
			count := s.db.ODataGetCount(table, q.Get("$filter"))
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "%d", count)
			return
		}

		top, _ := strconv.Atoi(q.Get("$top"))
		skip, _ := strconv.Atoi(q.Get("$skip"))
		countParam := strings.ToLower(q.Get("$count"))

		params := storage.ODataParams{
			Top:     top,
			Skip:    skip,
			Filter:  q.Get("$filter"),
			OrderBy: q.Get("$orderby"),
			Select:  q.Get("$select"),
			Count:   countParam == "true",
		}

		result := s.db.ODataGetRecords(table, params)

		// Build response
		resp := map[string]interface{}{
			"@odata.context": s.odataBaseURL(r) + "/$metadata#" + table,
			"value":          result.Value,
		}
		if result.Count != nil {
			resp["@odata.count"] = *result.Count
		}

		// NextLink if there are more records
		effectiveTop := top
		if effectiveTop <= 0 {
			effectiveTop = 100
		}
		if result.Count != nil && skip+effectiveTop < *result.Count {
			nextSkip := skip + effectiveTop
			nextURL := fmt.Sprintf("%s/odata/%s?$top=%d&$skip=%d", s.odataBaseURL(r), table, effectiveTop, nextSkip)
			if params.Filter != "" {
				nextURL += "&$filter=" + params.Filter
			}
			if params.OrderBy != "" {
				nextURL += "&$orderby=" + params.OrderBy
			}
			if params.Select != "" {
				nextURL += "&$select=" + params.Select
			}
			if params.Count {
				nextURL += "&$count=true"
			}
			resp["@odata.nextLink"] = nextURL
		}

		w.Header().Set("Content-Type", "application/json;odata.metadata=minimal")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Server) handleODataEntity(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/odata/"+table)
		// Support both /odata/clients/900123 and /odata/clients('900123')
		key := strings.TrimLeft(path, "/")
		key = strings.Trim(key, "'\"()")
		if key == "" {
			jsonError(w, "Key required", 400)
			return
		}
		record := s.db.APIGetRecord(table, key)
		if record == nil {
			jsonError(w, "Not found", 404)
			return
		}
		resp := map[string]interface{}{
			"@odata.context": s.odataBaseURL(r) + "/$metadata#" + table + "/$entity",
		}
		for k, v := range record {
			resp[k] = v
		}
		w.Header().Set("Content-Type", "application/json;odata.metadata=minimal")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Server) odataBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/odata"
}

// ==================== AUDIT TRAIL & HISTORY ====================

// handleVentas reads ventas_productos from SQLite (populated from Z09 tipo F cuenta 412).
// GET /api/ventas/{nit}       → ventas for one client
// GET /api/ventas/all         → ventas for all clients
// Query params:
//   ?desde=2026-01&hasta=2026-03     (month range, default: current year)
//   ?fecha_desde=2026-01-15&fecha_hasta=2026-03-20  (exact date range)
//   ?empresa=003                     (filter by empresa)
//   ?cuenta=120470100                (filter by cuenta/linea)
//   ?producto=2008189                (filter by product code)
func (s *Server) handleVentas(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "GET only", 405)
		return
	}

	nit := strings.TrimPrefix(r.URL.Path, "/api/ventas/")
	nit = strings.TrimSpace(nit)
	if nit == "" {
		jsonError(w, "NIT required (use 'all' for all clients)", 400)
		return
	}

	// Filters
	desde := r.URL.Query().Get("desde")
	hasta := r.URL.Query().Get("hasta")
	fechaDesde := r.URL.Query().Get("fecha_desde")
	fechaHasta := r.URL.Query().Get("fecha_hasta")
	empresa := r.URL.Query().Get("empresa")
	cuentaFilter := r.URL.Query().Get("cuenta")
	productoFilter := r.URL.Query().Get("producto")

	if desde == "" && fechaDesde == "" {
		desde = time.Now().Format("2006") + "-01"
	}
	if hasta == "" && fechaHasta == "" {
		hasta = time.Now().Format("2006-01")
	}

	conn := s.db.GetConn()

	// Build WHERE clauses
	conditions := []string{}
	var args []interface{}

	if nit != "all" {
		conditions = append(conditions, "nit = ?")
		args = append(args, nit)
	}
	if fechaDesde != "" && fechaHasta != "" {
		conditions = append(conditions, "fecha >= ? AND fecha <= ?")
		args = append(args, fechaDesde, fechaHasta)
	} else {
		conditions = append(conditions, "mes >= ? AND mes <= ?")
		args = append(args, desde, hasta)
	}
	if empresa != "" {
		conditions = append(conditions, "empresa = ?")
		args = append(args, empresa)
	}
	if cuentaFilter != "" {
		conditions = append(conditions, "cuenta LIKE ?")
		args = append(args, "%"+cuentaFilter+"%")
	}
	if productoFilter != "" {
		conditions = append(conditions, "codigo_producto LIKE ?")
		args = append(args, "%"+productoFilter+"%")
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "1=1"
	}

	rows, err := conn.Query(fmt.Sprintf(`
		SELECT nit, nombre_cliente, empresa, cuenta, codigo_producto, descripcion, mes,
			SUM(total_venta) as total, MAX(precio_unitario) as precio, SUM(cantidad) as qty,
			GROUP_CONCAT(DISTINCT CASE WHEN orden_compra != '' THEN orden_compra END) as ocs,
			GROUP_CONCAT(DISTINCT CASE WHEN lote != '' THEN lote END) as lotes
		FROM ventas_productos
		WHERE %s
		GROUP BY nit, cuenta, codigo_producto, mes
		ORDER BY nit, cuenta, codigo_producto, mes
	`, whereClause), args...)
	if err != nil {
		jsonError(w, "Query error: "+err.Error(), 500)
		return
	}
	defer rows.Close()

	type prodKey struct{ nit, cuenta, codProd string }
	type prodData struct {
		NIT          string             `json:"nit"`
		Nombre       string             `json:"nombre_cliente"`
		Empresa      string             `json:"empresa"`
		Cuenta       string             `json:"cuenta"`
		Producto     string             `json:"producto"`
		Desc         string             `json:"descripcion"`
		Precio       float64            `json:"precio_unitario"`
		OrdenCompra  string             `json:"orden_compra,omitempty"`
		Lote         string             `json:"lote,omitempty"`
		Meses        map[string]float64 `json:"valores_mes"`
		Cantidades   map[string]float64 `json:"cantidades_mes"`
		TotalValor   float64            `json:"total_valor"`
		TotalQty     float64            `json:"total_cantidad"`
	}

	grouped := make(map[prodKey]*prodData)
	clientSet := make(map[string]bool)

	for rows.Next() {
		var rNit, nombre, emp, cta, codProd, desc, mes string
		var total, precio, qty float64
		var ocs, lotes sql.NullString
		rows.Scan(&rNit, &nombre, &emp, &cta, &codProd, &desc, &mes, &total, &precio, &qty, &ocs, &lotes)

		clientSet[rNit] = true
		key := prodKey{rNit, cta, codProd}
		pd, ok := grouped[key]
		if !ok {
			pd = &prodData{
				NIT: rNit, Nombre: nombre, Empresa: emp, Cuenta: cta,
				OrdenCompra: ocs.String, Lote: lotes.String,
				Producto: codProd, Desc: desc, Precio: precio,
				Meses: make(map[string]float64), Cantidades: make(map[string]float64),
			}
			grouped[key] = pd
		}
		pd.Meses[mes] += total
		pd.Cantidades[mes] += qty
		pd.TotalValor += total
		pd.TotalQty += qty
	}

	result := make([]prodData, 0, len(grouped))
	for _, pd := range grouped {
		result = append(result, *pd)
	}
	// Sort
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].NIT > result[j].NIT || (result[i].NIT == result[j].NIT && result[i].Cuenta+result[i].Producto > result[j].Cuenta+result[j].Producto) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	var grandTotal, grandQty float64
	for _, pd := range result {
		grandTotal += pd.TotalValor
		grandQty += pd.TotalQty
	}

	jsonResponse(w, map[string]interface{}{
		"desde":          desde,
		"hasta":          hasta,
		"total_lineas":   len(result),
		"total_clientes": len(clientSet),
		"total_valor":    math.Round(grandTotal*100) / 100,
		"total_cantidad": grandQty,
		"ventas":         result,
	})
}

// handleCarteraCliente reads cartera_cxc from SQLite (populated from Z07 cuenta 1305).
// GET /api/cartera-cliente/{nit}   → cartera for one client
// GET /api/cartera-cliente/all     → cartera for all clients
// Params: ?dias_mora=-270&dias_cobro=10&vencido=true
func (s *Server) handleCarteraCliente(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "GET only", 405)
		return
	}

	nit := strings.TrimPrefix(r.URL.Path, "/api/cartera-cliente/")
	nit = strings.TrimSpace(nit)
	if nit == "" {
		jsonError(w, "NIT required (use 'all' for all clients)", 400)
		return
	}

	diasMora := -9999
	diasCobro := 9999
	soloVencido := r.URL.Query().Get("vencido") == "true"
	if v, err := strconv.Atoi(r.URL.Query().Get("dias_mora")); err == nil {
		diasMora = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("dias_cobro")); err == nil {
		diasCobro = v
	}

	conn := s.db.GetConn()
	today := time.Now().Format("2006-01-02")

	var whereNIT string
	var args []interface{}
	if nit == "all" {
		whereNIT = "1=1"
	} else {
		whereNIT = "nit = ?"
		args = append(args, nit)
	}

	rows, err := conn.Query(fmt.Sprintf(`
		SELECT nit, nombre_cliente, tipo_comprobante, num_documento, documento_ref,
			fecha, fecha_vencimiento, saldo
		FROM cartera_cxc
		WHERE %s
		ORDER BY fecha DESC, num_documento DESC
	`, whereNIT), args...)
	if err != nil {
		jsonError(w, "Query error: "+err.Error(), 500)
		return
	}
	defer rows.Close()

	type carteraRow struct {
		NIT           string  `json:"nit"`
		NombreEmpresa string  `json:"nombre_empresa"`
		Cuenta        string  `json:"cuenta"`
		DescCuenta    string  `json:"descripcion_cuenta"`
		Documento     string  `json:"documento"`
		Fecha         string  `json:"fecha"`
		Vence         string  `json:"vence"`
		Dias          int     `json:"dias"`
		SaldoContable float64 `json:"saldo_contable"`
		Vencido       float64 `json:"vencido"`
		SaldoVencido  float64 `json:"saldo_vencido"`
		CateraType    string  `json:"catera_type"`
	}

	var result []carteraRow
	for rows.Next() {
		var rNit, nombre, tipoComp, docRef, fecha, vence string
		var numDoc int
		var saldo float64
		rows.Scan(&rNit, &nombre, &tipoComp, &numDoc, &docRef, &fecha, &vence, &saldo)

		dias := 0
		if tv, err := time.Parse("2006-01-02", vence); err == nil {
			if tt, err := time.Parse("2006-01-02", today); err == nil {
				dias = int(tv.Sub(tt).Hours() / 24)
			}
		}

		if dias < diasMora || dias > diasCobro {
			continue
		}
		if soloVencido && dias >= 0 {
			continue
		}

		vencido := 0.0
		if dias < 0 && saldo > 0 {
			vencido = saldo
		}

		result = append(result, carteraRow{
			NIT: rNit, NombreEmpresa: nombre,
			Cuenta: "13050500", DescCuenta: "DEUDORES NACIONALES",
			Documento: docRef, Fecha: fecha, Vence: vence, Dias: dias,
			SaldoContable: math.Round(saldo*100) / 100,
			Vencido: math.Round(vencido*100) / 100,
			SaldoVencido: math.Round(vencido*100) / 100,
			CateraType: "nacional",
		})
	}

	var totalVencidos, totalPorVencer float64
	for _, cr := range result {
		if cr.Dias < 0 {
			totalVencidos += cr.SaldoContable
		} else {
			totalPorVencer += cr.SaldoContable
		}
	}

	jsonResponse(w, map[string]interface{}{
		"fecha_cartera":    today,
		"total_documentos": len(result),
		"total_vencidos":   math.Round(totalVencidos*100) / 100,
		"total_por_vencer": math.Round(totalPorVencer*100) / 100,
		"cartera":          result,
	})
}

func (s *Server) handleAuditTrail(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	entries, total := s.db.GetAuditTrail(page, 50)
	jsonResponse(w, map[string]interface{}{"entries": entries, "total": total, "page": page})
}

func (s *Server) handleChangeHistory(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	key := r.URL.Query().Get("key")
	if table == "" || key == "" {
		jsonError(w, "table and key required", 400)
		return
	}
	entries := s.db.GetChangeHistory(table, key, 50)
	jsonResponse(w, map[string]interface{}{"entries": entries})
}

func (s *Server) handleSyncStatsHistory(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	entries := s.db.GetSyncStats(hours)
	jsonResponse(w, map[string]interface{}{"entries": entries})
}

// ==================== BACKUP / RESTORE ====================

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(r, "root", "admin") {
		jsonError(w, "Solo admin/root", 403)
		return
	}
	username := s.getUsernameFromRequest(r)
	s.db.AddAudit(username, "backup", "", "", "Database backup downloaded")

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=siigo_web_backup.db")

	// Use SQLite backup via VACUUM INTO
	tmpPath := "siigo_web_backup_tmp.db"
	os.Remove(tmpPath)
	if err := s.db.VacuumInto(tmpPath); err != nil {
		jsonError(w, "Backup failed: "+err.Error(), 500)
		return
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		jsonError(w, "Cannot read backup", 500)
		return
	}
	w.Write(data)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	if !s.requireRole(r, "root", "admin") {
		jsonError(w, "Solo admin/root", 403)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "File required", 400)
		return
	}
	defer file.Close()

	// Read uploaded file
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(file); err != nil {
		jsonError(w, "Cannot read file", 400)
		return
	}
	fileData := buf.Bytes()

	// Validate SQLite magic bytes
	if len(fileData) < 16 || string(fileData[:15]) != "SQLite format 3" {
		jsonError(w, "Invalid SQLite file", 400)
		return
	}

	username := s.getUsernameFromRequest(r)
	s.db.AddAudit(username, "restore", "", "", "Database restored from upload")

	// Write to temp, then replace
	tmpPath := "siigo_web_restore_tmp.db"
	if err := os.WriteFile(tmpPath, fileData, 0644); err != nil {
		jsonError(w, "Cannot write file", 500)
		return
	}

	// Close current DB, replace, reopen
	s.db.Close()
	os.Rename(tmpPath, "siigo_web.db")
	newDB, err := storage.NewDB("siigo_web.db")
	if err != nil {
		jsonError(w, "Cannot reopen DB: "+err.Error(), 500)
		return
	}
	s.db = newDB
	jsonResponse(w, map[string]string{"status": "ok"})
}

// ==================== SSE (Server-Sent Events) ====================

type sseClient struct {
	ch chan string
}

var (
	sseClients   = make(map[*sseClient]bool)
	sseClientsMu gosync.Mutex
)

func sseNotify(event, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	for c := range sseClients {
		select {
		case c.ch <- msg:
		default:
			// Drop if buffer full
		}
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "Streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{ch: make(chan string, 10)}
	sseClientsMu.Lock()
	sseClients[client] = true
	sseClientsMu.Unlock()

	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, client)
		sseClientsMu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// ==================== BULK ACTIONS ====================

func (s *Server) handleBulkAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}

	var body struct {
		Table  string  `json:"table"`
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"` // "delete", "retry", "reset"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "JSON invalido", 400)
		return
	}
	if len(body.IDs) == 0 {
		jsonError(w, "No IDs provided", 400)
		return
	}

	username := s.getUsernameFromRequest(r)

	switch body.Action {
	case "delete":
		count := s.db.BulkDelete(body.Table, body.IDs)
		s.db.AddAudit(username, "bulk_delete", body.Table, fmt.Sprintf("%d records", count), "")
		jsonResponse(w, map[string]interface{}{"status": "ok", "deleted": count})
	case "retry":
		count := s.db.BulkUpdateStatus(body.Table, body.IDs, "pending")
		s.db.AddAudit(username, "bulk_retry", body.Table, fmt.Sprintf("%d records", count), "")
		jsonResponse(w, map[string]interface{}{"status": "ok", "updated": count})
	case "reset":
		count := s.db.BulkUpdateStatus(body.Table, body.IDs, "pending")
		s.db.AddAudit(username, "bulk_reset", body.Table, fmt.Sprintf("%d records", count), "")
		jsonResponse(w, map[string]interface{}{"status": "ok", "updated": count})
	default:
		jsonError(w, "Invalid action", 400)
	}
}

// ==================== WEBHOOKS ====================

func (s *Server) webhookDispatch(event string, data interface{}) {
	if !s.cfg.Webhooks.Enabled || len(s.cfg.Webhooks.Hooks) == 0 {
		return
	}
	payload := map[string]interface{}{
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	for _, hook := range s.cfg.Webhooks.Hooks {
		if !hook.Active {
			continue
		}
		// Check if this hook is subscribed to this event
		subscribed := false
		for _, ev := range hook.Events {
			if ev == event || ev == "*" {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}
		go s.sendWebhook(hook, body)
	}
}

func (s *Server) sendWebhookWithResult(hook config.WebhookDef, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("error creando request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SiigoSync-Webhook/1.0")

	if hook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(hook.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error enviando: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook respondio HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *Server) sendWebhook(hook config.WebhookDef, body []byte) {
	if err := s.sendWebhookWithResult(hook, body); err != nil {
		s.db.AddLog("error", "WEBHOOK", fmt.Sprintf("%s: %v", hook.URL, err))
	}
}

func (s *Server) handleWebhookConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// Return webhook config (mask secrets)
		result := struct {
			Enabled bool              `json:"enabled"`
			Hooks   []config.WebhookDef `json:"hooks"`
		}{
			Enabled: s.cfg.Webhooks.Enabled,
			Hooks:   s.cfg.Webhooks.Hooks,
		}
		if result.Hooks == nil {
			result.Hooks = []config.WebhookDef{}
		}
		jsonResponse(w, result)
	case "POST":
		var body struct {
			Enabled *bool               `json:"enabled,omitempty"`
			Hooks   *[]config.WebhookDef `json:"hooks,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		if body.Enabled != nil {
			s.cfg.Webhooks.Enabled = *body.Enabled
		}
		if body.Hooks != nil {
			s.cfg.Webhooks.Hooks = *body.Hooks
		}
		s.cfg.Save("config.json")
		username := s.getUsernameFromRequest(r)
		s.db.AddAudit(username, "config_update", "webhooks", "", "")
		jsonResponse(w, map[string]string{"status": "ok"})
	default:
		jsonError(w, "GET or POST only", 405)
	}
}

func (s *Server) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	var body struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		jsonError(w, "URL requerida", 400)
		return
	}
	hook := config.WebhookDef{URL: body.URL, Secret: body.Secret, Events: []string{"test"}, Active: true}
	payload, _ := json.Marshal(map[string]interface{}{
		"event":     "test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      map[string]string{"message": "Test webhook from Siigo Sync"},
	})
	if err := s.sendWebhookWithResult(hook, payload); err != nil {
		jsonResponse(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	jsonResponse(w, map[string]string{"status": "ok", "message": "Webhook de prueba enviado exitosamente"})
}

// ==================== SERVER INFO (LAN IP) ====================

func getLANIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	port := "3210"
	if s.cfg.Server.Port != "" {
		port = s.cfg.Server.Port
	}
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	lanIPs := getLANIPs()
	urls := make([]string, 0, len(lanIPs))
	for _, ip := range lanIPs {
		urls = append(urls, fmt.Sprintf("http://%s:%s", ip, port))
	}

	tunnelURL := GetCurrentTunnelURL()
	if tunnelURL == "" {
		tunnelURL = readTunnelURL() // fallback to old file-based detection
	}

	info := map[string]interface{}{
		"port":       port,
		"lan_ips":    lanIPs,
		"lan_urls":   urls,
		"tunnel_url": tunnelURL,
		"odata_paths": []string{"/odata"},
		"api_v1_paths": []string{"/api/v1/docs", "/api/v1/auth"},
	}
	jsonResponse(w, info)
}

// ==================== USER PREFERENCES ====================

func (s *Server) handleUserPrefs(w http.ResponseWriter, r *http.Request) {
	username := s.getUsernameFromRequest(r)
	key := r.URL.Query().Get("key")
	if key == "" {
		key = "dashboard"
	}

	if r.Method == "GET" {
		val := s.db.GetUserPref(username, key)
		if val == "" {
			val = "{}"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, val)
		return
	}

	if r.Method == "POST" || r.Method == "PUT" {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		if err := s.db.SetUserPref(username, key, string(body)); err != nil {
			jsonError(w, "Error guardando preferencias", 500)
			return
		}
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}

	jsonError(w, "GET o POST", 405)
}

// ==================== USER MANAGEMENT ====================

func (s *Server) requireRole(r *http.Request, roles ...string) bool {
	token := extractToken(r)
	info := s.getTokenInfo(token)
	if info == nil {
		return false
	}
	for _, role := range roles {
		if info.Role == role {
			return true
		}
	}
	return false
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(r, "root", "admin") {
		jsonError(w, "Solo administradores pueden gestionar usuarios", 403)
		return
	}

	switch r.Method {
	case "GET":
		users, err := s.db.ListAppUsers()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		jsonResponse(w, map[string]interface{}{
			"users":       users,
			"all_modules": AllModules,
		})
	case "POST":
		var body struct {
			Username    string   `json:"username"`
			Password    string   `json:"password"`
			Role        string   `json:"role"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		if body.Username == "" || body.Password == "" {
			jsonError(w, "Usuario y contrasena son requeridos", 400)
			return
		}
		if body.Username == s.cfg.Auth.Username {
			jsonError(w, "No se puede crear un usuario con el mismo nombre que el root", 400)
			return
		}
		if body.Role == "" {
			body.Role = "viewer"
		}
		if body.Permissions == nil {
			body.Permissions = []string{"dashboard"}
		}
		if err := s.db.CreateAppUser(body.Username, body.Password, body.Role, body.Permissions); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				jsonError(w, "El usuario ya existe", 400)
			} else {
				jsonError(w, err.Error(), 500)
			}
			return
		}
		s.db.AddLog("info", "USERS", fmt.Sprintf("Usuario '%s' creado con rol '%s'", body.Username, body.Role))
		jsonResponse(w, map[string]string{"status": "ok"})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(r, "root", "admin") {
		jsonError(w, "Solo administradores pueden gestionar usuarios", 403)
		return
	}

	// Extract ID from /api/users/{id}
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		jsonError(w, "ID requerido", 400)
		return
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		jsonError(w, "ID invalido", 400)
		return
	}

	switch r.Method {
	case "PUT":
		var body struct {
			Role        string   `json:"role"`
			Permissions []string `json:"permissions"`
			Active      *bool    `json:"active"`
			Password    string   `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "JSON invalido", 400)
			return
		}
		user, err := s.db.GetAppUserByID(id)
		if err != nil {
			jsonError(w, "Usuario no encontrado", 404)
			return
		}
		role := user.Role
		if body.Role != "" {
			role = body.Role
		}
		perms := user.Permissions
		if body.Permissions != nil {
			perms = body.Permissions
		}
		active := user.Active
		if body.Active != nil {
			active = *body.Active
		}
		if err := s.db.UpdateAppUser(id, role, perms, active); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if body.Password != "" {
			s.db.UpdateAppUserPassword(id, body.Password)
		}
		s.db.AddLog("info", "USERS", fmt.Sprintf("User '%s' updated", user.Username))
		jsonResponse(w, map[string]string{"status": "ok"})
	case "DELETE":
		user, err := s.db.GetAppUserByID(id)
		if err != nil {
			jsonError(w, "Usuario no encontrado", 404)
			return
		}
		if err := s.db.DeleteAppUser(id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "USERS", fmt.Sprintf("User '%s' deleted", user.Username))
		jsonResponse(w, map[string]string{"status": "ok"})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// ==================== TELEGRAM BOT COMMANDS ====================

func (s *Server) registerBotCommands() {
	s.bot.RegisterCommand("/status", func(args string) string {
		detectState := "En espera"
		if s.detecting {
			detectState = "Leyendo ISAM..."
		}
		sendState := "En espera"
		if s.sending {
			sendState = "Enviando al API..."
		}
		if s.sendPaused {
			sendState = fmt.Sprintf("AUTO-PAUSED (%d fallos)", s.sendFailCount)
		}
		if s.paused {
			detectState = "PAUSED"
			sendState = "PAUSED"
		}
		uptime := time.Since(s.startTime).Round(time.Second)
		stats := s.db.GetStats()

		totalPending := toInt(stats["clients_pending"]) + toInt(stats["products_pending"]) + toInt(stats["movements_pending"]) + toInt(stats["cartera_pending"])
		totalErrors := toInt(stats["clients_errors"]) + toInt(stats["products_errors"]) + toInt(stats["movements_errors"]) + toInt(stats["cartera_errors"])

		msg := fmt.Sprintf("📡 <b>Server status</b>\n\n📥 Detection: %s (every %ds)\n📤 Send: %s (every %ds)\n⏱ Uptime: %s\n⏳ Pending: %d\n❌ Errors: %d",
			detectState, s.cfg.Sync.IntervalSeconds,
			sendState, s.cfg.Sync.SendIntervalSeconds,
			uptime, totalPending, totalErrors)
		if s.sendPaused {
			msg += "\n\n⚠️ Usa /send-resume para reactivar el envio"
		}
		return msg
	})

	s.bot.RegisterCommand("/stats", func(args string) string {
		stats := s.db.GetStats()
		return fmt.Sprintf("📊 <b>Statistics</b>\n\n👤 Clients: %v total, %v sync, %v pend, %v err\n📦 Products: %v total, %v sync, %v pend, %v err\n📋 Movements: %v total, %v sync, %v pend, %v err\n💰 Cartera: %v total, %v sync, %v pend, %v err",
			stats["clients_total"], stats["clients_synced"], stats["clients_pending"], stats["clients_errors"],
			stats["products_total"], stats["products_synced"], stats["products_pending"], stats["products_errors"],
			stats["movements_total"], stats["movements_synced"], stats["movements_pending"], stats["movements_errors"],
			stats["cartera_total"], stats["cartera_synced"], stats["cartera_pending"], stats["cartera_errors"],
		)
	})

	s.bot.RegisterCommand("/errors", func(args string) string {
		errors := s.db.GetErrorSummary()
		if len(errors) == 0 {
			return "✅ <b>No errors</b>\n\nNo error records found."
		}
		msg := "❌ <b>Resumen de errores</b>\n"
		for _, e := range errors {
			msg += fmt.Sprintf("\n📋 <b>%s</b> (%d)\n<code>%s</code>", e.Table, e.Count, truncateStr(e.Error, 100))
		}
		return msg
	})

	s.bot.RegisterCommand("/sync", func(args string) string {
		if s.detecting || s.sending {
			return "⏳ Sync already in progress."
		}
		go func() {
			s.doDetectCycle()
			s.doSendCycle()
		}()
		return "🔄 Manual sync started (detect + send)."
	})

	s.bot.RegisterCommand("/pause", func(args string) string {
		s.paused = true
		s.db.AddLog("info", "SYNC", "Sync paused via Telegram")
		return "⏸ Sync paused."
	})

	s.bot.RegisterCommand("/resume", func(args string) string {
		s.paused = false
		s.sendPaused = false
		s.sendFailCount = 0
		s.db.AddLog("info", "SYNC", "Sync resumed via Telegram")
		return "▶️ Sync resumed (detect + send)."
	})

	s.bot.RegisterCommand("/send-resume", func(args string) string {
		if !s.sendPaused {
			return "✅ Send is already active, not paused."
		}
		s.sendPaused = false
		s.sendFailCount = 0
		s.db.AddLog("info", "SEND", "Send reactivated via Telegram (circuit breaker reset)")
		return "▶️ Send reactivated. Failure counter reset to 0."
	})

	s.bot.RegisterCommand("/retry", func(args string) string {
		tables := []string{"clients", "products", "movements", "cartera"}
		total := 0
		for _, t := range tables {
			n := s.db.RetryErrors(t)
			total += n
		}
		if total == 0 {
			return "✅ No hay errores para reintentar."
		}
		s.db.AddLog("info", "SYNC", fmt.Sprintf("Retrying %d errors via Telegram", total))
		return fmt.Sprintf("🔄 %d %d records moved to pending for retry.", total)
	})

	s.bot.RegisterCommand("/url", func(args string) string {
		port := "3210"
		if s.cfg.Server.Port != "" {
			port = s.cfg.Server.Port
		}
		msg := fmt.Sprintf("🌐 <b>URLs</b>\n\n🖥 Local: http://localhost:%s", port)
		if ip := getLocalIP(); ip != "" {
			msg += fmt.Sprintf("\n🏠 LAN: http://%s:%s", ip, port)
		}
		if tunnelURL := readTunnelURL(); tunnelURL != "" {
			msg += fmt.Sprintf("\n🌍 Public: %s", tunnelURL)
			msg += fmt.Sprintf("\n📄 Swagger: %s/api/v1/docs", tunnelURL)
		}
		return msg
	})

	s.bot.RegisterCommand("/logs", func(args string) string {
		logs, _, err := s.db.GetLogs(10, 0)
		if err != nil || len(logs) == 0 {
			return "📝 No hay logs disponibles."
		}
		msg := "📝 <b>Ultimos logs</b>\n"
		for _, l := range logs {
			icon := "ℹ️"
			if l.Level == "error" {
				icon = "❌"
			} else if l.Level == "warning" {
				icon = "⚠️"
			}
			msg += fmt.Sprintf("\n%s [%s] %s", icon, l.Source, truncateStr(l.Message, 80))
		}
		return msg
	})

	s.bot.RegisterCommand("/health", func(args string) string {
		uptime := time.Since(s.startTime).Round(time.Second)
		stats := s.db.GetStats()
		totalRecords := toInt(stats["clients_total"]) + toInt(stats["products_total"]) + toInt(stats["movements_total"]) + toInt(stats["cartera_total"])

		status := "🟢 OK"
		if s.paused {
			status = "🟡 PAUSED"
		}

		return fmt.Sprintf("%s\n\n⏱ Uptime: %s\n📊 Records: %d\n📥 Detecting: %v\n📤 Sending: %v\n⏸ Paused: %v", status, uptime, totalRecords, s.detecting, s.sending, s.paused)
	})

	s.bot.RegisterCommand("/exec", func(args string) string {
		pin := s.cfg.Telegram.ExecPin
		if pin == "" {
			return "⛔ PIN not configured. Set exec_pin in Telegram Config."
		}
		parts := strings.SplitN(args, " ", 2)
		if len(parts) < 2 {
			return "Uso: /exec {pin} {comando}\nEjemplo: /exec 2337 ls -la"
		}
		if parts[0] != pin {
			return "🔴 PIN incorrecto."
		}
		cmdStr := parts[1]

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
		cmd.Dir = `C:\Users\lordmacu\siigo`
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n" + stderr.String()
		}
		if err != nil {
			output += "\n⚠️ " + err.Error()
		}
		output = strings.TrimSpace(output)
		if output == "" {
			output = "(sin output)"
		}
		if len(output) > 3900 {
			output = output[:3900] + "\n... (truncado)"
		}

		return fmt.Sprintf("💻 <b>$ %s</b>\n\n<code>%s</code>", truncateStr(cmdStr, 100), output)
	})

	s.bot.RegisterCommand("/claude", func(args string) string {
		pin := s.cfg.Telegram.ExecPin
		if pin == "" {
			return "⛔ PIN no configurado."
		}
		if strings.TrimSpace(args) != pin {
			return "Uso: /claude {pin}"
		}

		cmd := exec.Command("bash", "-c", "claude --dangerously-skip-permissions &")
		cmd.Dir = `C:\Users\lordmacu\siigo`
		if err := cmd.Start(); err != nil {
			return "❌ Error iniciando Claude: " + err.Error()
		}

		return "🤖 <b>Claude started</b>\n\nProcess launched in background.\nCheck server terminal for connection URL."
	})
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func handleSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, "swagger.json")
}

// handlePostmanCollection generates a Postman v2.1 collection dynamically from the current routes.
func (s *Server) handlePostmanCollection(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	baseURL := scheme + "://" + host

	// Table metadata for descriptions
	tableDescriptions := map[string]string{
		"clients": "Clientes / Terceros (Z08A)", "products": "Productos / Inventario (Z04)",
		"cartera": "Cartera / CxC (Z09)",
		"documentos": "Documentos / Facturas (Z11)",
		"condiciones_pago": "Condiciones de Pago (Z05)",
		"codigos_dane": "Codigos DANE Municipios",
		"formulas": "Formulas/Recetas (Z06 tipo R)",
		"vendedores_areas": "Vendedores/Areas (Z06A)",
	}

	type pmKV struct {
		Key   string `json:"key"`
		Value string `json:"value"`
		Type  string `json:"type,omitempty"`
	}
	type pmURL struct {
		Raw   string   `json:"raw"`
		Host  []string `json:"host"`
		Path  []string `json:"path"`
		Query []pmKV   `json:"query,omitempty"`
	}
	type pmBody struct {
		Mode string `json:"mode"`
		Raw  string `json:"raw"`
		Opts map[string]interface{} `json:"options,omitempty"`
	}
	type pmReq struct {
		Method string `json:"method"`
		Header []pmKV `json:"header"`
		URL    pmURL  `json:"url"`
		Body   *pmBody `json:"body,omitempty"`
	}
	type pmItem struct {
		Name    string   `json:"name"`
		Request *pmReq   `json:"request,omitempty"`
		Item    []pmItem `json:"item,omitempty"`
	}
	type pmCollection struct {
		Info struct {
			Name   string `json:"name"`
			Schema string `json:"schema"`
			Desc   string `json:"description"`
		} `json:"info"`
		Variable []pmKV   `json:"variable"`
		Item     []pmItem `json:"item"`
	}

	makeURL := func(path string, query ...pmKV) pmURL {
		return pmURL{
			Raw:   "{{base_url}}" + path,
			Host:  []string{"{{base_url}}"},
			Path:  strings.Split(strings.TrimPrefix(path, "/"), "/"),
			Query: query,
		}
	}
	authHeader := []pmKV{
		{Key: "Authorization", Value: "Bearer {{token}}", Type: "text"},
		{Key: "Content-Type", Value: "application/json", Type: "text"},
	}
	jsonOpts := map[string]interface{}{"raw": map[string]string{"language": "json"}}

	// --- Auth folder ---
	authFolder := pmItem{Name: "Auth", Item: []pmItem{
		{Name: "Login (API Key)", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/v1/auth"),
			Body: &pmBody{Mode: "raw", Raw: `{"api_key": "tu-clave-secreta"}`, Opts: jsonOpts}}},
		{Name: "Login (Credentials)", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/v1/auth"),
			Body: &pmBody{Mode: "raw", Raw: `{"username": "admin", "password": "tu-password"}`, Opts: jsonOpts}}},
		{Name: "Dashboard Login", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/login"),
			Body: &pmBody{Mode: "raw", Raw: `{"username": "admin", "password": "tu-password"}`, Opts: jsonOpts}}},
	}}

	// --- Stats folder ---
	statsFolder := pmItem{Name: "Stats & Status", Item: []pmItem{
		{Name: "Stats Summary", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/v1/stats")}},
		{Name: "Sync Status", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/sync-status")}},
		{Name: "Server Info", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/server-info")}},
		{Name: "ISAM Info", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/isam-info")}},
		{Name: "EXTFH Status", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/extfh-status")}},
	}}

	// --- API v1 Data folders (one per table) ---
	var dataItems []pmItem
	for _, table := range odataTableOrder {
		desc := tableDescriptions[table]
		if desc == "" {
			desc = table
		}
		folder := pmItem{Name: desc, Item: []pmItem{
			{Name: "List " + table, Request: &pmReq{Method: "GET", Header: authHeader,
				URL: makeURL("/api/v1/"+table, pmKV{Key: "page", Value: "1"}, pmKV{Key: "search", Value: ""})}},
			{Name: "Detail " + table, Request: &pmReq{Method: "GET", Header: authHeader,
				URL: makeURL("/api/v1/" + table + "/EXAMPLE_KEY")}},
		}}
		dataItems = append(dataItems, folder)
	}
	dataFolder := pmItem{Name: "API v1 - Data Tables (13)", Item: dataItems}

	// --- OData folder ---
	odataFolder := pmItem{Name: "OData v4 (Power BI)", Item: []pmItem{
		{Name: "Service Document", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/odata")}},
		{Name: "Metadata (CSDL)", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/odata/$metadata")}},
		{Name: "Query clients (example)", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/odata/clients", pmKV{Key: "$top", Value: "10"}, pmKV{Key: "$count", Value: "true"})}},
		{Name: "Filter example", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/odata/clients", pmKV{Key: "$filter", Value: "contains(nombre,'empresa')"}, pmKV{Key: "$top", Value: "50"})}},
		{Name: "Entity by key", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/odata/clients('900123456')")}},
		{Name: "Count only", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/odata/clients/$count")}},
	}}

	// --- Sync Control folder ---
	syncFolder := pmItem{Name: "Sync Control", Item: []pmItem{
		{Name: "Sync Now (trigger detect)", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/sync-now")}},
		{Name: "Pause Sync", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/pause")}},
		{Name: "Resume Sync", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/resume")}},
		{Name: "Resume Send (circuit breaker)", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/send-resume")}},
		{Name: "Retry Errors", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/retry-errors"),
			Body: &pmBody{Mode: "raw", Raw: `{"table": "clients"}`, Opts: jsonOpts}}},
		{Name: "Get Send Enabled", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/send-enabled")}},
		{Name: "Set Send Enabled", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/send-enabled"),
			Body: &pmBody{Mode: "raw", Raw: `{"clients": true, "products": false}`, Opts: jsonOpts}}},
		{Name: "Get Detect Enabled", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/detect-enabled")}},
		{Name: "Set Detect Enabled", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/detect-enabled"),
			Body: &pmBody{Mode: "raw", Raw: `{"clients": true, "products": false}`, Opts: jsonOpts}}},
	}}

	// --- Config folder ---
	configFolder := pmItem{Name: "Configuration", Item: []pmItem{
		{Name: "Get Config", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/config")}},
		{Name: "Save Config", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/config"),
			Body: &pmBody{Mode: "raw", Raw: `{"data_path": "C:\\\\SIIWI02", "interval": 60}`, Opts: jsonOpts}}},
		{Name: "Get Field Mappings", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/field-mappings")}},
		{Name: "Test Connection", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/test-connection")}},
		{Name: "Get Telegram Config", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/telegram-config")}},
		{Name: "Get Public API Config", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/public-api-config")}},
		{Name: "Allow Edit/Delete", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/allow-edit-delete")}},
	}}

	// --- Data Management folder ---
	mgmtFolder := pmItem{Name: "Data Management", Item: []pmItem{
		{Name: "SQL Query", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/query"),
			Body: &pmBody{Mode: "raw", Raw: `{"query": "SELECT * FROM clients LIMIT 10", "limit": 10, "offset": 0}`, Opts: jsonOpts}}},
		{Name: "Error Summary", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/error-summary")}},
		{Name: "Logs", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/api/logs", pmKV{Key: "page", Value: "1"})}},
		{Name: "Sync History", Request: &pmReq{Method: "GET", Header: authHeader,
			URL: makeURL("/api/sync-history", pmKV{Key: "table", Value: "clients"}, pmKV{Key: "page", Value: "1"})}},
		{Name: "Clear Database", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/clear-database")}},
		{Name: "Backup URL", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/backup")}},
	}}

	// --- Users folder ---
	usersFolder := pmItem{Name: "User Management", Item: []pmItem{
		{Name: "List Users", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/users")}},
		{Name: "Create User", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/users"),
			Body: &pmBody{Mode: "raw", Raw: `{"username": "nuevo", "password": "pass123", "role": "editor", "permissions": ["dashboard","clients","products"]}`, Opts: jsonOpts}}},
	}}

	col := pmCollection{}
	col.Info.Name = "Siigo Sync API"
	col.Info.Schema = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
	col.Info.Desc = fmt.Sprintf("Coleccion generada automaticamente desde Siigo Sync Middleware.\n\nBase URL: %s\n\n27 tablas de datos de Siigo Pyme con sync bidireccional, OData v4 para Power BI, y API REST completa.", baseURL)
	col.Variable = []pmKV{
		{Key: "base_url", Value: baseURL, Type: "string"},
		{Key: "token", Value: "", Type: "string"},
	}
	col.Item = []pmItem{authFolder, statsFolder, dataFolder, odataFolder, syncFolder, configFolder, mgmtFolder, usersFolder}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=siigo-sync-postman.json")
	json.NewEncoder(w).Encode(col)
}

func handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8">
  <title>Siigo Sync - API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #fafafa; }
    .topbar { display: none !important; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: '/api/v1/swagger.json',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: 'BaseLayout',
    });
  </script>
</body>
</html>`)
}
