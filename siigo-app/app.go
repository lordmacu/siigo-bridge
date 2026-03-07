package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"siigo-app/api"
	"siigo-app/config"
	"siigo-app/isam"
	"siigo-app/parsers"
	"siigo-app/storage"
	gosync "siigo-app/sync"
	"strings"
	"time"
)

type App struct {
	ctx      context.Context
	db       *storage.DB
	cfg      *config.Config
	client   *api.Client
	state    *gosync.SyncState
	syncing  bool
	looping  bool      // auto-sync loop running
	paused   bool      // user paused the loop
	stopCh   chan bool
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
	a.db.AddLog("info", "APP", "Aplicacion iniciada")

	cfg, err := config.Load("config.json")
	if err != nil {
		cfg = config.Default()
		cfg.Save("config.json")
		a.db.AddLog("warning", "APP", "Config no encontrada, se creo config por defecto")
	}
	a.cfg = cfg

	state, err := gosync.LoadState(cfg.Sync.StatePath)
	if err != nil {
		state = gosync.NewSyncState()
	}
	a.state = state

	// Auto-start sync loop
	go a.startSyncLoop()
}

func (a *App) shutdown(ctx context.Context) {
	// Stop the loop
	if a.looping {
		a.stopCh <- true
	}
	if a.db != nil {
		a.db.Close()
	}
	if a.state != nil && a.cfg != nil {
		a.state.Save(a.cfg.Sync.StatePath)
	}
}

// ==================== SYNC LOOP ====================

func (a *App) startSyncLoop() {
	a.looping = true
	a.paused = false
	a.db.AddLog("info", "SYNC", "Sync loop iniciado automaticamente")

	interval := time.Duration(a.cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	// Run first sync immediately
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
			a.db.AddLog("info", "SYNC", "Sync loop detenido")
			return
		}
	}
}

func (a *App) doOneSyncCycle() {
	a.syncing = true
	defer func() { a.syncing = false }()

	// Login
	client := api.NewClient(a.cfg.Finearom.BaseURL, a.cfg.Finearom.Email, a.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		a.db.AddLog("error", "API", "Login failed: "+err.Error())
		return
	}
	a.client = client

	a.db.AddLog("info", "SYNC", "--- Ciclo de sincronizacion iniciado ---")
	a.syncClientes()
	a.syncProductos()
	a.syncMovimientos()
	a.state.Save(a.cfg.Sync.StatePath)
	a.db.AddLog("info", "SYNC", "--- Ciclo de sincronizacion completado ---")
}

// PauseSync pauses the auto-sync loop
func (a *App) PauseSync() string {
	if a.paused {
		return "already_paused"
	}
	a.paused = true
	a.db.AddLog("info", "SYNC", "Sincronizacion pausada por el usuario")
	return "ok"
}

// ResumeSync resumes the auto-sync loop
func (a *App) ResumeSync() string {
	if !a.paused {
		return "already_running"
	}
	a.paused = false
	a.db.AddLog("info", "SYNC", "Sincronizacion reanudada por el usuario")
	return "ok"
}

// IsPaused returns whether the sync loop is paused
func (a *App) IsPaused() bool {
	return a.paused
}

// SyncNow triggers an immediate sync cycle (even if paused)
func (a *App) SyncNow() string {
	if a.syncing {
		return "Ya hay una sincronizacion en curso"
	}

	go a.doOneSyncCycle()
	return "Sincronizacion manual iniciada"
}

func (a *App) IsSyncing() bool {
	return a.syncing
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
	a.db.AddLog("info", "CONFIG", "Configuracion guardada")
	return "ok"
}

// ==================== ISAM DATA ====================

type ISAMPreview struct {
	File       string `json:"file"`
	RecordSize int    `json:"record_size"`
	Records    int    `json:"records"`
	ModTime    string `json:"mod_time"`
}

func (a *App) GetISAMInfo() []ISAMPreview {
	files := []string{"Z17", "Z06", "Z49"}
	var result []ISAMPreview
	for _, f := range files {
		path := a.cfg.Siigo.DataPath + f
		records, recSize, err := isam.ReadIsamFile(path)
		if err != nil {
			result = append(result, ISAMPreview{File: f, Records: -1})
			continue
		}
		modTime, _ := isam.GetModTime(path)
		t := time.Unix(0, modTime)
		result = append(result, ISAMPreview{
			File:       f,
			RecordSize: recSize,
			Records:    len(records),
			ModTime:    t.Format("2006-01-02 15:04:05"),
		})
	}
	return result
}

// ISAM cache - avoid re-reading files on every page request
var (
	cachedClientes    []parsers.Tercero
	cachedProductos   []parsers.Producto
	cachedMovimientos []parsers.Movimiento
)

func (a *App) RefreshCache(which string) {
	switch which {
	case "clients":
		c, err := parsers.ParseTercerosClientes(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z17", "Error leyendo clientes: "+err.Error())
			return
		}
		cachedClientes = c
		a.db.AddLog("info", "Z17", fmt.Sprintf("Cache actualizado: %d clientes", len(c)))
	case "products":
		p, err := parsers.ParseProductos(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z06", "Error leyendo productos: "+err.Error())
			return
		}
		cachedProductos = p
		a.db.AddLog("info", "Z06", fmt.Sprintf("Cache actualizado: %d productos", len(p)))
	case "movements":
		m, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z49", "Error leyendo movimientos: "+err.Error())
			return
		}
		cachedMovimientos = m
		a.db.AddLog("info", "Z49", fmt.Sprintf("Cache actualizado: %d movimientos", len(m)))
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
			if strings.Contains(strings.ToLower(c.NumeroDoc), q) ||
				strings.Contains(strings.ToLower(c.Nombre), q) ||
				strings.Contains(strings.ToLower(c.Empresa), q) ||
				strings.Contains(strings.ToLower(c.Codigo), q) {
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
		var filtered []parsers.Producto
		q := strings.ToLower(search)
		for _, p := range data {
			if strings.Contains(strings.ToLower(p.Codigo), q) ||
				strings.Contains(strings.ToLower(p.Nombre), q) {
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
			if strings.Contains(strings.ToLower(m.TipoComprobante), q) ||
				strings.Contains(strings.ToLower(m.NumeroDoc), q) ||
				strings.Contains(strings.ToLower(m.NitTercero), q) ||
				strings.Contains(strings.ToLower(m.Descripcion), q) ||
				strings.Contains(strings.ToLower(m.Fecha), q) {
				filtered = append(filtered, m)
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

// ==================== SYNC WORKERS ====================

func (a *App) TestConnection() string {
	client := api.NewClient(a.cfg.Finearom.BaseURL, a.cfg.Finearom.Email, a.cfg.Finearom.Password)
	if err := client.Login(); err != nil {
		return "Error: " + err.Error()
	}
	a.client = client
	a.db.AddLog("info", "API", "Conexion exitosa con "+a.cfg.Finearom.BaseURL)
	return "ok"
}

func (a *App) syncClientes() {
	clientes, err := parsers.ParseTercerosClientes(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z17", "Error: "+err.Error())
		return
	}

	sent, errors := 0, 0
	for _, t := range clientes {
		data := t.ToFinearomClient()
		jsonData, _ := json.Marshal(data)

		if err := a.client.SyncClient(data); err != nil {
			a.db.SaveSentRecord("clients", "Z17", t.NumeroDoc, string(jsonData), "error", err.Error(), t.Hash)
			errors++
			continue
		}
		a.db.SaveSentRecord("clients", "Z17", t.NumeroDoc, string(jsonData), "sent", "", t.Hash)
		sent++
	}
	a.db.AddLog("info", "Z17", fmt.Sprintf("Clientes: %d enviados, %d errores", sent, errors))
}

func (a *App) syncProductos() {
	productos, err := parsers.ParseProductos(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z06", "Error: "+err.Error())
		return
	}

	sent, errors := 0, 0
	for _, p := range productos {
		data := p.ToFinearomProduct()
		jsonData, _ := json.Marshal(data)

		if err := a.client.SyncProduct(data); err != nil {
			a.db.SaveSentRecord("products", "Z06", p.Codigo, string(jsonData), "error", err.Error(), p.Hash)
			errors++
			continue
		}
		a.db.SaveSentRecord("products", "Z06", p.Codigo, string(jsonData), "sent", "", p.Hash)
		sent++
	}
	a.db.AddLog("info", "Z06", fmt.Sprintf("Productos: %d enviados, %d errores", sent, errors))
}

func (a *App) syncMovimientos() {
	movimientos, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z49", "Error: "+err.Error())
		return
	}

	sent, errors := 0, 0
	for _, m := range movimientos {
		fecha := m.Fecha
		if len(fecha) == 8 {
			fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
		}
		data := map[string]interface{}{
			"tipo_comprobante": m.TipoComprobante,
			"numero_doc":      m.NumeroDoc,
			"fecha":           fecha,
			"nit_tercero":     m.NitTercero,
			"cuenta_contable": m.CuentaContable,
			"descripcion":     m.Descripcion,
			"valor":           m.Valor,
			"tipo_mov":        m.TipoMov,
			"siigo_sync_hash": m.Hash,
		}
		jsonData, _ := json.Marshal(data)
		key := m.TipoComprobante + "-" + m.NumeroDoc

		if err := a.client.SyncMovement(data); err != nil {
			a.db.SaveSentRecord("movements", "Z49", key, string(jsonData), "error", err.Error(), m.Hash)
			errors++
			continue
		}
		a.db.SaveSentRecord("movements", "Z49", key, string(jsonData), "sent", "", m.Hash)
		sent++
	}
	a.db.AddLog("info", "Z49", fmt.Sprintf("Movimientos: %d enviados, %d errores", sent, errors))
}

// ==================== RECORD DETAIL ====================

func (a *App) GetRecordDetail(id int64) *storage.SentRecord {
	record, err := a.db.GetRecordByID(id)
	if err != nil {
		return nil
	}
	return record
}

// ==================== RESEND ====================

func (a *App) ResendRecord(id int64) string {
	record, err := a.db.GetRecordByID(id)
	if err != nil {
		return "Error: registro no encontrado"
	}

	if a.client == nil || !a.client.IsAuthenticated() {
		client := api.NewClient(a.cfg.Finearom.BaseURL, a.cfg.Finearom.Email, a.cfg.Finearom.Password)
		if err := client.Login(); err != nil {
			return "Error login: " + err.Error()
		}
		a.client = client
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(record.Data), &data); err != nil {
		return "Error parsing data: " + err.Error()
	}

	var sendErr error
	switch record.Table {
	case "clients":
		sendErr = a.client.SyncClient(data)
	case "products":
		sendErr = a.client.SyncProduct(data)
	case "movements":
		sendErr = a.client.SyncMovement(data)
	}

	if sendErr != nil {
		a.db.UpdateSentRecord(id, "error", sendErr.Error())
		a.db.AddLog("error", record.SourceFile, "Reenvio fallido: "+sendErr.Error())
		return "Error: " + sendErr.Error()
	}

	a.db.UpdateSentRecord(id, "sent", "")
	a.db.AddLog("info", record.SourceFile, "Reenvio exitoso: "+record.Key)
	return "ok"
}

// ==================== HISTORY & LOGS ====================

type PaginatedISAM struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
}

type PaginatedRecords struct {
	Records []storage.SentRecord `json:"records"`
	Total   int                  `json:"total"`
}

type PaginatedLogs struct {
	Logs  []storage.LogEntry `json:"logs"`
	Total int                `json:"total"`
}

func (a *App) GetSentRecords(tableName string, page int) PaginatedRecords {
	return a.SearchSentRecords(tableName, "", page)
}

func (a *App) SearchSentRecords(tableName, search string, page int) PaginatedRecords {
	return a.SearchSentRecordsWithDates(tableName, search, "", "", "", page)
}

func (a *App) SearchSentRecordsWithDates(tableName, search, dateFrom, dateTo, status string, page int) PaginatedRecords {
	limit := 50
	offset := (page - 1) * limit
	records, total, err := a.db.SearchSentRecordsWithDates(tableName, search, dateFrom, dateTo, status, limit, offset)
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
	a.db.AddLog("warning", "APP", "Base de datos vaciada por el usuario")
	return "ok"
}

func (a *App) ClearLogs() string {
	if err := a.db.ClearLogs(); err != nil {
		return "Error: " + err.Error()
	}
	a.db.AddLog("info", "APP", "Logs limpiados por el usuario")
	return "ok"
}
