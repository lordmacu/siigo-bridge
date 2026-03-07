package storage

import (
	"database/sql"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// SyncRecord represents a record stored locally from ISAM, with sync status
type SyncRecord struct {
	ID         int64  `json:"id"`
	Table      string `json:"table"`       // clients, products, movements, cartera
	Key        string `json:"key"`         // unique key (NIT, code, etc.)
	Data       string `json:"data"`        // JSON of the record fields
	Hash       string `json:"hash"`        // SHA256 hash for change detection
	SyncStatus string `json:"sync_status"` // synced, pending, error
	SyncError  string `json:"sync_error"`
	SyncAction string `json:"sync_action"` // add, edit, delete
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
		// Main data table: mirrors what we parse from ISAM and send to server
		`CREATE TABLE IF NOT EXISTS siigo_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			record_key TEXT NOT NULL,
			data TEXT NOT NULL,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_error TEXT,
			sync_action TEXT DEFAULT 'add',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME,
			UNIQUE(table_name, record_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_siigo_table ON siigo_records(table_name)`,
		`CREATE INDEX IF NOT EXISTS idx_siigo_status ON siigo_records(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_siigo_table_status ON siigo_records(table_name, sync_status)`,

		// Sync history log (what was sent to server and when)
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
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// ==================== SIIGO RECORDS (local mirror) ====================

// GetAllHashes returns a map of key→hash for a given table (for diff detection)
func (db *DB) GetAllHashes(tableName string) map[string]string {
	hashes := make(map[string]string)
	rows, err := db.conn.Query(
		`SELECT record_key, hash FROM siigo_records WHERE table_name=?`, tableName,
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

// UpsertRecord inserts or updates a record in the local mirror.
// Returns the action taken: "add" if new, "edit" if changed, "" if unchanged.
func (db *DB) UpsertRecord(tableName, key, data, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(
		`SELECT hash FROM siigo_records WHERE table_name=? AND record_key=?`,
		tableName, key,
	).Scan(&existingHash)

	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		// New record
		db.conn.Exec(
			`INSERT INTO siigo_records (table_name, record_key, data, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, 'pending', 'add', ?)`,
			tableName, key, data, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		// Changed record
		db.conn.Exec(
			`UPDATE siigo_records SET data=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE table_name=? AND record_key=?`,
			data, hash, now, tableName, key,
		)
		return "edit"
	}

	// Unchanged
	return ""
}

// MarkDeleted marks records that no longer exist in ISAM as pending delete.
// currentKeys is the set of keys that exist in the current ISAM parse.
func (db *DB) MarkDeleted(tableName string, currentKeys map[string]bool) int {
	rows, err := db.conn.Query(
		`SELECT record_key FROM siigo_records WHERE table_name=? AND sync_action != 'delete'`,
		tableName,
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
			`UPDATE siigo_records SET sync_status='pending', sync_action='delete', updated_at=?
			 WHERE table_name=? AND record_key=?`,
			now, tableName, key,
		)
	}
	return len(toDelete)
}

// GetPendingRecords returns all records that need to be synced to the server
func (db *DB) GetPendingRecords(tableName string) []SyncRecord {
	rows, err := db.conn.Query(
		`SELECT id, table_name, record_key, data, hash, sync_status, COALESCE(sync_error,''), sync_action, updated_at, COALESCE(synced_at,'')
		 FROM siigo_records WHERE table_name=? AND sync_status='pending'
		 ORDER BY id`,
		tableName,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		if err := rows.Scan(&r.ID, &r.Table, &r.Key, &r.Data, &r.Hash, &r.SyncStatus, &r.SyncError, &r.SyncAction, &r.UpdatedAt, &r.SyncedAt); err != nil {
			log.Printf("scan error: %v", err)
			continue
		}
		records = append(records, r)
	}
	return records
}

// MarkSynced marks a record as successfully synced
func (db *DB) MarkSynced(id int64) {
	now := time.Now().Format(time.RFC3339)
	db.conn.Exec(
		`UPDATE siigo_records SET sync_status='synced', sync_error='', synced_at=? WHERE id=?`,
		now, id,
	)
}

// MarkSyncError marks a record as failed to sync
func (db *DB) MarkSyncError(id int64, errMsg string) {
	db.conn.Exec(
		`UPDATE siigo_records SET sync_status='error', sync_error=? WHERE id=?`,
		errMsg, id,
	)
}

// RemoveDeletedSynced removes records that were deleted and successfully synced
func (db *DB) RemoveDeletedSynced(tableName string) {
	db.conn.Exec(
		`DELETE FROM siigo_records WHERE table_name=? AND sync_action='delete' AND sync_status='synced'`,
		tableName,
	)
}

// RetryErrors resets error records to pending so they get retried
func (db *DB) RetryErrors(tableName string) int {
	result, err := db.conn.Exec(
		`UPDATE siigo_records SET sync_status='pending' WHERE table_name=? AND sync_status='error'`,
		tableName,
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
		db.conn.QueryRow(`SELECT COUNT(*) FROM siigo_records WHERE table_name=?`, t).Scan(&total)
		db.conn.QueryRow(`SELECT COUNT(*) FROM siigo_records WHERE table_name=? AND sync_status='synced'`, t).Scan(&synced)
		db.conn.QueryRow(`SELECT COUNT(*) FROM siigo_records WHERE table_name=? AND sync_status='pending'`, t).Scan(&pending)
		db.conn.QueryRow(`SELECT COUNT(*) FROM siigo_records WHERE table_name=? AND sync_status='error'`, t).Scan(&errors)
		stats[t+"_total"] = total
		stats[t+"_synced"] = synced
		stats[t+"_pending"] = pending
		stats[t+"_errors"] = errors
	}

	return stats
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
	db.conn.Exec(`DELETE FROM siigo_records`)
	db.conn.Exec(`DELETE FROM sync_history`)
	_, err := db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) Close() {
	db.conn.Close()
}
