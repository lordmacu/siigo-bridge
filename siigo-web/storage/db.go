package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// ==================== TYPED RECORDS (mirror Siigo tables) ====================

type ClientRecord struct {
	ID            int64  `json:"id"`
	Nit           string `json:"nit"`
	Nombre        string `json:"nombre"`
	TipoDoc       string `json:"tipo_doc"`
	TipoClave     string `json:"tipo_clave"`
	Empresa       string `json:"empresa"`
	Codigo        string `json:"codigo"`
	FechaCreacion string `json:"fecha_creacion"`
	TipoCtaPref   string `json:"tipo_cta_pref"`
	Hash          string `json:"hash"`
	SyncStatus    string `json:"sync_status"`
	SyncAction    string `json:"sync_action"`
	SyncError     string `json:"sync_error"`
	UpdatedAt     string `json:"updated_at"`
	SyncedAt      string `json:"synced_at"`
}

type ProductRecord struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Nombre         string `json:"nombre"`
	Comprobante    string `json:"comprobante"`
	Secuencia      string `json:"secuencia"`
	TipoTercero    string `json:"tipo_tercero"`
	Grupo          string `json:"grupo"`
	CuentaContable string `json:"cuenta_contable"`
	Fecha          string `json:"fecha"`
	TipoMov        string `json:"tipo_mov"`
	Hash           string `json:"hash"`
	SyncStatus     string `json:"sync_status"`
	SyncAction     string `json:"sync_action"`
	SyncError      string `json:"sync_error"`
	UpdatedAt      string `json:"updated_at"`
	SyncedAt       string `json:"synced_at"`
}

type MovementRecord struct {
	ID              int64  `json:"id"`
	RecordKey       string `json:"record_key"`
	TipoComprobante string `json:"tipo_comprobante"`
	Empresa         string `json:"empresa"`
	NumeroDoc       string `json:"numero_doc"`
	Fecha           string `json:"fecha"`
	NitTercero      string `json:"nit_tercero"`
	CuentaContable  string `json:"cuenta_contable"`
	Descripcion     string `json:"descripcion"`
	Valor           string `json:"valor"`
	TipoMov         string `json:"tipo_mov"`
	Hash            string `json:"hash"`
	SyncStatus      string `json:"sync_status"`
	SyncAction      string `json:"sync_action"`
	SyncError       string `json:"sync_error"`
	UpdatedAt       string `json:"updated_at"`
	SyncedAt        string `json:"synced_at"`
}

type CarteraRecord struct {
	ID             int64  `json:"id"`
	RecordKey      string `json:"record_key"`
	TipoRegistro   string `json:"tipo_registro"`
	Empresa        string `json:"empresa"`
	Secuencia      string `json:"secuencia"`
	TipoDoc        string `json:"tipo_doc"`
	NitTercero     string `json:"nit_tercero"`
	CuentaContable string `json:"cuenta_contable"`
	Fecha          string `json:"fecha"`
	Descripcion    string `json:"descripcion"`
	TipoMov        string `json:"tipo_mov"`
	Hash           string `json:"hash"`
	SyncStatus     string `json:"sync_status"`
	SyncAction     string `json:"sync_action"`
	SyncError      string `json:"sync_error"`
	UpdatedAt      string `json:"updated_at"`
	SyncedAt       string `json:"synced_at"`
}

// PendingRecord is a generic record ready to send to the API
type PendingRecord struct {
	ID         int64
	Key        string
	SyncAction string
	Data       map[string]interface{}
}

// SyncRecord is used for sync_history display
type SyncRecord struct {
	ID         int64  `json:"id"`
	Table      string `json:"table"`
	Key        string `json:"key"`
	Data       string `json:"data"`
	Hash       string `json:"hash"`
	SyncStatus string `json:"sync_status"`
	SyncError  string `json:"sync_error"`
	SyncAction string `json:"sync_action"`
	UpdatedAt  string `json:"updated_at"`
	SyncedAt   string `json:"synced_at"`
}

type LogEntry struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) migrate() error {
	queries := []string{
		// Clients table (from Z17 - Terceros)
		`CREATE TABLE IF NOT EXISTS clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nit TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			tipo_clave TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			codigo TEXT DEFAULT '',
			fecha_creacion TEXT DEFAULT '',
			tipo_cta_pref TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_nit ON clients(nit)`,

		// Products table (from Z06CP)
		`CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			comprobante TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			tipo_tercero TEXT DEFAULT '',
			grupo TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_products_status ON products(sync_status)`,

		// Movements table (from Z49)
		`CREATE TABLE IF NOT EXISTS movements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_comprobante TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			numero_doc TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			valor TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_movements_status ON movements(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_movements_nit ON movements(nit_tercero)`,

		// Cartera table (from Z09YYYY)
		`CREATE TABLE IF NOT EXISTS cartera (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_registro TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cartera_status ON cartera(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_cartera_nit ON cartera(nit_tercero)`,

		// Sync history log
		`CREATE TABLE IF NOT EXISTS sync_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			record_key TEXT NOT NULL,
			action TEXT NOT NULL,
			data TEXT,
			status TEXT DEFAULT 'sent',
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_table ON sync_history(table_name)`,
		`CREATE INDEX IF NOT EXISTS idx_history_status ON sync_history(status)`,

		// App logs
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT DEFAULT 'info',
			source TEXT,
			message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_source ON logs(source)`,

		// Drop old generic table if it exists (migration from previous schema)
		`DROP TABLE IF EXISTS siigo_records`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// ==================== CLIENTS ====================

func (db *DB) GetAllClientHashes() map[string]string {
	return db.getAllHashes("clients", "nit")
}

func (db *DB) UpsertClient(nit, nombre, tipoDoc, tipoClave, empresa, codigo, fechaCreacion, tipoCtaPref, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM clients WHERE nit=?`, nit).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO clients (nit, nombre, tipo_doc, tipo_clave, empresa, codigo, fecha_creacion, tipo_cta_pref, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			nit, nombre, tipoDoc, tipoClave, empresa, codigo, fechaCreacion, tipoCtaPref, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE clients SET nombre=?, tipo_doc=?, tipo_clave=?, empresa=?, codigo=?, fecha_creacion=?, tipo_cta_pref=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE nit=?`,
			nombre, tipoDoc, tipoClave, empresa, codigo, fechaCreacion, tipoCtaPref, hash, now, nit,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedClients(currentKeys map[string]bool) int {
	return db.markDeleted("clients", "nit", currentKeys)
}

func (db *DB) GetPendingClients() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, nit, nombre, tipo_doc, tipo_clave, empresa, codigo, fecha_creacion, tipo_cta_pref, sync_action
		 FROM clients WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var nit, nombre, tipoDoc, tipoClave, empresa, codigo, fechaCreacion, tipoCtaPref, action string
		if err := rows.Scan(&id, &nit, &nombre, &tipoDoc, &tipoClave, &empresa, &codigo, &fechaCreacion, &tipoCtaPref, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: nit, SyncAction: action,
			Data: map[string]interface{}{
				"nit":             nit,
				"client_name":     nombre,
				"business_name":   nombre,
				"taxpayer_type":   tipoDoc,
				"tipo_clave":      tipoClave,
				"siigo_empresa":   empresa,
				"siigo_codigo":    codigo,
				"fecha_creacion":  fechaCreacion,
			},
		})
	}
	return records
}

// ==================== PRODUCTS ====================

func (db *DB) GetAllProductHashes() map[string]string {
	return db.getAllHashes("products", "code")
}

func (db *DB) UpsertProduct(code, nombre, comprobante, secuencia, tipoTercero, grupo, cuentaContable, fecha, tipoMov, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM products WHERE code=?`, code).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO products (code, nombre, comprobante, secuencia, tipo_tercero, grupo, cuenta_contable, fecha, tipo_mov, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			code, nombre, comprobante, secuencia, tipoTercero, grupo, cuentaContable, fecha, tipoMov, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE products SET nombre=?, comprobante=?, secuencia=?, tipo_tercero=?, grupo=?, cuenta_contable=?, fecha=?, tipo_mov=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE code=?`,
			nombre, comprobante, secuencia, tipoTercero, grupo, cuentaContable, fecha, tipoMov, hash, now, code,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedProducts(currentKeys map[string]bool) int {
	return db.markDeleted("products", "code", currentKeys)
}

func (db *DB) GetPendingProducts() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, code, nombre, grupo, cuenta_contable, sync_action
		 FROM products WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var code, nombre, grupo, cuentaContable, action string
		if err := rows.Scan(&id, &code, &nombre, &grupo, &cuentaContable, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: code, SyncAction: action,
			Data: map[string]interface{}{
				"code":            code,
				"product_name":    nombre,
				"grupo":           grupo,
				"cuenta_contable": cuentaContable,
			},
		})
	}
	return records
}

// ==================== MOVEMENTS ====================

func (db *DB) GetAllMovementHashes() map[string]string {
	return db.getAllHashes("movements", "record_key")
}

func (db *DB) UpsertMovement(key, tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM movements WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO movements (record_key, tipo_comprobante, empresa, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE movements SET tipo_comprobante=?, empresa=?, numero_doc=?, fecha=?, nit_tercero=?, cuenta_contable=?, descripcion=?, valor=?, tipo_mov=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedMovements(currentKeys map[string]bool) int {
	return db.markDeleted("movements", "record_key", currentKeys)
}

func (db *DB) GetPendingMovements() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, record_key, tipo_comprobante, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, sync_action
		 FROM movements WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var key, tipoComp, numDoc, fecha, nit, cuenta, desc, valor, tipoMov, action string
		if err := rows.Scan(&id, &key, &tipoComp, &numDoc, &fecha, &nit, &cuenta, &desc, &valor, &tipoMov, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: key, SyncAction: action,
			Data: map[string]interface{}{
				"tipo_comprobante": tipoComp,
				"numero_doc":       numDoc,
				"fecha":            fecha,
				"nit_tercero":      nit,
				"cuenta_contable":  cuenta,
				"descripcion":      desc,
				"valor":            valor,
				"tipo_mov":         tipoMov,
			},
		})
	}
	return records
}

// ==================== CARTERA ====================

func (db *DB) GetAllCarteraHashes() map[string]string {
	return db.getAllHashes("cartera", "record_key")
}

func (db *DB) UpsertCartera(key, tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM cartera WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO cartera (record_key, tipo_registro, empresa, secuencia, tipo_doc, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE cartera SET tipo_registro=?, empresa=?, secuencia=?, tipo_doc=?, nit_tercero=?, cuenta_contable=?, fecha=?, descripcion=?, tipo_mov=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedCartera(currentKeys map[string]bool) int {
	return db.markDeleted("cartera", "record_key", currentKeys)
}

func (db *DB) GetPendingCartera() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, record_key, tipo_registro, empresa, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, sync_action
		 FROM cartera WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var key, tipoReg, empresa, nit, cuenta, fecha, desc, tipoMov, action string
		if err := rows.Scan(&id, &key, &tipoReg, &empresa, &nit, &cuenta, &fecha, &desc, &tipoMov, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: key, SyncAction: action,
			Data: map[string]interface{}{
				"nit":             nit,
				"cuenta_contable": cuenta,
				"fecha":           fecha,
				"tipo_movimiento": tipoMov,
				"descripcion":     desc,
				"tipo_registro":   tipoReg,
			},
		})
	}
	return records
}

// ==================== GENERIC SYNC HELPERS ====================

func (db *DB) getAllHashes(table, keyCol string) map[string]string {
	hashes := make(map[string]string)
	rows, err := db.conn.Query(
		fmt.Sprintf(`SELECT %s, hash FROM %s`, keyCol, table),
	)
	if err != nil {
		return hashes
	}
	defer rows.Close()

	for rows.Next() {
		var key, hash string
		if err := rows.Scan(&key, &hash); err == nil {
			hashes[key] = hash
		}
	}
	return hashes
}

func (db *DB) markDeleted(table, keyCol string, currentKeys map[string]bool) int {
	rows, err := db.conn.Query(
		fmt.Sprintf(`SELECT %s FROM %s WHERE sync_action != 'delete'`, keyCol, table),
	)
	if err != nil {
		return 0
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil {
			if !currentKeys[key] {
				toDelete = append(toDelete, key)
			}
		}
	}

	now := time.Now().Format(time.RFC3339)
	for _, key := range toDelete {
		db.conn.Exec(
			fmt.Sprintf(`UPDATE %s SET sync_status='pending', sync_action='delete', updated_at=? WHERE %s=?`, table, keyCol),
			now, key,
		)
	}
	return len(toDelete)
}

// MarkSynced marks a record as successfully synced
func (db *DB) MarkSynced(table string, id int64) {
	now := time.Now().Format(time.RFC3339)
	db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='synced', sync_error='', synced_at=? WHERE id=?`, table),
		now, id,
	)
}

// MarkSyncError marks a record as failed to sync
func (db *DB) MarkSyncError(table string, id int64, errMsg string) {
	db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='error', sync_error=? WHERE id=?`, table),
		errMsg, id,
	)
}

// RemoveDeletedSynced removes records that were deleted and successfully synced
func (db *DB) RemoveDeletedSynced(table string) {
	db.conn.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE sync_action='delete' AND sync_status='synced'`, table),
	)
}

// RetryErrors resets error records to pending so they get retried
func (db *DB) RetryErrors(table string) int {
	result, err := db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='pending' WHERE sync_status='error'`, table),
	)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// ==================== SYNC HISTORY ====================

func (db *DB) AddSyncHistory(tableName, key, action, data, status, errMsg string) {
	db.conn.Exec(
		`INSERT INTO sync_history (table_name, record_key, action, data, status, error)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tableName, key, action, data, status, errMsg,
	)
}

func (db *DB) GetSyncHistory(tableName string, limit, offset int) ([]SyncRecord, int, error) {
	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM sync_history WHERE table_name=?`, tableName).Scan(&total)

	rows, err := db.conn.Query(
		`SELECT id, table_name, record_key, COALESCE(data,''), '', status, COALESCE(error,''), action, created_at, ''
		 FROM sync_history WHERE table_name=? ORDER BY id DESC LIMIT ? OFFSET ?`,
		tableName, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		if err := rows.Scan(&r.ID, &r.Table, &r.Key, &r.Data, &r.Hash, &r.SyncStatus, &r.SyncError, &r.SyncAction, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total, nil
}

func (db *DB) SearchSyncHistory(tableName, search, dateFrom, dateTo, status string, limit, offset int) ([]SyncRecord, int, error) {
	where := "table_name=?"
	args := []interface{}{tableName}

	if search != "" {
		like := "%" + search + "%"
		where += " AND (record_key LIKE ? OR data LIKE ? OR action LIKE ?)"
		args = append(args, like, like, like)
	}
	if dateFrom != "" {
		where += " AND created_at >= ?"
		args = append(args, dateFrom+"T00:00:00")
	}
	if dateTo != "" {
		where += " AND created_at <= ?"
		args = append(args, dateTo+"T23:59:59")
	}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	db.conn.QueryRow("SELECT COUNT(*) FROM sync_history WHERE "+where, countArgs...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, table_name, record_key, COALESCE(data,''), '', status, COALESCE(error,''), action, created_at, '' FROM sync_history WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		if err := rows.Scan(&r.ID, &r.Table, &r.Key, &r.Data, &r.Hash, &r.SyncStatus, &r.SyncError, &r.SyncAction, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total, nil
}

// ==================== STATS ====================

func (db *DB) GetStats() map[string]interface{} {
	stats := map[string]interface{}{}

	tables := []string{"clients", "products", "movements", "cartera"}
	for _, t := range tables {
		var total, synced, pending, errors int
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, t)).Scan(&total)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='synced'`, t)).Scan(&synced)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='pending'`, t)).Scan(&pending)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='error'`, t)).Scan(&errors)
		stats[t+"_total"] = total
		stats[t+"_synced"] = synced
		stats[t+"_pending"] = pending
		stats[t+"_errors"] = errors
	}

	return stats
}

// ==================== QUERY TABLES (for UI display) ====================

func (db *DB) GetClients(limit, offset int, search string) ([]ClientRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit) LIKE ? OR LOWER(nombre) LIKE ? OR LOWER(codigo) LIKE ?)"
		args = append(args, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM clients WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, nit, nombre, tipo_doc, tipo_clave, empresa, codigo, fecha_creacion, tipo_cta_pref, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM clients WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []ClientRecord
	for rows.Next() {
		var r ClientRecord
		if err := rows.Scan(&r.ID, &r.Nit, &r.Nombre, &r.TipoDoc, &r.TipoClave, &r.Empresa, &r.Codigo, &r.FechaCreacion, &r.TipoCtaPref, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			log.Printf("scan client error: %v", err)
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetProducts(limit, offset int, search string) ([]ProductRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nombre) LIKE ? OR LOWER(code) LIKE ? OR LOWER(grupo) LIKE ?)"
		args = append(args, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM products WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, code, nombre, comprobante, secuencia, tipo_tercero, grupo, cuenta_contable, fecha, tipo_mov, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM products WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []ProductRecord
	for rows.Next() {
		var r ProductRecord
		if err := rows.Scan(&r.ID, &r.Code, &r.Nombre, &r.Comprobante, &r.Secuencia, &r.TipoTercero, &r.Grupo, &r.CuentaContable, &r.Fecha, &r.TipoMov, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetMovements(limit, offset int, search string) ([]MovementRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(tipo_comprobante) LIKE ? OR LOWER(numero_doc) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM movements WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, tipo_comprobante, empresa, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM movements WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []MovementRecord
	for rows.Next() {
		var r MovementRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.TipoComprobante, &r.Empresa, &r.NumeroDoc, &r.Fecha, &r.NitTercero, &r.CuentaContable, &r.Descripcion, &r.Valor, &r.TipoMov, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetCarteraRecords(limit, offset int, search string) ([]CarteraRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(cuenta_contable) LIKE ? OR LOWER(tipo_registro) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM cartera WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, tipo_registro, empresa, secuencia, tipo_doc, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM cartera WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []CarteraRecord
	for rows.Next() {
		var r CarteraRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.TipoRegistro, &r.Empresa, &r.Secuencia, &r.TipoDoc, &r.NitTercero, &r.CuentaContable, &r.Fecha, &r.Descripcion, &r.TipoMov, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== LOGS ====================

func (db *DB) AddLog(level, source, message string) {
	db.conn.Exec(
		`INSERT INTO logs (level, source, message) VALUES (?, ?, ?)`,
		level, source, message,
	)
}

func (db *DB) GetLogs(limit, offset int) ([]LogEntry, int, error) {
	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM logs`).Scan(&total)

	rows, err := db.conn.Query(
		`SELECT id, level, source, message, created_at FROM logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Level, &l.Source, &l.Message, &l.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// ==================== CLEANUP ====================

func (db *DB) ClearLogs() error {
	_, err := db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) ClearAll() error {
	db.conn.Exec(`DELETE FROM clients`)
	db.conn.Exec(`DELETE FROM products`)
	db.conn.Exec(`DELETE FROM movements`)
	db.conn.Exec(`DELETE FROM cartera`)
	db.conn.Exec(`DELETE FROM sync_history`)
	_, err := db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) Close() {
	db.conn.Close()
}
