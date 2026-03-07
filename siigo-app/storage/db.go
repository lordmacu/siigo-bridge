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

type SentRecord struct {
	ID        int64  `json:"id"`
	Table     string `json:"table"`      // clients, products, movements
	SourceFile string `json:"source_file"` // Z17, Z06, Z49
	Key       string `json:"key"`        // NIT, codigo, hash
	Data      string `json:"data"`       // JSON of sent data
	Status    string `json:"status"`     // sent, error, pending
	Error     string `json:"error"`
	Hash      string `json:"hash"`
	SentAt    string `json:"sent_at"`
	CreatedAt string `json:"created_at"`
}

type LogEntry struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"`   // info, error, warning
	Source    string `json:"source"`  // Z17, Z06, Z49, API, SYNC
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
		`CREATE TABLE IF NOT EXISTS sent_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			source_file TEXT NOT NULL,
			record_key TEXT NOT NULL,
			data TEXT,
			status TEXT DEFAULT 'sent',
			error TEXT,
			hash TEXT,
			sent_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sent_table ON sent_records(table_name)`,
		`CREATE INDEX IF NOT EXISTS idx_sent_status ON sent_records(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sent_key ON sent_records(record_key)`,
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

func (db *DB) SaveSentRecord(tableName, sourceFile, key, data, status, errMsg, hash string) error {
	_, err := db.conn.Exec(
		`INSERT INTO sent_records (table_name, source_file, record_key, data, status, error, hash, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tableName, sourceFile, key, data, status, errMsg, hash, time.Now().Format(time.RFC3339),
	)
	return err
}

func (db *DB) UpdateSentRecord(id int64, status, errMsg string) error {
	_, err := db.conn.Exec(
		`UPDATE sent_records SET status=?, error=?, sent_at=? WHERE id=?`,
		status, errMsg, time.Now().Format(time.RFC3339), id,
	)
	return err
}

func (db *DB) GetSentRecords(tableName string, limit, offset int) ([]SentRecord, int, error) {
	return db.SearchSentRecords(tableName, "", limit, offset)
}

func (db *DB) SearchSentRecords(tableName, search string, limit, offset int) ([]SentRecord, int, error) {
	return db.SearchSentRecordsWithDates(tableName, search, "", "", "", limit, offset)
}

func (db *DB) SearchSentRecordsWithDates(tableName, search, dateFrom, dateTo, status string, limit, offset int) ([]SentRecord, int, error) {
	where := "table_name=?"
	args := []interface{}{tableName}

	if search != "" {
		like := "%" + search + "%"
		where += " AND (record_key LIKE ? OR data LIKE ? OR status LIKE ? OR error LIKE ?)"
		args = append(args, like, like, like, like)
	}
	if dateFrom != "" {
		where += " AND sent_at >= ?"
		args = append(args, dateFrom+"T00:00:00")
	}
	if dateTo != "" {
		where += " AND sent_at <= ?"
		args = append(args, dateTo+"T23:59:59")
	}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := db.conn.QueryRow("SELECT COUNT(*) FROM sent_records WHERE "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, table_name, source_file, record_key, data, status, error, hash, sent_at, created_at FROM sent_records WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []SentRecord
	for rows.Next() {
		var r SentRecord
		var errField, sentAt sql.NullString
		if err := rows.Scan(&r.ID, &r.Table, &r.SourceFile, &r.Key, &r.Data, &r.Status, &errField, &r.Hash, &sentAt, &r.CreatedAt); err != nil {
			log.Printf("scan error: %v", err)
			continue
		}
		r.Error = errField.String
		r.SentAt = sentAt.String
		records = append(records, r)
	}
	return records, total, nil
}

func (db *DB) GetRecordByID(id int64) (*SentRecord, error) {
	var r SentRecord
	var errField, sentAt sql.NullString
	err := db.conn.QueryRow(
		`SELECT id, table_name, source_file, record_key, data, status, error, hash, sent_at, created_at
		 FROM sent_records WHERE id=?`, id,
	).Scan(&r.ID, &r.Table, &r.SourceFile, &r.Key, &r.Data, &r.Status, &errField, &r.Hash, &sentAt, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.Error = errField.String
	r.SentAt = sentAt.String
	return &r, nil
}

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

func (db *DB) GetStats() map[string]interface{} {
	stats := map[string]interface{}{}

	var clientsSent, productsSent, movementsSent, errors int
	db.conn.QueryRow(`SELECT COUNT(*) FROM sent_records WHERE table_name='clients' AND status='sent'`).Scan(&clientsSent)
	db.conn.QueryRow(`SELECT COUNT(*) FROM sent_records WHERE table_name='products' AND status='sent'`).Scan(&productsSent)
	db.conn.QueryRow(`SELECT COUNT(*) FROM sent_records WHERE table_name='movements' AND status='sent'`).Scan(&movementsSent)
	db.conn.QueryRow(`SELECT COUNT(*) FROM sent_records WHERE status='error'`).Scan(&errors)

	stats["clients_sent"] = clientsSent
	stats["products_sent"] = productsSent
	stats["movements_sent"] = movementsSent
	stats["errors"] = errors

	return stats
}

func (db *DB) ClearLogs() error {
	_, err := db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) ClearAll() error {
	_, err := db.conn.Exec(`DELETE FROM sent_records`)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) Close() {
	db.conn.Close()
}
