package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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

	"github.com/golang-jwt/jwt/v5"
)

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
}

func main() {
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

	db.AddLog("info", "APP", "Servidor web iniciado")

	srv.registerBotCommands()
	bot.StartPolling()

	if srv.cfg.SetupComplete {
		go srv.startSyncLoop()
	} else {
		db.AddLog("info", "APP", "Setup pendiente — sync loops deshabilitados hasta completar wizard")
	}

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// Serve React static files
	frontendDir := "frontend/dist"
	if _, err := os.Stat(frontendDir); err == nil {
		fs := http.FileServer(http.Dir(frontendDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// If it's an API call, skip
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// Sanitize path to prevent directory traversal
			cleanPath := filepath.Clean(r.URL.Path)
			if strings.Contains(cleanPath, "..") {
				http.NotFound(w, r)
				return
			}
			// Try to serve the file; if not found, serve index.html (SPA)
			fullPath := filepath.Join(frontendDir, cleanPath)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) && r.URL.Path != "/" {
				http.ServeFile(w, r, filepath.Join(frontendDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, r)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, "Run 'npm run build' in frontend/ to build the UI")
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

	log.Printf("Siigo Web running on http://localhost:%s", port)
	// Notify with tunnel URL if available (tunnel may start after server)
	go func() {
		time.Sleep(5 * time.Second) // give tunnel time to start
		localURL := "http://localhost:" + port
		if tunnelURL := readTunnelURL(); tunnelURL != "" {
			bot.Send(fmt.Sprintf("🟢 <b>Servidor iniciado</b>\n\n🖥 Local: %s\n🌍 Publica: %s\n📄 Swagger: %s/api/v1/docs", localURL, tunnelURL, tunnelURL))
		} else {
			bot.NotifyServerStarted(localURL)
		}
	}()

	// Graceful shutdown
	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: corsMiddleware(mux),
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		<-sigCh
		log.Println("Shutdown signal received, stopping...")
		db.AddLog("info", "APP", "Servidor detenido (shutdown graceful)")

		// Stop sync loops
		select {
		case srv.stopCh <- true:
		default:
		}
		select {
		case srv.stopCh <- true: // one for each loop
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	}()

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
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
	"dashboard", "clients", "products", "movements", "cartera",
	"plan_cuentas", "activos_fijos", "saldos_terceros", "saldos_consolidados",
	"documentos", "terceros_ampliados",
	"field-mappings", "errors", "logs", "explorer", "config", "users",
}

// apiPathToModule maps API endpoint paths to their required module permission.
// Routes not listed here are accessible to any authenticated user.
var apiPathToModule = map[string]string{
	"/api/clients":              "clients",
	"/api/products":             "products",
	"/api/movements":            "movements",
	"/api/cartera":              "cartera",
	"/api/plan-cuentas":         "plan_cuentas",
	"/api/activos-fijos":        "activos_fijos",
	"/api/saldos-terceros":      "saldos_terceros",
	"/api/saldos-consolidados":  "saldos_consolidados",
	"/api/documentos":           "documentos",
	"/api/terceros-ampliados":      "terceros_ampliados",
	"/api/movimientos-inventario":  "movimientos_inventario",
	"/api/saldos-inventario":       "saldos_inventario",
	"/api/activos-fijos-detalle":   "activos_fijos_detalle",
	"/api/audit-trail-terceros":    "audit_trail_terceros",
	"/api/transacciones-detalle":    "transacciones_detalle",
	"/api/periodos-contables":       "periodos_contables",
	"/api/condiciones-pago":         "condiciones_pago",
	"/api/libros-auxiliares":        "libros_auxiliares",
	"/api/codigos-dane":             "codigos_dane",
	"/api/actividades-ica":          "actividades_ica",
	"/api/conceptos-pila":           "conceptos_pila",
	"/api/clasificacion-cuentas":    "clasificacion_cuentas",
	"/api/historial":               "historial",
	"/api/maestros":                "maestros",
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
			jsonError(w, "No autorizado", 401)
			return
		}
		if !hasModulePermission(info, module) {
			jsonError(w, "Sin permiso para este modulo", 403)
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
		jsonError(w, "Demasiados intentos. Intente de nuevo en 1 minuto.", 429)
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
			jsonError(w, "Credenciales incorrectas", 401)
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
		"token":          token,
		"username":       tokenInfo.Username,
		"role":           tokenInfo.Role,
		"permissions":    tokenInfo.Permissions,
		"setup_complete": s.cfg.SetupComplete,
	})
}

func (s *Server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if !s.isValidToken(token) {
		jsonError(w, "No autorizado", 401)
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
			jsonError(w, "No autorizado", 401)
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
	mux.HandleFunc("/api/isam-info", s.authMiddleware(s.handleISAMInfo))
	mux.HandleFunc("/api/extfh-status", s.authMiddleware(s.handleExtfhStatus))
	mux.HandleFunc("/api/clients", s.permMiddleware(s.handleClients))
	mux.HandleFunc("/api/products", s.permMiddleware(s.handleProducts))
	mux.HandleFunc("/api/movements", s.permMiddleware(s.handleMovements))
	mux.HandleFunc("/api/cartera", s.permMiddleware(s.handleCartera))
	mux.HandleFunc("/api/plan-cuentas", s.permMiddleware(s.handlePlanCuentas))
	mux.HandleFunc("/api/activos-fijos", s.permMiddleware(s.handleActivosFijos))
	mux.HandleFunc("/api/saldos-terceros", s.permMiddleware(s.handleSaldosTerceros))
	mux.HandleFunc("/api/saldos-consolidados", s.permMiddleware(s.handleSaldosConsolidados))
	mux.HandleFunc("/api/documentos", s.permMiddleware(s.handleDocumentos))
	mux.HandleFunc("/api/terceros-ampliados", s.permMiddleware(s.handleTercerosAmpliados))
	mux.HandleFunc("/api/movimientos-inventario", s.permMiddleware(s.handleMovimientosInventario))
	mux.HandleFunc("/api/saldos-inventario", s.permMiddleware(s.handleSaldosInventario))
	mux.HandleFunc("/api/activos-fijos-detalle", s.permMiddleware(s.handleActivosFijosDetalle))
	mux.HandleFunc("/api/audit-trail-terceros", s.permMiddleware(s.handleAuditTrailTerceros))
	mux.HandleFunc("/api/transacciones-detalle", s.permMiddleware(s.handleGenericTable("transacciones_detalle")))
	mux.HandleFunc("/api/periodos-contables", s.permMiddleware(s.handleGenericTable("periodos_contables")))
	mux.HandleFunc("/api/condiciones-pago", s.permMiddleware(s.handleGenericTable("condiciones_pago")))
	mux.HandleFunc("/api/libros-auxiliares", s.permMiddleware(s.handleGenericTable("libros_auxiliares")))
	mux.HandleFunc("/api/codigos-dane", s.permMiddleware(s.handleGenericTable("codigos_dane")))
	mux.HandleFunc("/api/actividades-ica", s.permMiddleware(s.handleGenericTable("actividades_ica")))
	mux.HandleFunc("/api/conceptos-pila", s.permMiddleware(s.handleGenericTable("conceptos_pila")))
	mux.HandleFunc("/api/clasificacion-cuentas", s.permMiddleware(s.handleGenericTable("clasificacion_cuentas")))
	mux.HandleFunc("/api/historial", s.permMiddleware(s.handleGenericTable("historial")))
	mux.HandleFunc("/api/maestros", s.permMiddleware(s.handleGenericTable("maestros")))
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

// ==================== SYNC LOOP ====================

func (s *Server) startSyncLoop() {
	s.paused = false
	s.db.AddLog("info", "SYNC", "Loops iniciados (deteccion + envio independientes)")

	go s.startDetectLoop()
	s.startSendLoop()
}

// startDetectLoop reads ISAM files and updates SQLite independently
func (s *Server) startDetectLoop() {
	interval := time.Duration(s.cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	s.doDetectCycle()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !s.paused && !s.detecting {
				s.doDetectCycle()
			}
		case <-s.stopCh:
			s.db.AddLog("info", "DETECT", "Detect loop detenido")
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
			s.db.AddLog("info", "SEND", "Send loop detenido")
			return
		}
	}
}

func (s *Server) doDetectCycle() {
	s.detecting = true
	defer func() { s.detecting = false }()

	s.db.AddLog("info", "DETECT", "--- Ciclo deteccion iniciado ---")

	// Each diff only runs if detection is enabled for that table
	type detectEntry struct {
		table string
		fn    func()
	}
	detects := []detectEntry{
		{"clients", s.diffClientes},
		{"products", s.diffProductos},
		{"movements", s.diffMovimientos},
		{"cartera", s.diffCartera},
		{"plan_cuentas", s.diffPlanCuentas},
		{"activos_fijos", s.diffActivosFijos},
		{"saldos_terceros", s.diffSaldosTerceros},
		{"saldos_consolidados", s.diffSaldosConsolidados},
		{"documentos", s.diffDocumentos},
		{"terceros_ampliados", s.diffTercerosAmpliados},
		{"transacciones_detalle", s.diffTransaccionesDetalle},
		{"periodos_contables", s.diffPeriodosContables},
		{"condiciones_pago", s.diffCondicionesPago},
		{"libros_auxiliares", s.diffLibrosAuxiliares},
		{"codigos_dane", s.diffCodigosDane},
		{"actividades_ica", s.diffActividadesICA},
		{"conceptos_pila", s.diffConceptosPILA},
		{"activos_fijos_detalle", s.diffActivosFijosDetalle},
		{"audit_trail_terceros", s.diffAuditTrailTerceros},
		{"clasificacion_cuentas", s.diffClasificacionCuentas},
		{"movimientos_inventario", s.diffMovimientosInventario},
		{"saldos_inventario", s.diffSaldosInventario},
		{"historial", s.diffHistorial},
		{"maestros", s.diffMaestros},
	}
	for _, d := range detects {
		if s.cfg.IsDetectEnabled(d.table) {
			d.fn()
		}
	}

	s.db.AddLog("info", "DETECT", "--- Ciclo deteccion completado ---")

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

	// Webhook notification
	s.webhookDispatch("sync_complete", stats)
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

	s.db.AddLog("info", "SEND", "--- Ciclo envio iniciado ---")

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
			s.db.AddLog("info", "SEND", fmt.Sprintf("Envio recuperado (fallos consecutivos reseteados de %d a 0)", s.sendFailCount))
		}
		s.sendFailCount = 0
	}

	s.db.AddLog("info", "SEND", "--- Ciclo envio completado ---")

	// SSE notification
	sseNotify("send_complete", fmt.Sprintf(`{"sent":%d,"errors":%d}`, totalSent, totalErrors))

	// Webhook notification
	s.webhookDispatch("send_complete", map[string]int{"sent": totalSent, "errors": totalErrors})
}

func (s *Server) checkSendCircuitBreaker(reason string) {
	threshold := s.cfg.Sync.GetCircuitBreakerThreshold()
	if s.sendFailCount >= threshold && !s.sendPaused {
		s.sendPaused = true
		msg := fmt.Sprintf("🔴 <b>Envio auto-pausado</b>\n\n%d fallos consecutivos.\nUltimo: %s\n\nRevisa el servidor de destino y reactiva el envio con /send-resume o desde la web.",
			s.sendFailCount, reason)
		s.bot.Send(msg)
		s.db.AddLog("error", "SEND", fmt.Sprintf("Circuit breaker activado: %d fallos consecutivos (%s). Envio pausado automaticamente.", s.sendFailCount, reason))
		s.webhookDispatch("send_paused", map[string]interface{}{"reason": reason, "fail_count": s.sendFailCount})
	} else if s.sendFailCount > 0 && !s.sendPaused {
		s.db.AddLog("warn", "SEND", fmt.Sprintf("Fallo de envio %d/%d (%s)", s.sendFailCount, threshold, reason))
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
	{"clients", "Clientes (Z17)"},
	{"products", "Productos (Z04)"},
	{"movements", "Movimientos (Z49)"},
	{"cartera", "Cartera (Z09)"},
	{"plan_cuentas", "Plan de Cuentas (Z03)"},
	{"activos_fijos", "Activos Fijos (Z27)"},
	{"saldos_terceros", "Saldos por Tercero (Z25)"},
	{"saldos_consolidados", "Saldos Consolidados (Z28)"},
	{"documentos", "Documentos (Z11)"},
	{"terceros_ampliados", "Terceros Ampliados (Z08A)"},
	{"transacciones_detalle", "Transacciones Detalle (Z07T)"},
	{"periodos_contables", "Periodos Contables (Z26)"},
	{"condiciones_pago", "Condiciones de Pago (Z05)"},
	{"libros_auxiliares", "Libros Auxiliares (Z07)"},
	{"codigos_dane", "Codigos DANE"},
	{"actividades_ica", "Actividades ICA"},
	{"conceptos_pila", "Conceptos PILA"},
	{"activos_fijos_detalle", "Activos Fijos Detalle (Z27)"},
	{"audit_trail_terceros", "Audit Trail Terceros (Z11N)"},
	{"clasificacion_cuentas", "Clasificacion Cuentas (Z279CP)"},
	{"movimientos_inventario", "Movimientos Inventario (Z16)"},
	{"saldos_inventario", "Saldos Inventario (Z15)"},
	{"historial", "Historial Documentos (Z18)"},
	{"maestros", "Maestros Config (Z06)"},
}

func (s *Server) getDiffFunc(table string) func() {
	switch table {
	case "clients":
		return s.diffClientes
	case "products":
		return s.diffProductos
	case "movements":
		return s.diffMovimientos
	case "cartera":
		return s.diffCartera
	case "plan_cuentas":
		return s.diffPlanCuentas
	case "activos_fijos":
		return s.diffActivosFijos
	case "saldos_terceros":
		return s.diffSaldosTerceros
	case "saldos_consolidados":
		return s.diffSaldosConsolidados
	case "documentos":
		return s.diffDocumentos
	case "terceros_ampliados":
		return s.diffTercerosAmpliados
	case "transacciones_detalle":
		return s.diffTransaccionesDetalle
	case "periodos_contables":
		return s.diffPeriodosContables
	case "condiciones_pago":
		return s.diffCondicionesPago
	case "libros_auxiliares":
		return s.diffLibrosAuxiliares
	case "codigos_dane":
		return s.diffCodigosDane
	case "actividades_ica":
		return s.diffActividadesICA
	case "conceptos_pila":
		return s.diffConceptosPILA
	case "activos_fijos_detalle":
		return s.diffActivosFijosDetalle
	case "audit_trail_terceros":
		return s.diffAuditTrailTerceros
	case "clasificacion_cuentas":
		return s.diffClasificacionCuentas
	case "movimientos_inventario":
		return s.diffMovimientosInventario
	case "saldos_inventario":
		return s.diffSaldosInventario
	case "historial":
		return s.diffHistorial
	case "maestros":
		return s.diffMaestros
	}
	return nil
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
		jsonError(w, "Setup ya completado", 400)
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
		diffFn()
		var count int
		s.db.QueryCount(req.Table, &count)
		s.setupPopulated[req.Table] = true
		s.setupPopulating = ""
		data, _ := json.Marshal(map[string]interface{}{"table": req.Table, "count": count})
		sseNotify("setup_table_done", string(data))
		s.db.AddLog("info", "SETUP", fmt.Sprintf("Tabla %s poblada: %d registros", req.Table, count))
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

	s.db.AddLog("info", "SETUP", "Setup wizard completado — sync loops iniciados")
	go s.startSyncLoop()
	sseNotify("setup_complete", "{}")

	jsonResponse(w, map[string]string{"status": "ok"})
}

// ==================== DIFF ====================

func (s *Server) diffClientes() {
	clientes, _, err := parsers.ParseTercerosAmpliados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z08A", "Error parseando: "+err.Error())
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
		action := s.db.UpsertClient(nit, t.Nombre, t.TipoPersona, t.Empresa, t.Direccion, t.Email, t.RepresentanteLegal, t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedClients(currentKeys)
	s.db.AddLog("info", "Z08A", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(clientes)))
	s.bot.NotifyChangesDetected("Clientes", adds, edits, deletes)
}

func (s *Server) diffProductos() {
	productos, year, err := parsers.ParseInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z04", "Error parseando inventario: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(productos))

	for _, p := range productos {
		key := p.Codigo
		if key == "" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertProduct(key, p.Nombre, p.NombreCorto, p.Grupo, p.Referencia, p.Empresa, p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedProducts(currentKeys)
	source := "Z04" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(productos)))
	s.bot.NotifyChangesDetected("Productos", adds, edits, deletes)
}

func (s *Server) diffMovimientos() {
	movimientos, err := parsers.ParseMovimientos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z49", "Error parseando: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(movimientos))

	for _, m := range movimientos {
		key := m.TipoComprobante + "-" + m.NumeroDoc
		if key == "-" {
			key = m.Hash
		}
		currentKeys[key] = true

		desc := m.Descripcion
		if m.Descripcion2 != "" {
			if desc != "" {
				desc += " | " + m.Descripcion2
			} else {
				desc = m.Descripcion2
			}
		}
		// Z49 only has tipo, numero, nombre, descripcion. No fecha/cuenta/valor/tipoMov.
		action := s.db.UpsertMovement(key, m.TipoComprobante, m.Empresa, m.NumeroDoc, "", m.NombreTercero, "", desc, "", "", m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedMovements(currentKeys)
	s.db.AddLog("info", "Z49", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(movimientos)))
	s.bot.NotifyChangesDetected("Movimientos", adds, edits, deletes)
}

func (s *Server) diffCartera() {
	cartera, year, err := parsers.ParseCarteraLatest(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z09", "Error parseando cartera: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(cartera))
	file := "Z09" + year

	for _, c := range cartera {
		key := file + "-" + c.TipoRegistro + "-" + c.Empresa + "-" + c.Secuencia
		currentKeys[key] = true

		fecha := c.Fecha
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		action := s.db.UpsertCartera(key, c.TipoRegistro, c.Empresa, c.Secuencia, c.TipoDoc, c.NitTercero, c.CuentaContable, fecha, c.Descripcion, c.TipoMov, c.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedCartera(currentKeys)
	s.db.AddLog("info", file, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(cartera)))
	s.bot.NotifyChangesDetected("Cartera ("+file+")", adds, edits, deletes)
}

func (s *Server) diffPlanCuentas() {
	cuentas, year, err := parsers.ParsePlanCuentas(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z03", "Error parseando plan de cuentas: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(cuentas))

	for _, c := range cuentas {
		if c.CodigoCuenta == "" {
			continue
		}
		currentKeys[c.CodigoCuenta] = true
		action := s.db.UpsertPlanCuenta(c.CodigoCuenta, c.Nombre, c.Empresa, c.Naturaleza, c.Hash, c.Activa, c.Auxiliar)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedPlanCuentas(currentKeys)
	source := "Z03" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(cuentas)))
	s.bot.NotifyChangesDetected("Plan Cuentas", adds, edits, deletes)
}

func (s *Server) diffActivosFijos() {
	activos, year, err := parsers.ParseActivosFijos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z27", "Error parseando activos fijos: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(activos))

	for _, a := range activos {
		if a.Codigo == "" {
			continue
		}
		currentKeys[a.Codigo] = true
		action := s.db.UpsertActivoFijo(a.Codigo, a.Nombre, a.Empresa, a.NitResponsable, a.FechaAdquisicion, a.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActivosFijos(currentKeys)
	source := "Z27" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(activos)))
	s.bot.NotifyChangesDetected("Activos Fijos", adds, edits, deletes)
}

func (s *Server) diffSaldosTerceros() {
	saldos, year, err := parsers.ParseSaldosTerceros(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z25", "Error parseando saldos terceros: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(saldos))

	for _, st := range saldos {
		key := st.CuentaContable + "-" + st.NitTercero
		if key == "-" {
			continue
		}
		currentKeys[key] = true
		action := s.db.UpsertSaldoTercero(key, st.CuentaContable, st.NitTercero, st.Empresa, st.Hash, st.SaldoAnterior, st.Debito, st.Credito, st.SaldoFinal)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedSaldosTerceros(currentKeys)
	source := "Z25" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(saldos)))
	s.bot.NotifyChangesDetected("Saldos Terceros", adds, edits, deletes)
}

func (s *Server) diffSaldosConsolidados() {
	saldos, year, err := parsers.ParseSaldosConsolidados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z28", "Error parseando saldos consolidados: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(saldos))

	for _, sc := range saldos {
		if sc.CuentaContable == "" {
			continue
		}
		currentKeys[sc.CuentaContable] = true
		action := s.db.UpsertSaldoConsolidado(sc.CuentaContable, sc.Empresa, sc.Hash, sc.SaldoAnterior, sc.Debito, sc.Credito, sc.SaldoFinal)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedSaldosConsolidados(currentKeys)
	source := "Z28" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(saldos)))
	s.bot.NotifyChangesDetected("Saldos Consolidados", adds, edits, deletes)
}

func (s *Server) diffDocumentos() {
	docs, year, err := parsers.ParseDocumentos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z11", "Error parseando documentos: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(docs))

	for _, d := range docs {
		key := d.TipoComprobante + "-" + d.CodigoComp + "-" + d.Secuencia + "-" + d.Hash[:8]
		currentKeys[key] = true
		fecha := d.Fecha
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		action := s.db.UpsertDocumento(key, d.TipoComprobante, d.CodigoComp, d.Secuencia, d.NitTercero, d.CuentaContable, d.ProductoRef, d.Bodega, d.CentroCosto, fecha, d.Descripcion, d.TipoMov, d.Referencia, d.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedDocumentos(currentKeys)
	source := "Z11" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(docs)))
	s.bot.NotifyChangesDetected("Documentos", adds, edits, deletes)
}

func (s *Server) diffTercerosAmpliados() {
	terceros, year, err := parsers.ParseTercerosAmpliados(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z08A", "Error parseando terceros ampliados: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(terceros))

	for _, t := range terceros {
		if t.Nit == "" {
			continue
		}
		currentKeys[t.Nit] = true
		action := s.db.UpsertTerceroAmpliado(t.Nit, t.Nombre, t.Empresa, t.TipoPersona, t.RepresentanteLegal, t.Direccion, t.Email, t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedTercerosAmpliados(currentKeys)
	source := "Z08A" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(terceros)))
	s.bot.NotifyChangesDetected("Terceros Ampliados", adds, edits, deletes)
}

func (s *Server) diffTransaccionesDetalle() {
	items, err := parsers.ParseTransaccionesDetalle(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z07T", "Error parseando transacciones detalle: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, t := range items {
		key := t.TipoComprobante + "-" + t.Empresa + "-" + t.Secuencia + "-" + t.CuentaContable
		if key == "---" {
			key = t.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertTransaccionDetalle(key, t.TipoComprobante, t.Empresa, t.Secuencia, t.NitTercero, t.CuentaContable, t.FechaDocumento, t.FechaVencimiento, t.TipoMovimiento, t.Referencia, t.Hash, t.Valor)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedTransaccionesDetalle(currentKeys)
	s.db.AddLog("info", "Z07T", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffPeriodosContables() {
	items, year, err := parsers.ParsePeriodos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z26", "Error parseando periodos contables: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, p := range items {
		key := p.Empresa + "-" + p.NumeroPeriodo
		if key == "-" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertPeriodoContable(key, p.Empresa, p.NumeroPeriodo, p.FechaInicio, p.FechaFin, p.Estado, p.Hash, p.Saldo1, p.Saldo2, p.Saldo3)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedPeriodosContables(currentKeys)
	source := "Z26" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffCondicionesPago() {
	items, year, err := parsers.ParseCondicionesPago(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z05", "Error parseando condiciones pago: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, c := range items {
		key := c.Tipo + "-" + c.Empresa + "-" + c.Secuencia + "-" + c.NIT
		if key == "---" {
			key = c.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertCondicionPago(key, c.Tipo, c.Empresa, c.Secuencia, c.TipoDoc, c.Fecha, c.NIT, c.TipoSecundario, c.FechaRegistro, c.Hash, c.Valor)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedCondicionesPago(currentKeys)
	source := "Z05" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffLibrosAuxiliares() {
	items, year, err := parsers.ParseLibrosAuxiliares(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z07", "Error parseando libros auxiliares: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, la := range items {
		key := la.TipoComprobante + "-" + la.Empresa + "-" + la.CuentaContable + "-" + la.NitTercero + "-" + la.FechaDocumento
		if key == "----" {
			key = la.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertLibroAuxiliar(key, la.Empresa, la.CuentaContable, la.TipoComprobante, la.CodigoComprobante, la.FechaDocumento, la.NitTercero, la.Hash, la.Saldo, la.Debito, la.Credito)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedLibrosAuxiliares(currentKeys)
	source := "Z07" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffCodigosDane() {
	items, err := parsers.ParseDane(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZDANE", "Error parseando codigos DANE: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, d := range items {
		if d.Codigo == "" {
			continue
		}
		currentKeys[d.Codigo] = true
		action := s.db.UpsertCodigoDane(d.Codigo, d.Nombre, d.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedCodigosDane(currentKeys)
	s.db.AddLog("info", "ZDANE", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffActividadesICA() {
	items, err := parsers.ParseICA(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZICA", "Error parseando actividades ICA: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, a := range items {
		if a.Codigo == "" {
			continue
		}
		currentKeys[a.Codigo] = true
		action := s.db.UpsertActividadICA(a.Codigo, a.Nombre, a.Tarifa, a.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActividadesICA(currentKeys)
	s.db.AddLog("info", "ZICA", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffConceptosPILA() {
	items, err := parsers.ParsePILA(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "ZPILA", "Error parseando conceptos PILA: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, p := range items {
		key := p.Tipo + "-" + p.Fondo + "-" + p.Concepto
		if key == "--" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertConceptoPILA(key, p.Tipo, p.Fondo, p.Concepto, p.Flags, p.TipoBase, p.BaseCalculo, p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedConceptosPILA(currentKeys)
	s.db.AddLog("info", "ZPILA", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffActivosFijosDetalle() {
	items, year, err := parsers.ParseActivosFijosDetalle(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z27D", "Error parseando activos fijos detalle: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, a := range items {
		key := a.Grupo + "-" + a.Secuencia
		if key == "-" {
			key = a.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertActivoFijoDetalle(key, a.Grupo, a.Secuencia, a.Nombre, a.NitResponsable, a.Codigo, a.Fecha, a.Hash, a.ValorCompra)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedActivosFijosDetalle(currentKeys)
	source := "Z27D" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffAuditTrailTerceros() {
	items, year, err := parsers.ParseAuditTrailTerceros(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z11N", "Error parseando audit trail terceros: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, at := range items {
		key := at.NitTercero + "-" + at.Timestamp
		if key == "-" {
			key = at.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertAuditTrailTercero(key, at.FechaCambio, at.NitTercero, at.Timestamp, at.Usuario, at.FechaPeriodo, at.TipoDoc, at.Nombre, at.NitRepresentante, at.NombreRepresentante, at.Direccion, at.Email, at.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedAuditTrailTerceros(currentKeys)
	source := "Z11N" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffClasificacionCuentas() {
	items, year, err := parsers.ParseClasificacionCuentas(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z279CP", "Error parseando clasificacion cuentas: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, c := range items {
		if c.CodigoCuenta == "" {
			continue
		}
		currentKeys[c.CodigoCuenta] = true
		action := s.db.UpsertClasificacionCuenta(c.CodigoCuenta, c.CodigoGrupo, c.CodigoDetalle, c.Descripcion, c.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedClasificacionCuentas(currentKeys)
	source := "Z279CP" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffMovimientosInventario() {
	items, year, err := parsers.ParseMovimientosInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z16", "Error parseando movimientos inventario: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, m := range items {
		if m.RecordKey == "" {
			continue
		}
		currentKeys[m.RecordKey] = true
		action := s.db.UpsertMovimientoInventario(m.RecordKey, m.Empresa, m.Grupo, m.CodigoProducto, m.TipoComprobante, m.CodigoComp, m.Secuencia, m.TipoDoc, m.Fecha, m.Cantidad, m.Valor, m.TipoMov, m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedMovimientosInventario(currentKeys)
	source := "Z16" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffSaldosInventario() {
	items, year, err := parsers.ParseSaldosInventario(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z15", "Error parseando saldos inventario: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, si := range items {
		if si.RecordKey == "" {
			continue
		}
		currentKeys[si.RecordKey] = true
		action := s.db.UpsertSaldoInventario(si.RecordKey, si.Empresa, si.Grupo, si.CodigoProducto, si.Hash, si.SaldoInicial, si.Entradas, si.Salidas, si.SaldoFinal)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedSaldosInventario(currentKeys)
	source := "Z15" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffHistorial() {
	items, year, err := parsers.ParseHistorial(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z18", "Error parseando historial: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, h := range items {
		key := h.Empresa + "-" + h.SubTipo + "-" + h.Fecha + "-" + h.NitOrigen
		if key == "---" {
			continue
		}
		currentKeys[key] = true
		action := s.db.UpsertHistorial(key, h.TipoRegistro, h.SubTipo, h.Empresa, h.Fecha, h.NombreOrigen, h.NombreDestin, h.NitOrigen, h.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedHistorial(currentKeys)
	source := "Z18" + year
	s.db.AddLog("info", source, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
}

func (s *Server) diffMaestros() {
	items, err := parsers.ParseMaestros(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z06", "Error parseando maestros: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(items))

	for _, m := range items {
		key := m.Tipo + "-" + m.Codigo
		if key == "-" {
			continue
		}
		currentKeys[key] = true
		action := s.db.UpsertMaestro(key, m.Tipo, m.Codigo, m.Nombre, m.Responsable, m.Direccion, m.Email, m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedMaestros(currentKeys)
	s.db.AddLog("info", "Z06", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(items)))
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
			s.db.AddLog("info", "API", fmt.Sprintf("[%s] Batch %d completado, esperando %dms...", tableName, i/batchSize, s.cfg.Sync.BatchDelayMs))
			time.Sleep(batchDelay)
		}

		filteredData := s.cfg.ApplyFieldMapping(tableName, rec.Data)
		err := s.client.Sync(tableName, rec.SyncAction, rec.Key, filteredData)
		dataJSON, _ := json.Marshal(rec.Data)
		dataStr := string(dataJSON)

		if err != nil {
			s.db.MarkSyncError(tableName, rec.ID, err.Error())
			s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "error", err.Error())
			lastErr = err.Error()
			errors++
			continue
		}

		s.db.MarkSynced(tableName, rec.ID)
		s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "sent", "")
		sent++
	}

	s.db.RemoveDeletedSynced(tableName)
	s.db.AddLog("info", "API", fmt.Sprintf("[%s] Enviados: %d, Errores: %d (de %d pendientes, batch=%d)", tableName, sent, errors, len(pending), batchSize))
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

	tables := []string{"clients", "products", "movements", "cartera", "movimientos_inventario", "saldos_inventario", "activos_fijos_detalle", "audit_trail_terceros"}
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
			s.db.AddLog("info", "RETRY", fmt.Sprintf("[%s] %d registros reintentados automaticamente (intento %d/%d)", table, n, rc+1, maxRetries))
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

	s.db.AddLog("info", "RETRY", fmt.Sprintf("Backoff exponencial: esperando %ds antes de reintentar %d registros...", int(delay.Seconds()), totalRequeued))
	time.Sleep(delay)

	for _, table := range tables {
		s.sendPending(table)
	}
}

// ==================== ISAM CACHE ====================

var (
	cachedClientes    []parsers.TerceroAmpliado
	cachedProductos   []parsers.Inventario
	cachedMovimientos []parsers.Movimiento
	cachedCartera     []parsers.Cartera
)

func (s *Server) refreshCache(which string) {
	switch which {
	case "clients":
		c, _, err := parsers.ParseTercerosAmpliados(s.cfg.Siigo.DataPath)
		if err == nil {
			cachedClientes = c
		}
	case "products":
		p, _, err := parsers.ParseInventario(s.cfg.Siigo.DataPath)
		if err == nil {
			cachedProductos = p
		}
	case "movements":
		m, err := parsers.ParseMovimientos(s.cfg.Siigo.DataPath)
		if err == nil {
			cachedMovimientos = m
		}
	case "cartera":
		var all []parsers.Cartera
		for _, file := range s.cfg.Sync.Files {
			if len(file) < 3 || file[:3] != "Z09" {
				continue
			}
			anio := ""
			if len(file) > 3 {
				anio = file[3:]
			}
			c, err := parsers.ParseCartera(s.cfg.Siigo.DataPath, anio)
			if err == nil {
				all = append(all, c...)
			}
		}
		cachedCartera = all
	case "movimientos_inventario":
		s.diffMovimientosInventario()
	case "saldos_inventario":
		s.diffSaldosInventario()
	case "activos_fijos_detalle":
		s.diffActivosFijosDetalle()
	case "audit_trail_terceros":
		s.diffAuditTrailTerceros()
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

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			DataPath          string `json:"data_path"`
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
		s.cfg.Finearom.BaseURL = body.BaseURL
		s.cfg.Finearom.Email = body.Email
		s.cfg.Finearom.Password = body.Password
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
		s.db.AddLog("info", "CONFIG", "Configuracion guardada")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	// Return config WITHOUT secrets
	jsonResponse(w, map[string]interface{}{
		"siigo": s.cfg.Siigo,
		"finearom": map[string]interface{}{
			"base_url": s.cfg.Finearom.BaseURL,
			"email":    s.cfg.Finearom.Email,
			"password": maskSecret(s.cfg.Finearom.Password),
		},
		"sync": s.cfg.Sync,
	})
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
		s.db.AddLog("info", "CONFIG", fmt.Sprintf("Edicion/eliminacion de registros: %v", body.Enabled))
		jsonResponse(w, map[string]interface{}{"status": "ok", "enabled": body.Enabled})
		return
	}
	jsonResponse(w, map[string]interface{}{"enabled": s.cfg.AllowEditDelete})
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowEditDelete {
		jsonError(w, "Edicion/eliminacion de registros no esta habilitada", 403)
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
		s.db.AddLog("info", "EDIT", fmt.Sprintf("Registro %d editado en %s", id, table))
		s.db.AddAudit(s.getUsernameFromRequest(r), "edit_record", table, fmt.Sprintf("%d", id), "")
		jsonResponse(w, map[string]string{"status": "ok"})

	case "DELETE":
		if err := s.db.DeleteRecord(table, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "DELETE", fmt.Sprintf("Registro %d eliminado de %s", id, table))
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
		path := s.cfg.Siigo.DataPath + f
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

func (s *Server) handlePlanCuentas(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetPlanCuentas(limit, offset, search)
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

func (s *Server) handleSaldosTerceros(w http.ResponseWriter, r *http.Request) {
	page := getQueryInt(r, "page", 1)
	search := r.URL.Query().Get("search")
	limit := 50
	offset := (page - 1) * limit
	records, total := s.db.GetSaldosTerceros(limit, offset, search)
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
		jsonResponse(w, map[string]string{"message": "Ya hay una sincronizacion en curso"})
		return
	}
	go func() {
		s.doDetectCycle()
		s.doSendCycle()
	}()
	jsonResponse(w, map[string]string{"message": "Sincronizacion manual iniciada (deteccion + envio)"})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	s.paused = true
	s.db.AddLog("info", "SYNC", "Sincronizacion pausada por el usuario")
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	s.paused = false
	s.db.AddLog("info", "SYNC", "Sincronizacion reanudada por el usuario")
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
		s.db.AddLog("info", "SEND", "Envio reactivado desde la web (circuit breaker reseteado)")
		s.bot.Send("▶️ <b>Envio reactivado</b>\n\nReactivado desde la web. Contador de fallos reseteado.")
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
	s.db.AddLog("info", "SYNC", fmt.Sprintf("Reintentando %d registros con error en %s", n, body.Table))
	jsonResponse(w, map[string]interface{}{"status": "ok", "count": n})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	client := api.NewClient(s.cfg.Finearom.BaseURL, s.cfg.Finearom.Email, s.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		jsonResponse(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	s.client = client
	s.db.AddLog("info", "API", "Conexion exitosa con "+s.cfg.Finearom.BaseURL)
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleClearDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
		return
	}
	// Reset setup_complete so the wizard reappears
	s.cfg.SetupComplete = false
	s.setupPopulated = make(map[string]bool)
	if err := s.cfg.Save("config.json"); err != nil {
		s.db.AddLog("warn", "CONFIG", "Error resetting setup_complete: "+err.Error())
	}

	if err := s.db.ClearAll(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("warning", "APP", "Base de datos vaciada por el usuario")
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
	s.db.AddLog("info", "APP", "Logs limpiados por el usuario")
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
		s.db.AddLog("info", "CONFIG", "Mapeo de campos actualizado")
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
		s.db.AddLog("info", "CONFIG", "Envio por modulo actualizado")
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
		s.db.AddLog("info", "CONFIG", "Deteccion por modulo actualizado")
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
			s.db.AddLog("info", "CONFIG", "Telegram bot activado y polling reiniciado")
		} else {
			s.db.AddLog("info", "CONFIG", "Telegram bot desactivado")
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
		jsonError(w, "Telegram no esta configurado o deshabilitado", 400)
		return
	}
	s.bot.Send("✅ <b>Test exitoso</b>\n\nLas notificaciones de Telegram estan funcionando.")
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
			jsonError(w, "Credenciales incorrectas", 401)
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
	"clients":               {EntityType: "Client", KeyProp: "nit"},
	"products":              {EntityType: "Product", KeyProp: "code"},
	"movements":             {EntityType: "Movement", KeyProp: "record_key"},
	"cartera":               {EntityType: "Cartera", KeyProp: "record_key"},
	"plan_cuentas":          {EntityType: "PlanCuenta", KeyProp: "codigo_cuenta"},
	"activos_fijos":         {EntityType: "ActivoFijo", KeyProp: "record_key"},
	"saldos_terceros":       {EntityType: "SaldoTercero", KeyProp: "record_key"},
	"saldos_consolidados":   {EntityType: "SaldoConsolidado", KeyProp: "record_key"},
	"documentos":            {EntityType: "Documento", KeyProp: "record_key"},
	"terceros_ampliados":    {EntityType: "TerceroAmpliado", KeyProp: "record_key"},
	"transacciones_detalle": {EntityType: "TransaccionDetalle", KeyProp: "record_key"},
	"periodos_contables":    {EntityType: "PeriodoContable", KeyProp: "record_key"},
	"condiciones_pago":      {EntityType: "CondicionPago", KeyProp: "record_key"},
	"libros_auxiliares":     {EntityType: "LibroAuxiliar", KeyProp: "record_key"},
	"codigos_dane":          {EntityType: "CodigoDane", KeyProp: "codigo"},
	"actividades_ica":       {EntityType: "ActividadICA", KeyProp: "codigo"},
	"conceptos_pila":        {EntityType: "ConceptoPILA", KeyProp: "record_key"},
	"activos_fijos_detalle": {EntityType: "ActivoFijoDetalle", KeyProp: "record_key"},
	"audit_trail_terceros":  {EntityType: "AuditTrailTercero", KeyProp: "record_key"},
	"clasificacion_cuentas":    {EntityType: "ClasificacionCuenta", KeyProp: "codigo_cuenta"},
	"movimientos_inventario":   {EntityType: "MovimientoInventario", KeyProp: "record_key"},
	"saldos_inventario":        {EntityType: "SaldoInventario", KeyProp: "record_key"},
	"historial":                {EntityType: "Historial", KeyProp: "record_key"},
	"maestros":                 {EntityType: "Maestro", KeyProp: "record_key"},
}

// OData ordered table list (for consistent output, v1 API, and OData routing)
var odataTableOrder = []string{
	"clients", "products", "movements", "cartera",
	"plan_cuentas", "activos_fijos", "saldos_terceros", "saldos_consolidados",
	"documentos", "terceros_ampliados", "transacciones_detalle",
	"periodos_contables", "condiciones_pago", "libros_auxiliares",
	"codigos_dane", "actividades_ica", "conceptos_pila",
	"activos_fijos_detalle", "audit_trail_terceros", "clasificacion_cuentas",
	"movimientos_inventario", "saldos_inventario",
	"historial", "maestros",
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
	// Core tables
	{"movements", "nit_tercero", "clients", "nit", "Client"},
	{"cartera", "nit_tercero", "clients", "nit", "Client"},
	{"cartera", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"saldos_terceros", "nit_tercero", "clients", "nit", "Client"},
	{"saldos_terceros", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"saldos_consolidados", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"documentos", "nit_tercero", "clients", "nit", "Client"},
	{"documentos", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"terceros_ampliados", "nit", "clients", "nit", "Client"},
	{"activos_fijos", "nit_responsable", "clients", "nit", "Responsable"},
	// New tables
	{"transacciones_detalle", "nit_tercero", "clients", "nit", "Client"},
	{"transacciones_detalle", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"condiciones_pago", "nit", "clients", "nit", "Client"},
	{"libros_auxiliares", "nit_tercero", "clients", "nit", "Client"},
	{"libros_auxiliares", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	{"activos_fijos_detalle", "nit_responsable", "clients", "nit", "Responsable"},
	{"audit_trail_terceros", "nit_tercero", "clients", "nit", "Client"},
	{"clasificacion_cuentas", "codigo_cuenta", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
	// Inventory tables
	{"movimientos_inventario", "codigo_producto", "products", "code", "Producto"},
	{"saldos_inventario", "codigo_producto", "products", "code", "Producto"},
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

func (s *Server) sendWebhook(hook config.WebhookDef, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		s.db.AddLog("error", "WEBHOOK", fmt.Sprintf("Error creando request a %s: %v", hook.URL, err))
		return
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
		s.db.AddLog("error", "WEBHOOK", fmt.Sprintf("Error enviando a %s: %v", hook.URL, err))
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.db.AddLog("warning", "WEBHOOK", fmt.Sprintf("Webhook %s respondio %d", hook.URL, resp.StatusCode))
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
	go s.sendWebhook(hook, payload)
	jsonResponse(w, map[string]string{"status": "ok", "message": "Webhook de prueba enviado"})
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

	tunnelURL := readTunnelURL()

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
		s.db.AddLog("info", "USERS", fmt.Sprintf("Usuario '%s' actualizado", user.Username))
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
		s.db.AddLog("info", "USERS", fmt.Sprintf("Usuario '%s' eliminado", user.Username))
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
			sendState = fmt.Sprintf("AUTO-PAUSADO (%d fallos)", s.sendFailCount)
		}
		if s.paused {
			detectState = "PAUSADO"
			sendState = "PAUSADO"
		}
		uptime := time.Since(s.startTime).Round(time.Second)
		stats := s.db.GetStats()

		totalPending := toInt(stats["clients_pending"]) + toInt(stats["products_pending"]) + toInt(stats["movements_pending"]) + toInt(stats["cartera_pending"])
		totalErrors := toInt(stats["clients_errors"]) + toInt(stats["products_errors"]) + toInt(stats["movements_errors"]) + toInt(stats["cartera_errors"])

		msg := fmt.Sprintf("📡 <b>Estado del servidor</b>\n\n📥 Deteccion: %s (cada %ds)\n📤 Envio: %s (cada %ds)\n⏱ Uptime: %s\n⏳ Pendientes: %d\n❌ Errores: %d",
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
		return fmt.Sprintf("📊 <b>Estadisticas</b>\n\n👤 Clientes: %v total, %v sync, %v pend, %v err\n📦 Productos: %v total, %v sync, %v pend, %v err\n📋 Movimientos: %v total, %v sync, %v pend, %v err\n💰 Cartera: %v total, %v sync, %v pend, %v err",
			stats["clients_total"], stats["clients_synced"], stats["clients_pending"], stats["clients_errors"],
			stats["products_total"], stats["products_synced"], stats["products_pending"], stats["products_errors"],
			stats["movements_total"], stats["movements_synced"], stats["movements_pending"], stats["movements_errors"],
			stats["cartera_total"], stats["cartera_synced"], stats["cartera_pending"], stats["cartera_errors"],
		)
	})

	s.bot.RegisterCommand("/errors", func(args string) string {
		errors := s.db.GetErrorSummary()
		if len(errors) == 0 {
			return "✅ <b>Sin errores</b>\n\nNo hay registros con error."
		}
		msg := "❌ <b>Resumen de errores</b>\n"
		for _, e := range errors {
			msg += fmt.Sprintf("\n📋 <b>%s</b> (%d)\n<code>%s</code>", e.Table, e.Count, truncateStr(e.Error, 100))
		}
		return msg
	})

	s.bot.RegisterCommand("/sync", func(args string) string {
		if s.detecting || s.sending {
			return "⏳ Ya hay una sincronizacion en curso."
		}
		go func() {
			s.doDetectCycle()
			s.doSendCycle()
		}()
		return "🔄 Sincronizacion manual iniciada (deteccion + envio)."
	})

	s.bot.RegisterCommand("/pause", func(args string) string {
		s.paused = true
		s.db.AddLog("info", "SYNC", "Sync pausado via Telegram")
		return "⏸ Sincronizacion pausada."
	})

	s.bot.RegisterCommand("/resume", func(args string) string {
		s.paused = false
		s.sendPaused = false
		s.sendFailCount = 0
		s.db.AddLog("info", "SYNC", "Sync reanudado via Telegram")
		return "▶️ Sincronizacion reanudada (deteccion + envio)."
	})

	s.bot.RegisterCommand("/send-resume", func(args string) string {
		if !s.sendPaused {
			return "✅ El envio ya esta activo, no esta pausado."
		}
		s.sendPaused = false
		s.sendFailCount = 0
		s.db.AddLog("info", "SEND", "Envio reactivado via Telegram (circuit breaker reseteado)")
		return "▶️ Envio reactivado. Contador de fallos reseteado a 0."
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
		s.db.AddLog("info", "SYNC", fmt.Sprintf("Reintentando %d errores via Telegram", total))
		return fmt.Sprintf("🔄 %d registros movidos a pendiente para reintento.", total)
	})

	s.bot.RegisterCommand("/url", func(args string) string {
		msg := "🌐 <b>URLs</b>\n\n🖥 Local: http://localhost:3210"
		if tunnelURL := readTunnelURL(); tunnelURL != "" {
			msg += fmt.Sprintf("\n🌍 Publica: %s", tunnelURL)
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
			status = "🟡 PAUSADO"
		}

		return fmt.Sprintf("%s\n\n⏱ Uptime: %s\n📊 Registros: %d\n📥 Detectando: %v\n📤 Enviando: %v\n⏸ Pausado: %v", status, uptime, totalRecords, s.detecting, s.sending, s.paused)
	})

	s.bot.RegisterCommand("/exec", func(args string) string {
		pin := s.cfg.Telegram.ExecPin
		if pin == "" {
			return "⛔ PIN no configurado. Configura exec_pin en Telegram Config."
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

		return "🤖 <b>Claude iniciado</b>\n\nProceso lanzado en background.\nRevisa la terminal del servidor para la URL de conexion."
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
		"clients": "Clientes / Terceros (Z17)", "products": "Productos / Inventario (Z04)",
		"movements": "Movimientos contables (Z49)", "cartera": "Cartera / CxC (Z09)",
		"plan_cuentas": "Plan de Cuentas PUC (Z03)", "activos_fijos": "Activos Fijos (Z27)",
		"saldos_terceros": "Saldos por Tercero BCD (Z25)", "saldos_consolidados": "Saldos Consolidados BCD (Z28)",
		"documentos": "Documentos / Facturas (Z11)", "terceros_ampliados": "Terceros Ampliados (Z08A)",
		"transacciones_detalle": "Transacciones Detalle (Z07T)", "periodos_contables": "Periodos Contables (Z26)",
		"condiciones_pago": "Condiciones de Pago (Z05)", "libros_auxiliares": "Libros Auxiliares BCD (Z07)",
		"codigos_dane": "Codigos DANE Municipios", "actividades_ica": "Actividades ICA",
		"conceptos_pila": "Conceptos PILA", "activos_fijos_detalle": "Activos Fijos Detalle (Z27A)",
		"audit_trail_terceros": "Audit Trail Terceros (Z11N)", "clasificacion_cuentas": "Clasificacion Cuentas (Z279CP)",
		"movimientos_inventario": "Movimientos Inventario (Z16)", "saldos_inventario": "Saldos Inventario (Z23)",
		"historial": "Historial Documentos (Z18)", "maestros": "Maestros Config (Z06)",
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
	dataFolder := pmItem{Name: "API v1 - Data Tables (24)", Item: dataItems}

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
			Body: &pmBody{Mode: "raw", Raw: `{"clients": true, "historial": false}`, Opts: jsonOpts}}},
	}}

	// --- Config folder ---
	configFolder := pmItem{Name: "Configuration", Item: []pmItem{
		{Name: "Get Config", Request: &pmReq{Method: "GET", Header: authHeader, URL: makeURL("/api/config")}},
		{Name: "Save Config", Request: &pmReq{Method: "POST", Header: authHeader, URL: makeURL("/api/config"),
			Body: &pmBody{Mode: "raw", Raw: `{"data_path": "C:\\\\DEMOS01\\\\", "interval": 60}`, Opts: jsonOpts}}},
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
	col.Info.Desc = fmt.Sprintf("Coleccion generada automaticamente desde Siigo Sync Middleware.\n\nBase URL: %s\n\n24 tablas de datos de Siigo Pyme con sync bidireccional, OData v4 para Power BI, y API REST completa.", baseURL)
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
