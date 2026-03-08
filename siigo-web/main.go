package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
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

type Server struct {
	db             *storage.DB
	cfg            *config.Config
	client         *api.Client
	bot            *telegram.Bot
	detecting      bool
	sending        bool
	paused         bool
	sendPaused     bool // auto-paused by circuit breaker
	sendFailCount  int  // consecutive send cycle failures
	stopCh         chan bool
	tokens         map[string]time.Time
	tokenMu        gosync.RWMutex
	startTime      time.Time
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
		db:        db,
		cfg:       cfg,
		bot:       bot,
		stopCh:    make(chan bool, 1),
		tokens:    make(map[string]time.Time),
		startTime: time.Now(),
	}

	db.AddLog("info", "APP", "Servidor web iniciado")

	srv.registerBotCommands()
	bot.StartPolling()

	go srv.startSyncLoop()

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
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
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
	exp, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "POST only", 405)
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
	if body.Username != s.cfg.Auth.Username || body.Password != s.cfg.Auth.Password {
		jsonError(w, "Credenciales incorrectas", 401)
		return
	}
	token := s.generateToken()
	s.tokenMu.Lock()
	s.tokens[token] = time.Now().Add(24 * time.Hour)
	s.tokenMu.Unlock()

	jsonResponse(w, map[string]string{"token": token})
}

func (s *Server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if s.isValidToken(token) {
		jsonResponse(w, map[string]string{"status": "ok"})
	} else {
		jsonError(w, "No autorizado", 401)
	}
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

	// Protected routes
	mux.HandleFunc("/api/stats", s.authMiddleware(s.handleStats))
	mux.HandleFunc("/api/config", s.authMiddleware(s.handleConfig))
	mux.HandleFunc("/api/isam-info", s.authMiddleware(s.handleISAMInfo))
	mux.HandleFunc("/api/extfh-status", s.authMiddleware(s.handleExtfhStatus))
	mux.HandleFunc("/api/clients", s.authMiddleware(s.handleClients))
	mux.HandleFunc("/api/products", s.authMiddleware(s.handleProducts))
	mux.HandleFunc("/api/movements", s.authMiddleware(s.handleMovements))
	mux.HandleFunc("/api/cartera", s.authMiddleware(s.handleCartera))
	mux.HandleFunc("/api/sync-history", s.authMiddleware(s.handleSyncHistory))
	mux.HandleFunc("/api/logs", s.authMiddleware(s.handleLogs))
	mux.HandleFunc("/api/sync-now", s.authMiddleware(s.handleSyncNow))
	mux.HandleFunc("/api/pause", s.authMiddleware(s.handlePause))
	mux.HandleFunc("/api/resume", s.authMiddleware(s.handleResume))
	mux.HandleFunc("/api/sync-status", s.authMiddleware(s.handleSyncStatus))
	mux.HandleFunc("/api/send-resume", s.authMiddleware(s.handleSendResume))
	mux.HandleFunc("/api/retry-errors", s.authMiddleware(s.handleRetryErrors))
	mux.HandleFunc("/api/test-connection", s.authMiddleware(s.handleTestConnection))
	mux.HandleFunc("/api/clear-database", s.authMiddleware(s.handleClearDatabase))
	mux.HandleFunc("/api/clear-logs", s.authMiddleware(s.handleClearLogs))
	mux.HandleFunc("/api/refresh-cache", s.authMiddleware(s.handleRefreshCache))
	mux.HandleFunc("/api/field-mappings", s.authMiddleware(s.handleFieldMappings))
	mux.HandleFunc("/api/send-enabled", s.authMiddleware(s.handleSendEnabled))
	mux.HandleFunc("/api/error-summary", s.authMiddleware(s.handleErrorSummary))
	mux.HandleFunc("/api/export-history", s.authMiddleware(s.handleExportHistory))
	mux.HandleFunc("/api/export-logs", s.authMiddleware(s.handleExportLogs))
	mux.HandleFunc("/api/public-api-config", s.authMiddleware(s.handlePublicAPIConfig))
	mux.HandleFunc("/api/telegram-config", s.authMiddleware(s.handleTelegramConfig))
	mux.HandleFunc("/api/telegram-test", s.authMiddleware(s.handleTelegramTest))

	// Swagger docs
	mux.HandleFunc("/api/v1/docs", handleSwaggerUI)
	mux.HandleFunc("/api/v1/swagger.json", handleSwaggerJSON)

	// Public API v1 (JWT auth)
	mux.HandleFunc("/api/v1/auth", s.handleV1Auth)
	mux.HandleFunc("/api/v1/stats", s.jwtMiddleware(s.handleV1Stats))
	mux.HandleFunc("/api/v1/clients", s.jwtMiddleware(s.handleV1List("clients")))
	mux.HandleFunc("/api/v1/clients/", s.jwtMiddleware(s.handleV1Detail("clients")))
	mux.HandleFunc("/api/v1/products", s.jwtMiddleware(s.handleV1List("products")))
	mux.HandleFunc("/api/v1/products/", s.jwtMiddleware(s.handleV1Detail("products")))
	mux.HandleFunc("/api/v1/movements", s.jwtMiddleware(s.handleV1List("movements")))
	mux.HandleFunc("/api/v1/movements/", s.jwtMiddleware(s.handleV1Detail("movements")))
	mux.HandleFunc("/api/v1/cartera", s.jwtMiddleware(s.handleV1List("cartera")))
	mux.HandleFunc("/api/v1/cartera/", s.jwtMiddleware(s.handleV1Detail("cartera")))
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

	s.diffClientes()
	s.diffProductos()
	s.diffMovimientos()
	s.diffCartera()

	s.db.AddLog("info", "DETECT", "--- Ciclo deteccion completado ---")
}

const maxSendFailures = 4

func (s *Server) doSendCycle() {
	s.sending = true
	defer func() { s.sending = false }()

	if !s.ensureLogin() {
		s.sendFailCount++
		s.checkSendCircuitBreaker("Login fallido")
		return
	}

	s.db.AddLog("info", "SEND", "--- Ciclo envio iniciado ---")

	totalSent, totalErrors := 0, 0
	tables := []string{"clients", "products", "movements", "cartera"}
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
}

func (s *Server) checkSendCircuitBreaker(reason string) {
	if s.sendFailCount >= maxSendFailures && !s.sendPaused {
		s.sendPaused = true
		msg := fmt.Sprintf("🔴 <b>Envio auto-pausado</b>\n\n%d fallos consecutivos.\nUltimo: %s\n\nRevisa el servidor de destino y reactiva el envio con /send-resume o desde la web.",
			s.sendFailCount, reason)
		s.bot.Send(msg)
		s.db.AddLog("error", "SEND", fmt.Sprintf("Circuit breaker activado: %d fallos consecutivos (%s). Envio pausado automaticamente.", s.sendFailCount, reason))
	} else if s.sendFailCount > 0 && !s.sendPaused {
		s.db.AddLog("warn", "SEND", fmt.Sprintf("Fallo de envio %d/%d (%s)", s.sendFailCount, maxSendFailures, reason))
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

// ==================== DIFF ====================

func (s *Server) diffClientes() {
	clientes, err := parsers.ParseTercerosClientes(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z17", "Error parseando: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(clientes))

	for _, t := range clientes {
		nit := strings.TrimLeft(t.NumeroDoc, "0")
		if nit == "" {
			continue
		}
		currentKeys[nit] = true
		action := s.db.UpsertClient(nit, t.Nombre, t.TipoDoc, t.TipoClave, t.Empresa, t.Codigo, t.FechaCreacion, t.TipoCtaPref, t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedClients(currentKeys)
	s.db.AddLog("info", "Z17", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(clientes)))
	s.bot.NotifyChangesDetected("Clientes", adds, edits, deletes)
}

func (s *Server) diffProductos() {
	productos, err := parsers.ParseProductos(s.cfg.Siigo.DataPath)
	if err != nil {
		s.db.AddLog("error", "Z06CP", "Error parseando: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(productos))

	for _, p := range productos {
		key := p.Comprobante + "-" + p.Secuencia
		if key == "-" {
			key = p.Hash
		}
		currentKeys[key] = true
		action := s.db.UpsertProduct(key, p.Nombre, p.Comprobante, p.Secuencia, p.TipoTercero, p.Grupo, p.CuentaContable, p.Fecha, p.TipoMov, p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := s.db.MarkDeletedProducts(currentKeys)
	s.db.AddLog("info", "Z06CP", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(productos)))
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

		fecha := m.Fecha
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		action := s.db.UpsertMovement(key, m.TipoComprobante, m.Empresa, m.NumeroDoc, fecha, m.NitTercero, m.CuentaContable, m.Descripcion, m.Valor, m.TipoMov, m.Hash)
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
	for _, file := range s.cfg.Sync.Files {
		if len(file) < 3 || file[:3] != "Z09" {
			continue
		}
		anio := ""
		if len(file) > 3 {
			anio = file[3:]
		}

		cartera, err := parsers.ParseCartera(s.cfg.Siigo.DataPath, anio)
		if err != nil {
			s.db.AddLog("error", file, "Error parseando: "+err.Error())
			continue
		}

		adds, edits := 0, 0
		currentKeys := make(map[string]bool, len(cartera))

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

	tables := []string{"clients", "products", "movements", "cartera"}
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
	cachedClientes    []parsers.Tercero
	cachedProductos   []parsers.Producto
	cachedMovimientos []parsers.Movimiento
	cachedCartera     []parsers.Cartera
)

func (s *Server) refreshCache(which string) {
	switch which {
	case "clients":
		c, err := parsers.ParseTercerosClientes(s.cfg.Siigo.DataPath)
		if err == nil {
			cachedClientes = c
		}
	case "products":
		p, err := parsers.ParseProductos(s.cfg.Siigo.DataPath)
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
	}
}

// ==================== HTTP HANDLERS ====================

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
	files := []string{"Z17", "Z06", "Z06CP", "Z49"}
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
	logs, total, err := s.db.GetLogs(limit, offset)
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
	if err := s.db.ClearAll(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.db.AddLog("warning", "APP", "Base de datos vaciada por el usuario")
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
			Enabled  *bool  `json:"enabled"`
			BotToken string `json:"bot_token"`
			ChatID   int64  `json:"chat_id"`
			ExecPin  string `json:"exec_pin"`
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
		"enabled":       s.cfg.Telegram.Enabled,
		"bot_token":     maskSecret(s.cfg.Telegram.BotToken),
		"chat_id":       s.cfg.Telegram.ChatID,
		"has_exec_pin":  s.cfg.Telegram.ExecPin != "",
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

// ==================== PUBLIC API v1 (JWT) ====================

func (s *Server) jwtMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.PublicAPI.Enabled {
			jsonError(w, "API deshabilitada", 403)
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
		ApiKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "JSON invalido", 400)
		return
	}

	if body.ApiKey != s.cfg.PublicAPI.ApiKey {
		jsonError(w, "API key invalida", 401)
		return
	}

	// Generate JWT valid for 24 hours
	claims := jwt.MapClaims{
		"iss": "siigo-sync",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
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
		cmd.Dir = "/c/Users/lordmacu/siigo"
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
		cmd.Dir = "/c/Users/lordmacu/siigo"
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
