package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"siigo-app/api"
	"siigo-app/config"
	"siigo-common/isam"
	"siigo-common/parsers"
	"siigo-app/storage"
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
	a.db.AddLog("info", "APP", "Aplicacion iniciada")

	cfg, err := config.Load("config.json")
	if err != nil {
		cfg = config.Default()
		cfg.Save("config.json")
		a.db.AddLog("warning", "APP", "Config no encontrada, se creo config por defecto")
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
	a.db.AddLog("info", "SYNC", "Sync loop iniciado")

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
			a.db.AddLog("info", "SYNC", "Sync loop detenido")
			return
		}
	}
}

func (a *App) doOneSyncCycle() {
	a.syncing = true
	defer func() { a.syncing = false }()

	a.db.AddLog("info", "SYNC", "--- Ciclo iniciado ---")

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

	a.db.AddLog("info", "SYNC", "--- Ciclo completado ---")
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

func (a *App) IsPaused() bool  { return a.paused }
func (a *App) IsSyncing() bool { return a.syncing }

// SyncNow triggers an immediate sync cycle
func (a *App) SyncNow() string {
	if a.syncing {
		return "Ya hay una sincronizacion en curso"
	}
	go a.doOneSyncCycle()
	return "Sincronizacion manual iniciada"
}

// RetryErrors resets all error records to pending for a given table
func (a *App) RetryErrors(tableName string) string {
	n := a.db.RetryErrors(tableName)
	a.db.AddLog("info", "SYNC", fmt.Sprintf("Reintentando %d registros con error en %s", n, tableName))
	return fmt.Sprintf("ok: %d registros marcados para reenvio", n)
}

// ==================== DIFF: Parse ISAM → Compare with SQLite ====================

func (a *App) diffClientes() {
	clientes, err := parsers.ParseTercerosClientes(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z17", "Error parseando: "+err.Error())
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

		data := t.ToFinearomClient()
		jsonData, _ := json.Marshal(data)
		action := a.db.UpsertRecord("clients", nit, string(jsonData), t.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeleted("clients", currentKeys)
	a.db.AddLog("info", "Z17", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(clientes)))
}

func (a *App) diffProductos() {
	productos, err := parsers.ParseProductos(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z06CP", "Error parseando: "+err.Error())
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

		data := p.ToFinearomProduct()
		jsonData, _ := json.Marshal(data)
		action := a.db.UpsertRecord("products", key, string(jsonData), p.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeleted("products", currentKeys)
	a.db.AddLog("info", "Z06CP", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(productos)))
}

func (a *App) diffMovimientos() {
	movimientos, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
	if err != nil {
		a.db.AddLog("error", "Z49", "Error parseando: "+err.Error())
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
		data := map[string]interface{}{
			"tipo_comprobante": m.TipoComprobante,
			"numero_doc":       m.NumeroDoc,
			"fecha":            fecha,
			"nit_tercero":      m.NitTercero,
			"cuenta_contable":  m.CuentaContable,
			"descripcion":      m.Descripcion,
			"valor":            m.Valor,
			"tipo_mov":         m.TipoMov,
		}
		jsonData, _ := json.Marshal(data)
		action := a.db.UpsertRecord("movements", key, string(jsonData), m.Hash)
		switch action {
		case "add":
			adds++
		case "edit":
			edits++
		}
	}

	deletes := a.db.MarkDeleted("movements", currentKeys)
	a.db.AddLog("info", "Z49", fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(movimientos)))
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
			a.db.AddLog("error", file, "Error parseando: "+err.Error())
			continue
		}

		adds, edits := 0, 0
		currentKeys := make(map[string]bool, len(cartera))

		for _, c := range cartera {
			key := file + "-" + c.TipoRegistro + "-" + c.Empresa + "-" + c.Secuencia
			currentKeys[key] = true

			data := c.ToFinearomCartera()
			jsonData, _ := json.Marshal(data)
			action := a.db.UpsertRecord("cartera", key, string(jsonData), c.Hash)
			switch action {
			case "add":
				adds++
			case "edit":
				edits++
			}
		}

		// Only mark deletes for this specific year file
		// We prefix keys with the file name so different years don't conflict
		deletes := a.db.MarkDeleted("cartera", currentKeys)
		a.db.AddLog("info", file, fmt.Sprintf("Diff: %d nuevos, %d editados, %d eliminados (de %d)", adds, edits, deletes, len(cartera)))
	}
}

// ==================== SEND PENDING: SQLite → Server ====================

func (a *App) sendPending(tableName string) {
	pending := a.db.GetPendingRecords(tableName)
	if len(pending) == 0 {
		return
	}

	sent, errors := 0, 0
	for _, rec := range pending {
		var data map[string]interface{}
		if rec.SyncAction != "delete" {
			if err := json.Unmarshal([]byte(rec.Data), &data); err != nil {
				a.db.MarkSyncError(rec.ID, "JSON parse error: "+err.Error())
				errors++
				continue
			}
		}

		err := a.client.Sync(tableName, rec.SyncAction, rec.Key, data)
		if err != nil {
			a.db.MarkSyncError(rec.ID, err.Error())
			a.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, rec.Data, "error", err.Error())
			errors++
			continue
		}

		a.db.MarkSynced(rec.ID)
		a.db.AddSyncHistory(tableName, rec.Key, rec.SyncAction, rec.Data, "sent", "")
		sent++
	}

	// Clean up deleted records that were successfully synced
	a.db.RemoveDeletedSynced(tableName)

	a.db.AddLog("info", "API", fmt.Sprintf("[%s] Enviados: %d, Errores: %d (de %d pendientes)", tableName, sent, errors, len(pending)))
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
	NumKeys    int    `json:"num_keys"`
	HasIndex   bool   `json:"has_index"`
	UsedEXTFH  bool   `json:"used_extfh"`
	Format     int    `json:"format"`
	ModTime    string `json:"mod_time"`
}

func (a *App) GetISAMInfo() []ISAMPreview {
	files := []string{"Z17", "Z06", "Z06CP", "Z49"}
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
	cachedProductos   []parsers.Producto
	cachedMovimientos []parsers.Movimiento
	cachedCartera     []parsers.Cartera
	clientesByNIT     map[string]*parsers.Tercero
	productosByCodigo map[string]*parsers.Producto
	movimientosByNIT  map[string][]parsers.Movimiento
	carteraByNIT      map[string][]parsers.Cartera
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
		clientesByNIT = make(map[string]*parsers.Tercero, len(c))
		for i := range c {
			nit := strings.TrimLeft(c[i].NumeroDoc, "0")
			if nit != "" {
				clientesByNIT[nit] = &cachedClientes[i]
			}
		}
		a.db.AddLog("info", "Z17", fmt.Sprintf("Cache: %d clientes", len(c)))
	case "products":
		p, err := parsers.ParseProductos(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z06CP", "Error leyendo productos: "+err.Error())
			return
		}
		cachedProductos = p
		productosByCodigo = make(map[string]*parsers.Producto, len(p))
		for i := range p {
			key := p[i].Comprobante + "-" + p[i].Secuencia
			if key != "-" {
				productosByCodigo[key] = &cachedProductos[i]
			}
		}
		a.db.AddLog("info", "Z06CP", fmt.Sprintf("Cache: %d productos", len(p)))
	case "movements":
		m, err := parsers.ParseMovimientos(a.cfg.Siigo.DataPath)
		if err != nil {
			a.db.AddLog("error", "Z49", "Error leyendo movimientos: "+err.Error())
			return
		}
		cachedMovimientos = m
		movimientosByNIT = make(map[string][]parsers.Movimiento)
		for _, mov := range m {
			nit := strings.TrimLeft(mov.NitTercero, "0")
			if nit != "" {
				movimientosByNIT[nit] = append(movimientosByNIT[nit], mov)
			}
		}
		a.db.AddLog("info", "Z49", fmt.Sprintf("Cache: %d movimientos", len(m)))
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
				a.db.AddLog("error", file, "Error leyendo cartera: "+err.Error())
				continue
			}
			all = append(all, c...)
		}
		cachedCartera = all
		carteraByNIT = make(map[string][]parsers.Cartera)
		for _, c := range all {
			nit := strings.TrimLeft(c.NitTercero, "0")
			if nit != "" {
				carteraByNIT[nit] = append(carteraByNIT[nit], c)
			}
		}
		a.db.AddLog("info", "Z09", fmt.Sprintf("Cache: %d cartera", len(all)))
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
			if strings.Contains(strings.ToLower(p.Nombre), q) ||
				strings.Contains(strings.ToLower(p.Comprobante), q) ||
				strings.Contains(strings.ToLower(p.Grupo), q) {
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

func (a *App) GetCartera(page int, search string) PaginatedISAM {
	if cachedCartera == nil {
		a.RefreshCache("cartera")
	}
	data := cachedCartera
	if search != "" {
		var filtered []parsers.Cartera
		q := strings.ToLower(search)
		for _, c := range data {
			if strings.Contains(strings.ToLower(c.NitTercero), q) ||
				strings.Contains(strings.ToLower(c.Descripcion), q) ||
				strings.Contains(strings.ToLower(c.CuentaContable), q) ||
				strings.Contains(strings.ToLower(c.Fecha), q) ||
				strings.Contains(strings.ToLower(c.TipoRegistro), q) {
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

func (a *App) LookupProducto(codigo string) *parsers.Producto {
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
	a.db.AddLog("info", "API", "Conexion exitosa con "+a.cfg.Finearom.BaseURL)
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
