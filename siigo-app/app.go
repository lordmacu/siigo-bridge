package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"siigo-common/api"
	"siigo-common/config"
	"siigo-common/storage"
	"siigo-common/isam"
	"siigo-common/parsers"
	"strings"
	"time"
)

type App struct {
	ctx     context.Context
	db      *storage.DB
	cfg     *config.Config
	client  *api.Client
	syncing bool
	looping bool
	paused  bool
	stopCh  chan bool
}

func NewApp() *App {
	return &App{
		stopCh: make(chan bool, 1),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	db, err := storage.NewDB("siigo_app.db")
	if err != nil {
		log.Printf("DB error: %v", err)
		return
	}
	a.db = db
	a.db.AddLog("info", "APP", "Application started")

	cfg, err := config.Load("config.json")
	if err != nil {
		cfg = config.Default()
		cfg.Save("config.json")
		a.db.AddLog("warning", "APP", "Config not found, default config created")
	}
	a.cfg = cfg

	go a.startSyncLoop()
}

func (a *App) shutdown(ctx context.Context) {
	if a.looping {
		a.stopCh <- true
	}
	if a.db != nil {
		a.db.Close()
	}
}

// ==================== SYNC LOOP ====================

func (a *App) startSyncLoop() {
	a.looping = true
	a.paused = false
	a.db.AddLog("info", "SYNC", "Sync loop started")

	interval := time.Duration(a.cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	a.doOneSyncCycle()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !a.paused && !a.syncing {
				a.doOneSyncCycle()
			}
		case <-a.stopCh:
			a.looping = false
			a.db.AddLog("info", "SYNC", "Sync loop stopped")
			return
		}
	}
}

func (a *App) doOneSyncCycle() {
	a.syncing = true
	defer func() { a.syncing = false }()

	a.db.AddLog("info", "SYNC", "--- Cycle started ---")

	// Step 1: Parse ISAM files and diff against SQLite
	a.diffClientes()
	a.diffProductos()
	a.diffMovimientos()
	a.diffCartera()

	// Step 2: Send pending changes to server
	if a.ensureLogin() {
		a.sendPending("clients")
		a.sendPending("products")
		a.sendPending("movements")
		a.sendPending("cartera")
	}

	a.db.AddLog("info", "SYNC", "--- Cycle completed ---")
}

func (a *App) ensureLogin() bool {
	if a.client != nil && a.client.IsAuthenticated() {
		return true
	}
	client := api.NewClient(a.cfg.Finearom.BaseURL, a.cfg.Finearom.Email, a.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		a.db.AddLog("error", "API", "Login failed: "+err.Error())
		return false
	}
	a.client = client
	return true
}

// PauseSync pauses the auto-sync loop
func (a *App) PauseSync() string {
	if a.paused {
		return "already_paused"
	}
	a.paused = true
	a.db.AddLog("info", "SYNC", "Sync paused by user")
	return "ok"
}

// ResumeSync resumes the auto-sync loop
func (a *App) ResumeSync() string {
	if !a.paused {
		return "already_running"
	}
	a.paused = false
	a.db.AddLog("info", "SYNC", "Sync resumed by user")
	return "ok"
}

func (a *App) IsPaused() bool  { return a.paused }
func (a *App) IsSyncing() bool { return a.syncing }

// SyncNow triggers an immediate sync cycle
func (a *App) SyncNow() string {
	if a.syncing {
		return "Sync already in progress"
	}
	go a.doOneSyncCycle()
	return "Manual sync started"
}

// RetryErrors resets all error records to pending for a given table
func (a *App) RetryErrors(tableName string) string {
	n := a.db.RetryErrors(tableName)
	a.db.AddLog("info", "SYNC", fmt.Sprintf("Retrying %d error records in %s", n, tableName))
	return fmt.Sprintf("ok: %d records marked for retry", n)
}

// ==================== DIFF: Parse ISAM → Compare with SQLite ====================

func (a *App) diffClientes() {
	clientes, err := parsers.ParseTercerosClientes(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z17", "Error parsing: "+err.Error())
		return
	}

	adds, edits := 0, 0
	currentKeys := make(map[string]bool, len(clientes))

	for _, t := range clientes {
		nit := strings.TrimLeft(t.DocNumber, "0")
		if nit == "" {
			continue
		}
		currentKeys[nit] = true

		action := a.db.UpsertClient(nit, t.Name, t.DocType, t.KeyType, t.Company, t.Code, t.CreationDate, t.PreferredAcctType, t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeletedClients(currentKeys)
	a.db.AddLog("info", "Z17", fmt.Sprintf("Diff: %d added, %d edited, %d deleted (of %d)", adds, edits, deletes, len(clientes)))
}

func (a *App) diffProductos() {
	productos, year, err := parsers.ParseInventario(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z04", "Error parsing inventory: "+err.Error())
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

		action := a.db.UpsertProduct(key, p.Name, p.ShortName, p.Group, p.Reference, p.Company, p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeletedProducts(currentKeys)
	source := "Z04" + year
	a.db.AddLog("info", source, fmt.Sprintf("Diff: %d added, %d edited, %d deleted (of %d)", adds, edits, deletes, len(productos)))
}

func (a *App) diffMovimientos() {
	movimientos, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z49", "Error parsing: "+err.Error())
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

		fecha := ""
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}

		action := a.db.UpsertMovement(key, m.VoucherType, m.Company, m.DocNumber, fecha, "", "", m.Description, 0, "", m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeletedMovements(currentKeys)
	a.db.AddLog("info", "Z49", fmt.Sprintf("Diff: %d added, %d edited, %d deleted (of %d)", adds, edits, deletes, len(movimientos)))
}

func (a *App) diffCartera() {
	for _, file := range a.cfg.Sync.Files {
		if len(file) < 3 || file[:3] != "Z09" {
			continue
		}
		anio := ""
		if len(file) > 3 {
			anio = file[3:]
		}

		cartera, err := parsers.ParseCartera(a.cfg.Siigo.DataPath, anio)
		if err != nil {
			a.db.AddLog("error", file, "Error parsing: "+err.Error())
			continue
		}

		adds, edits := 0, 0
		currentKeys := make(map[string]bool, len(cartera))

		for _, c := range cartera {
			key := file + "-" + c.RecordType + "-" + c.Company + "-" + c.Sequence
			currentKeys[key] = true

			fecha := c.Date
			if len(fecha) == 8 {
				fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
			}

			action := a.db.UpsertCartera(key, c.RecordType, c.Company, c.Sequence, c.DocType, c.ThirdPartyNit, c.LedgerAccount, fecha, c.Description, c.MovType, c.Hash)
			switch action {
			case "add":
				adds++
			case "edit":
				edits++
			}
		}

		deletes := a.db.MarkDeletedCartera(currentKeys)
		a.db.AddLog("info", file, fmt.Sprintf("Diff: %d added, %d edited, %d deleted (of %d)", adds, edits, deletes, len(cartera)))
	}
}

// ==================== SEND PENDING: SQLite → Server ====================

func (a *App) sendPending(tableName string) {
	var pending []storage.PendingRecord
	switch tableName {
	case "clients":
		pending = a.db.GetPendingClients()
	case "products":
		pending = a.db.GetPendingProducts()
	case "movements":
		pending = a.db.GetPendingMovements()
	case "cartera":
		pending = a.db.GetPendingCartera()
	}

	if len(pending) == 0 {
		return
	}

	sent, errors := 0, 0
	for _, rec := range pending {
		err := a.client.Sync(tableName, rec.SyncAction, rec.Key, rec.Data)

		dataJSON, _ := json.Marshal(rec.Data)
		dataStr := string(dataJSON)

		if err != nil {
			a.db.MarkSyncError(tableName, rec.ID, err.Error())
			a.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "error", err.Error())
			errors++
			continue
		}

		a.db.MarkSynced(tableName, rec.ID)
		a.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, dataStr, "sent", "")
		sent++
	}

	a.db.RemoveDeletedSynced(tableName)

	a.db.AddLog("info", "API", fmt.Sprintf("[%s] Sent: %d, Errors: %d (of %d pending)", tableName, sent, errors, len(pending)))
}

// ==================== CONFIG ====================

func (a *App) GetConfig() *config.Config {
	return a.cfg
}

func (a *App) SaveConfig(dataPath, baseURL, email, password string, interval int) string {
	a.cfg.Siigo.DataPath = dataPath
	a.cfg.Finearom.BaseURL = baseURL
	a.cfg.Finearom.Email = email
	a.cfg.Finearom.Password = password
	a.cfg.Sync.IntervalSeconds = interval

	if err := a.cfg.Save("config.json"); err != nil {
		return "Error: " + err.Error()
	}
	a.db.AddLog("info", "CONFIG", "Configuration saved")
	return "ok"
}

// ==================== ISAM DATA ====================

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

func (a *App) GetISAMInfo() []ISAMPreview {
	files := []string{"Z17", "Z06", "Z49"}
	var result []ISAMPreview
	for _, f := range files {
		path := a.cfg.Siigo.DataPath + f
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
	return result
}

// ISAM cache - avoid re-reading files on every page request
var (
	cachedClientes    []parsers.Tercero
	cachedProductos   []parsers.Inventario
	cachedMovimientos []parsers.Movimiento
	cachedCartera     []parsers.Cartera
	clientesByNIT     map[string]*parsers.Tercero
	productosByCodigo map[string]*parsers.Inventario
	movimientosByNIT  map[string][]parsers.Movimiento
	carteraByNIT      map[string][]parsers.Cartera
)

func (a *App) RefreshCache(which string) {
	switch which {
	case "clients":
		c, err := parsers.ParseTercerosClientes(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z17", "Error reading clients: "+err.Error())
			return
		}
		cachedClientes = c
		clientesByNIT = make(map[string]*parsers.Tercero, len(c))
		for i := range c {
			nit := strings.TrimLeft(c[i].DocNumber, "0")
			if nit != "" {
				clientesByNIT[nit] = &cachedClientes[i]
			}
		}
		a.db.AddLog("info", "Z17", fmt.Sprintf("Cache: %d clients", len(c)))
	case "products":
		p, year, err := parsers.ParseInventario(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z04", "Error reading inventory: "+err.Error())
			return
		}
		cachedProductos = p
		productosByCodigo = make(map[string]*parsers.Inventario, len(p))
		for i := range p {
			if p[i].Code != "" {
				productosByCodigo[p[i].Code] = &cachedProductos[i]
			}
		}
		a.db.AddLog("info", "Z04"+year, fmt.Sprintf("Cache: %d products", len(p)))
	case "movements":
		m, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z49", "Error reading movements: "+err.Error())
			return
		}
		cachedMovimientos = m
		movimientosByNIT = make(map[string][]parsers.Movimiento)
		for _, mov := range m {
			nit := strings.TrimLeft(mov.ThirdPartyName, "0")
			if nit != "" {
				movimientosByNIT[nit] = append(movimientosByNIT[nit], mov)
			}
		}
		a.db.AddLog("info", "Z49", fmt.Sprintf("Cache: %d movements", len(m)))
	case "cartera":
		var all []parsers.Cartera
		for _, file := range a.cfg.Sync.Files {
			if len(file) < 3 || file[:3] != "Z09" {
				continue
			}
			anio := ""
			if len(file) > 3 {
				anio = file[3:]
			}
			c, err := parsers.ParseCartera(a.cfg.Siigo.DataPath, anio)
			if err != nil {
				a.db.AddLog("error", file, "Error reading cartera: "+err.Error())
				continue
			}
			all = append(all, c...)
		}
		cachedCartera = all
		carteraByNIT = make(map[string][]parsers.Cartera)
		for _, c := range all {
			nit := strings.TrimLeft(c.ThirdPartyNit, "0")
			if nit != "" {
				carteraByNIT[nit] = append(carteraByNIT[nit], c)
			}
		}
		a.db.AddLog("info", "Z09", fmt.Sprintf("Cache: %d cartera records", len(all)))
	}
}

func (a *App) GetClientes(page int, search string) PaginatedISAM {
	if cachedClientes == nil {
		a.RefreshCache("clients")
	}
	data := cachedClientes
	if search != "" {
		var filtered []parsers.Tercero
		q := strings.ToLower(search)
		for _, c := range data {
			if strings.Contains(strings.ToLower(c.DocNumber), q) ||
				strings.Contains(strings.ToLower(c.Name), q) ||
				strings.Contains(strings.ToLower(c.Company), q) ||
				strings.Contains(strings.ToLower(c.Code), q) {
				filtered = append(filtered, c)
			}
		}
		data = filtered
	}
	total := len(data)
	start, end := paginate(total, page, 50)
	return PaginatedISAM{Data: data[start:end], Total: total}
}

func (a *App) GetProductos(page int, search string) PaginatedISAM {
	if cachedProductos == nil {
		a.RefreshCache("products")
	}
	data := cachedProductos
	if search != "" {
		var filtered []parsers.Inventario
		q := strings.ToLower(search)
		for _, p := range data {
			if strings.Contains(strings.ToLower(p.Name), q) ||
				strings.Contains(strings.ToLower(p.Code), q) ||
				strings.Contains(strings.ToLower(p.Group), q) {
				filtered = append(filtered, p)
			}
		}
		data = filtered
	}
	total := len(data)
	start, end := paginate(total, page, 50)
	return PaginatedISAM{Data: data[start:end], Total: total}
}

func (a *App) GetMovimientos(page int, search string) PaginatedISAM {
	if cachedMovimientos == nil {
		a.RefreshCache("movements")
	}
	data := cachedMovimientos
	if search != "" {
		var filtered []parsers.Movimiento
		q := strings.ToLower(search)
		for _, m := range data {
			if strings.Contains(strings.ToLower(m.VoucherType), q) ||
				strings.Contains(strings.ToLower(m.DocNumber), q) ||
				strings.Contains(strings.ToLower(m.ThirdPartyName), q) ||
				strings.Contains(strings.ToLower(m.Description), q) {
				filtered = append(filtered, m)
			}
		}
		data = filtered
	}
	total := len(data)
	start, end := paginate(total, page, 50)
	return PaginatedISAM{Data: data[start:end], Total: total}
}

func (a *App) GetCartera(page int, search string) PaginatedISAM {
	if cachedCartera == nil {
		a.RefreshCache("cartera")
	}
	data := cachedCartera
	if search != "" {
		var filtered []parsers.Cartera
		q := strings.ToLower(search)
		for _, c := range data {
			if strings.Contains(strings.ToLower(c.ThirdPartyNit), q) ||
				strings.Contains(strings.ToLower(c.Description), q) ||
				strings.Contains(strings.ToLower(c.LedgerAccount), q) ||
				strings.Contains(strings.ToLower(c.Date), q) ||
				strings.Contains(strings.ToLower(c.RecordType), q) {
				filtered = append(filtered, c)
			}
		}
		data = filtered
	}
	total := len(data)
	start, end := paginate(total, page, 50)
	return PaginatedISAM{Data: data[start:end], Total: total}
}

func paginate(total, page, perPage int) (int, int) {
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return start, end
}

// ==================== LOOKUPS ====================

func (a *App) LookupByNIT(nit string) *parsers.Tercero {
	if clientesByNIT == nil {
		a.RefreshCache("clients")
	}
	nit = strings.TrimLeft(strings.TrimSpace(nit), "0")
	return clientesByNIT[nit]
}

func (a *App) LookupProducto(codigo string) *parsers.Inventario {
	if productosByCodigo == nil {
		a.RefreshCache("products")
	}
	return productosByCodigo[strings.TrimSpace(codigo)]
}

func (a *App) LookupMovimientosPorNIT(nit string) []parsers.Movimiento {
	if movimientosByNIT == nil {
		a.RefreshCache("movements")
	}
	nit = strings.TrimLeft(strings.TrimSpace(nit), "0")
	return movimientosByNIT[nit]
}

func (a *App) LookupCarteraPorNIT(nit string) []parsers.Cartera {
	if carteraByNIT == nil {
		a.RefreshCache("cartera")
	}
	nit = strings.TrimLeft(strings.TrimSpace(nit), "0")
	return carteraByNIT[nit]
}

// GetExtfhStatus returns info about EXTFH availability
func (a *App) GetExtfhStatus() map[string]interface{} {
	return map[string]interface{}{
		"available": isam.ExtfhAvailable(),
		"dll_path":  isam.ExtfhDLLPath(),
	}
}

// ==================== CONNECTION TEST ====================

func (a *App) TestConnection() string {
	client := api.NewClient(a.cfg.Finearom.BaseURL, a.cfg.Finearom.Email, a.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		return "Error: " + err.Error()
	}
	a.client = client
	a.db.AddLog("info", "API", "Connection successful with "+a.cfg.Finearom.BaseURL)
	return "ok"
}

// ==================== HISTORY & LOGS ====================

type PaginatedISAM struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
}

type PaginatedRecords struct {
	Records []storage.SyncRecord `json:"records"`
	Total   int                  `json:"total"`
}

type PaginatedLogs struct {
	Logs  []storage.LogEntry `json:"logs"`
	Total int                `json:"total"`
}

func (a *App) GetSyncHistory(tableName string, page int) PaginatedRecords {
	limit := 50
	offset := (page - 1) * limit
	records, total, err := a.db.GetSyncHistory(tableName, limit, offset)
	if err != nil {
		return PaginatedRecords{}
	}
	return PaginatedRecords{Records: records, Total: total}
}

func (a *App) SearchSyncHistory(tableName, search, dateFrom, dateTo, status string, page int) PaginatedRecords {
	limit := 50
	offset := (page - 1) * limit
	records, total, err := a.db.SearchSyncHistory(tableName, search, dateFrom, dateTo, status, limit, offset)
	if err != nil {
		return PaginatedRecords{}
	}
	return PaginatedRecords{Records: records, Total: total}
}

func (a *App) GetLogs(page int) PaginatedLogs {
	limit := 100
	offset := (page - 1) * limit
	logs, total, err := a.db.GetLogs(limit, offset)
	if err != nil {
		return PaginatedLogs{}
	}
	return PaginatedLogs{Logs: logs, Total: total}
}

func (a *App) GetStats() map[string]interface{} {
	return a.db.GetStats()
}

func (a *App) ClearDatabase() string {
	if err := a.db.ClearAll(); err != nil {
		return "Error: " + err.Error()
	}
	a.db.AddLog("warning", "APP", "Database cleared by user")
	return "ok"
}

func (a *App) ClearLogs() string {
	if err := a.db.ClearLogs(); err != nil {
		return "Error: " + err.Error()
	}
	a.db.AddLog("info", "APP", "Logs cleared by user")
	return "ok"
}
