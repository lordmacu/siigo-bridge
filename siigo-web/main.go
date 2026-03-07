package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"siigo-common/isam"
	"siigo-common/parsers"
	"siigo-common/api"
	"siigo-common/config"
	"siigo-common/storage"
	"strings"
	"time"
)

type Server struct {
	db      *storage.DB
	cfg     *config.Config
	client  *api.Client
	syncing bool
	paused  bool
	stopCh  chan bool
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

	srv := &Server{
		db:     db,
		cfg:    cfg,
		stopCh: make(chan bool, 1),
	}

	db.AddLog("info", "APP", "Servidor web iniciado")

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
			// Try to serve the file; if not found, serve index.html (SPA)
			path := frontendDir + r.URL.Path
			if _, err := os.Stat(path); os.IsNotExist(err) && r.URL.Path != "/" {
				http.ServeFile(w, r, frontendDir+"/index.html")
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
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/isam-info", s.handleISAMInfo)
	mux.HandleFunc("/api/extfh-status", s.handleExtfhStatus)
	mux.HandleFunc("/api/clients", s.handleClients)
	mux.HandleFunc("/api/products", s.handleProducts)
	mux.HandleFunc("/api/movements", s.handleMovements)
	mux.HandleFunc("/api/cartera", s.handleCartera)
	mux.HandleFunc("/api/sync-history", s.handleSyncHistory)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/sync-now", s.handleSyncNow)
	mux.HandleFunc("/api/pause", s.handlePause)
	mux.HandleFunc("/api/resume", s.handleResume)
	mux.HandleFunc("/api/sync-status", s.handleSyncStatus)
	mux.HandleFunc("/api/retry-errors", s.handleRetryErrors)
	mux.HandleFunc("/api/test-connection", s.handleTestConnection)
	mux.HandleFunc("/api/clear-database", s.handleClearDatabase)
	mux.HandleFunc("/api/clear-logs", s.handleClearLogs)
	mux.HandleFunc("/api/refresh-cache", s.handleRefreshCache)
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
	s.db.AddLog("info", "SYNC", "Sync loop iniciado")

	interval := time.Duration(s.cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	s.doOneSyncCycle()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !s.paused && !s.syncing {
				s.doOneSyncCycle()
			}
		case <-s.stopCh:
			s.db.AddLog("info", "SYNC", "Sync loop detenido")
			return
		}
	}
}

func (s *Server) doOneSyncCycle() {
	s.syncing = true
	defer func() { s.syncing = false }()

	s.db.AddLog("info", "SYNC", "--- Ciclo iniciado ---")

	s.diffClientes()
	s.diffProductos()
	s.diffMovimientos()
	s.diffCartera()

	if s.ensureLogin() {
		s.sendPending("clients")
		s.sendPending("products")
		s.sendPending("movements")
		s.sendPending("cartera")
	}

	s.db.AddLog("info", "SYNC", "--- Ciclo completado ---")
}

func (s *Server) ensureLogin() bool {
	if s.client != nil && s.client.IsAuthenticated() {
		return true
	}
	client := api.NewClient(s.cfg.Finearom.BaseURL, s.cfg.Finearom.Email, s.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		s.db.AddLog("error", "API", "Login failed: "+err.Error())
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
	}
}

// ==================== SEND PENDING ====================

func (s *Server) sendPending(tableName string) {
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
		return
	}

	sent, errors := 0, 0
	for _, rec := range pending {
		err := s.client.Sync(tableName, rec.SyncAction, rec.Key, rec.Data)
		dataJSON, _ := json.Marshal(rec.Data)
		dataStr := string(dataJSON)

		if err != nil {
			s.db.MarkSyncError(tableName, rec.ID, err.Error())
			s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "error", err.Error())
			errors++
			continue
		}

		s.db.MarkSynced(tableName, rec.ID)
		s.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "sent", "")
		sent++
	}

	s.db.RemoveDeletedSynced(tableName)
	s.db.AddLog("info", "API", fmt.Sprintf("[%s] Enviados: %d, Errores: %d (de %d pendientes)", tableName, sent, errors, len(pending)))
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
			DataPath string `json:"data_path"`
			BaseURL  string `json:"base_url"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Interval int    `json:"interval"`
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
		if err := s.cfg.Save("config.json"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		s.db.AddLog("info", "CONFIG", "Configuracion guardada")
		jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	jsonResponse(w, s.cfg)
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
	if s.syncing {
		jsonResponse(w, map[string]string{"message": "Ya hay una sincronizacion en curso"})
		return
	}
	go s.doOneSyncCycle()
	jsonResponse(w, map[string]string{"message": "Sincronizacion manual iniciada"})
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
		"syncing": s.syncing,
		"paused":  s.paused,
	})
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
