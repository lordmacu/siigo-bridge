package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"isam-admin/internal/schemas"
	"isam-admin/pkg/isam"
)

// ---------------------------------------------------------------------------
// tables.go — Table CRUD handlers (open, list records, insert, update, delete)
// ---------------------------------------------------------------------------

// OpenTable represents a loaded ISAM table with its schema
type OpenTable struct {
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	RecSize    int               `json:"record_size"`
	RecCount   int               `json:"record_count"`
	Schema     *schemas.SavedSchema `json:"schema,omitempty"`
	table      *isam.Table
}

// TableManager manages currently opened tables
type TableManager struct {
	mu     sync.RWMutex
	tables map[string]*OpenTable
	schemas *schemas.Manager
}

// NewTableManager creates a new table manager
func NewTableManager(schemaManager *schemas.Manager) *TableManager {
	return &TableManager{
		tables:  make(map[string]*OpenTable),
		schemas: schemaManager,
	}
}

// OpenTableHandler opens/loads an ISAM file with an optional schema
func (tm *TableManager) OpenTableHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path       string `json:"path"`
		Name       string `json:"name"`
		SchemaName string `json:"schema_name,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	req.Path = filepath.Clean(req.Path)
	if req.Name == "" {
		req.Name = strings.TrimSuffix(filepath.Base(req.Path), filepath.Ext(req.Path))
	}

	// Read file to verify it's valid
	fi, hdr, err := isam.ReadIsamFile(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot read ISAM file: "+err.Error())
		return
	}

	recSize := int(hdr.MaxRecordLen)

	// Build ORM table
	var savedSchema *schemas.SavedSchema
	table := isam.NewTable(req.Name, req.Path, recSize)
	table.SafeMode = false

	// Load schema if specified
	if req.SchemaName != "" {
		s, err := tm.schemas.Load(req.SchemaName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Schema not found: "+err.Error())
			return
		}
		savedSchema = s
		applySchemaToTable(table, s)
	}

	ot := &OpenTable{
		Name:     req.Name,
		Path:     req.Path,
		RecSize:  recSize,
		RecCount: len(fi.Records),
		Schema:   savedSchema,
		table:    table,
	}

	tm.mu.Lock()
	tm.tables[req.Name] = ot
	tm.mu.Unlock()

	writeJSON(w, ot)
}

// ListTablesHandler returns all currently opened tables
func (tm *TableManager) ListTablesHandler(w http.ResponseWriter, r *http.Request) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var list []OpenTable
	for _, t := range tm.tables {
		// Refresh record count
		if fi, _, err := isam.ReadIsamFile(t.Path); err == nil {
			t.RecCount = len(fi.Records)
		}
		list = append(list, *t)
	}

	writeJSON(w, list)
}

// CloseTableHandler removes a table from the opened list
func (tm *TableManager) CloseTableHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	tm.mu.Lock()
	delete(tm.tables, name)
	tm.mu.Unlock()

	writeJSON(w, map[string]string{"status": "closed", "name": name})
}

// RecordsHandler lists records from an opened table with pagination
func (tm *TableManager) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	search := r.URL.Query().Get("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	tm.mu.RLock()
	ot, ok := tm.tables[name]
	tm.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "Table not open: "+name)
		return
	}

	// Read all records
	fi, _, err := isam.ReadIsamFile(ot.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read error: "+err.Error())
		return
	}

	type RecordData struct {
		Index  int               `json:"_index"`
		Fields map[string]string `json:"fields,omitempty"`
		Raw    string            `json:"raw,omitempty"`
	}

	// Parse records
	var allRecords []RecordData
	for i, rec := range fi.Records {
		rd := RecordData{Index: i}

		if ot.Schema != nil && len(ot.Schema.Fields) > 0 {
			rd.Fields = make(map[string]string)
			for _, f := range ot.Schema.Fields {
				if f.Offset+f.Length <= len(rec.Data) {
					val := strings.TrimRight(string(rec.Data[f.Offset:f.Offset+f.Length]), " \x00")
					rd.Fields[f.Name] = val
				}
			}
		} else {
			// No schema — return raw ASCII
			ascii := make([]byte, len(rec.Data))
			for j, b := range rec.Data {
				if b >= 32 && b < 127 {
					ascii[j] = b
				} else {
					ascii[j] = '.'
				}
			}
			rd.Raw = string(ascii)
		}

		// Apply search filter
		if search != "" {
			match := false
			searchLower := strings.ToLower(search)
			if rd.Fields != nil {
				for _, v := range rd.Fields {
					if strings.Contains(strings.ToLower(v), searchLower) {
						match = true
						break
					}
				}
			} else if strings.Contains(strings.ToLower(rd.Raw), searchLower) {
				match = true
			}
			if !match {
				continue
			}
		}

		allRecords = append(allRecords, rd)
	}

	// Paginate
	total := len(allRecords)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	writeJSON(w, map[string]interface{}{
		"name":      name,
		"records":   allRecords[start:end],
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     (total + pageSize - 1) / pageSize,
	})
}

// RecordCRUDHandler handles GET/PUT/DELETE for individual records
func (tm *TableManager) RecordCRUDHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	indexStr := r.URL.Query().Get("index")

	tm.mu.RLock()
	ot, ok := tm.tables[name]
	tm.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "Table not open: "+name)
		return
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	switch r.Method {
	case "GET":
		tm.getRecord(w, ot, index)
	case "PUT":
		tm.updateRecord(w, r, ot, index)
	case "DELETE":
		tm.deleteRecord(w, ot, index)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (tm *TableManager) getRecord(w http.ResponseWriter, ot *OpenTable, index int) {
	fi, _, err := isam.ReadIsamFile(ot.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read error: "+err.Error())
		return
	}

	if index < 0 || index >= len(fi.Records) {
		writeError(w, http.StatusNotFound, "Record index out of range")
		return
	}

	rec := fi.Records[index]
	result := map[string]interface{}{
		"index":       index,
		"record_size": len(rec.Data),
	}

	if ot.Schema != nil {
		fields := make(map[string]string)
		for _, f := range ot.Schema.Fields {
			if f.Offset+f.Length <= len(rec.Data) {
				fields[f.Name] = strings.TrimRight(string(rec.Data[f.Offset:f.Offset+f.Length]), " \x00")
			}
		}
		result["fields"] = fields
	}

	// Always include hex
	hex := ""
	for j, b := range rec.Data {
		if j > 0 {
			hex += " "
		}
		hex += fmt.Sprintf("%02X", b)
	}
	result["hex"] = hex

	writeJSON(w, result)
}

func (tm *TableManager) updateRecord(w http.ResponseWriter, r *http.Request, ot *OpenTable, index int) {
	if ot.Schema == nil {
		writeError(w, http.StatusBadRequest, "Cannot update without a schema")
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Build field patches
	fields := make(map[int][]byte)
	for fieldName, value := range req {
		for _, f := range ot.Schema.Fields {
			if f.Name == fieldName {
				// Pad value to field length
				padded := make([]byte, f.Length)
				for k := range padded {
					padded[k] = ' '
				}
				copy(padded, []byte(value))
				fields[f.Offset] = padded
				break
			}
		}
	}

	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "No valid fields to update")
		return
	}

	// Build key offsets for verification
	var keyOffsets [][2]int
	for _, f := range ot.Schema.Fields {
		if f.IsKey {
			keyOffsets = append(keyOffsets, [2]int{f.Offset, f.Length})
		}
	}

	result, err := isam.RewriteFields(ot.Path, index, fields, keyOffsets)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Update failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":     "updated",
		"index":      index,
		"offset":     result.FileOffset,
		"backup":     result.BackupPath,
	})
}

func (tm *TableManager) deleteRecord(w http.ResponseWriter, ot *OpenTable, index int) {
	result, err := isam.DeleteRecord(ot.Path, index)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": "deleted",
		"index":  index,
		"offset": result.FileOffset,
		"backup": result.BackupPath,
	})
}

// InsertRecordHandler inserts a new record into an opened table
func (tm *TableManager) InsertRecordHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	tm.mu.RLock()
	ot, ok := tm.tables[name]
	tm.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "Table not open: "+name)
		return
	}

	if ot.Schema == nil {
		writeError(w, http.StatusBadRequest, "Cannot insert without a schema")
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Build the record bytes
	recData := make([]byte, ot.RecSize)
	for k := range recData {
		recData[k] = ' ' // fill with spaces
	}

	for fieldName, value := range req {
		for _, f := range ot.Schema.Fields {
			if f.Name == fieldName {
				valBytes := []byte(value)
				end := f.Offset + f.Length
				if end > ot.RecSize {
					end = ot.RecSize
				}
				copy(recData[f.Offset:end], valBytes)
				break
			}
		}
	}

	result, err := isam.InsertRecord(ot.Path, recData, ot.Schema.KeyOffset, ot.Schema.KeyLength)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Insert failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": "inserted",
		"offset": result.FileOffset,
		"backup": result.BackupPath,
	})
}

func applySchemaToTable(table *isam.Table, s *schemas.SavedSchema) {
	for _, f := range s.Fields {
		switch f.Type {
		case "int":
			if f.IsKey {
				table.Key(f.Name, f.Offset, f.Length)
			} else {
				table.Int(f.Name, f.Offset, f.Length)
			}
		case "date":
			table.Date(f.Name, f.Offset, f.Length)
		case "bcd":
			table.BCD(f.Name, f.Offset, f.Length, f.Decimals)
		case "float":
			table.Float(f.Name, f.Offset, f.Length)
		default:
			if f.IsKey {
				table.Key(f.Name, f.Offset, f.Length)
			} else {
				table.String(f.Name, f.Offset, f.Length)
			}
		}
	}
}
