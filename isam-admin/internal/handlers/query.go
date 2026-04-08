package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"isam-admin/pkg/isam"
)

// ---------------------------------------------------------------------------
// query.go — Query execution handler
//
// Accepts a query specification (table, filters, sort, pagination)
// and runs it against the ORM query builder.
// ---------------------------------------------------------------------------

// QueryRequest describes a query to execute
type QueryRequest struct {
	Table   string        `json:"table"`   // opened table name
	Filters []QueryFilter `json:"filters"` // WHERE conditions
	OrderBy string        `json:"order_by,omitempty"`
	OrderDir string       `json:"order_dir,omitempty"` // "asc" or "desc"
	Limit   int           `json:"limit,omitempty"`
	Offset  int           `json:"offset,omitempty"`
	Select  []string      `json:"select,omitempty"` // field names to return
	GroupBy string        `json:"group_by,omitempty"`
	Distinct string       `json:"distinct,omitempty"`
}

// QueryFilter is a single WHERE condition
type QueryFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // =, !=, >, <, >=, <=, contains, starts_with, ends_with
	Value    string `json:"value"`
}

// QueryHandler executes queries against opened tables
func (tm *TableManager) QueryHandler(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid query: "+err.Error())
		return
	}

	tm.mu.RLock()
	ot, ok := tm.tables[req.Table]
	tm.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "Table not open: "+req.Table)
		return
	}

	if ot.Schema == nil || len(ot.Schema.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "Table has no schema — cannot query by fields")
		return
	}

	// Build query
	q := ot.table.Query()

	for _, f := range req.Filters {
		q = q.Where(f.Field, f.Operator, f.Value)
	}

	if req.OrderBy != "" {
		dir := "asc"
		if req.OrderDir != "" {
			dir = req.OrderDir
		}
		q = q.OrderBy(req.OrderBy, dir)
	}

	if req.Limit > 0 {
		q = q.Limit(req.Limit)
	} else {
		q = q.Limit(100) // default limit
	}

	if req.Offset > 0 {
		q = q.Offset(req.Offset)
	}

	if len(req.Select) > 0 {
		q = q.Select(req.Select...)
	}

	// Handle special queries
	if req.Distinct != "" {
		values, err := q.Distinct(req.Distinct)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Query error: "+err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{
			"type":   "distinct",
			"field":  req.Distinct,
			"values": values,
			"count":  len(values),
		})
		return
	}

	if req.GroupBy != "" {
		groups, err := q.GroupByCount(req.GroupBy)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Query error: "+err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{
			"type":   "group_by",
			"field":  req.GroupBy,
			"groups": groups,
		})
		return
	}

	// Execute standard query
	results, err := q.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query error: "+err.Error())
		return
	}

	// Convert to maps
	type ResultRecord struct {
		Index  int               `json:"_index"`
		Fields map[string]string `json:"fields"`
	}

	var records []ResultRecord
	for _, row := range results {
		rec := ResultRecord{
			Index:  row.Index(),
			Fields: make(map[string]string),
		}
		for _, f := range ot.Schema.Fields {
			rec.Fields[f.Name] = strings.TrimSpace(row.Get(f.Name))
		}
		records = append(records, rec)
	}

	// Get explain plan
	explain := q.Explain()

	writeJSON(w, map[string]interface{}{
		"type":    "records",
		"records": records,
		"count":   len(records),
		"explain": explain,
	})
}

// QuickQueryHandler handles simple GET queries with URL parameters
func (tm *TableManager) QuickQueryHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	field := r.URL.Query().Get("field")
	op := r.URL.Query().Get("op")
	value := r.URL.Query().Get("value")
	limitStr := r.URL.Query().Get("limit")

	tm.mu.RLock()
	ot, ok := tm.tables[name]
	tm.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "Table not open: "+name)
		return
	}

	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	fi, _, err := isam.ReadIsamFile(ot.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read error: "+err.Error())
		return
	}

	type ResultRecord struct {
		Index  int               `json:"_index"`
		Fields map[string]string `json:"fields,omitempty"`
		Raw    string            `json:"raw,omitempty"`
	}

	var results []ResultRecord
	for i, rec := range fi.Records {
		if len(results) >= limit {
			break
		}

		// If we have a filter, apply it
		if field != "" && op != "" && value != "" && ot.Schema != nil {
			match := false
			for _, f := range ot.Schema.Fields {
				if f.Name == field && f.Offset+f.Length <= len(rec.Data) {
					fieldVal := strings.TrimRight(string(rec.Data[f.Offset:f.Offset+f.Length]), " \x00")
					match = matchFilter(fieldVal, op, value)
					break
				}
			}
			if !match {
				continue
			}
		}

		rd := ResultRecord{Index: i}
		if ot.Schema != nil {
			rd.Fields = make(map[string]string)
			for _, f := range ot.Schema.Fields {
				if f.Offset+f.Length <= len(rec.Data) {
					rd.Fields[f.Name] = strings.TrimRight(string(rec.Data[f.Offset:f.Offset+f.Length]), " \x00")
				}
			}
		} else {
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

		results = append(results, rd)
	}

	writeJSON(w, map[string]interface{}{
		"records": results,
		"count":   len(results),
		"total":   len(fi.Records),
	})
}

func matchFilter(fieldVal, op, value string) bool {
	fieldLower := strings.ToLower(strings.TrimSpace(fieldVal))
	valueLower := strings.ToLower(value)

	switch op {
	case "=", "eq":
		return fieldLower == valueLower
	case "!=", "ne":
		return fieldLower != valueLower
	case "contains":
		return strings.Contains(fieldLower, valueLower)
	case "starts_with":
		return strings.HasPrefix(fieldLower, valueLower)
	case "ends_with":
		return strings.HasSuffix(fieldLower, valueLower)
	case ">", "gt":
		return fieldLower > valueLower
	case "<", "lt":
		return fieldLower < valueLower
	case ">=", "gte":
		return fieldLower >= valueLower
	case "<=", "lte":
		return fieldLower <= valueLower
	default:
		return fmt.Sprintf("%v", fieldLower) == fmt.Sprintf("%v", valueLower)
	}
}
