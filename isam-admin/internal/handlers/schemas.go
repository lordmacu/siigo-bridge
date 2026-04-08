package handlers

import (
	"encoding/json"
	"net/http"

	"isam-admin/internal/schemas"
	"isam-admin/pkg/isam"
)

// ---------------------------------------------------------------------------
// schemas.go — Schema management handlers (save, load, list, create file)
// ---------------------------------------------------------------------------

// SchemaHandlers provides HTTP handlers for schema operations
type SchemaHandlers struct {
	manager *schemas.Manager
}

// NewSchemaHandlers creates handlers for schema operations
func NewSchemaHandlers(manager *schemas.Manager) *SchemaHandlers {
	return &SchemaHandlers{manager: manager}
}

// ListHandler returns all saved schemas
func (sh *SchemaHandlers) ListHandler(w http.ResponseWriter, r *http.Request) {
	list, err := sh.manager.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error listing schemas: "+err.Error())
		return
	}
	if list == nil {
		list = []schemas.SavedSchema{}
	}
	writeJSON(w, list)
}

// GetHandler returns a single schema by name
func (sh *SchemaHandlers) GetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	schema, err := sh.manager.Load(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Schema not found: "+err.Error())
		return
	}

	writeJSON(w, schema)
}

// SaveHandler saves a new or updated schema
func (sh *SchemaHandlers) SaveHandler(w http.ResponseWriter, r *http.Request) {
	var schema schemas.SavedSchema
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid schema: "+err.Error())
		return
	}

	if schema.Name == "" {
		writeError(w, http.StatusBadRequest, "Schema name is required")
		return
	}
	if schema.RecordSize <= 0 {
		writeError(w, http.StatusBadRequest, "Record size must be positive")
		return
	}
	if len(schema.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "At least one field is required")
		return
	}

	// Validate key field exists
	hasKey := false
	for _, f := range schema.Fields {
		if f.IsKey {
			hasKey = true
			schema.KeyOffset = f.Offset
			schema.KeyLength = f.Length
			break
		}
	}
	if !hasKey {
		writeError(w, http.StatusBadRequest, "A key field is required")
		return
	}

	if err := sh.manager.Save(&schema); err != nil {
		writeError(w, http.StatusInternalServerError, "Save error: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "saved", "name": schema.Name})
}

// DeleteHandler deletes a schema
func (sh *SchemaHandlers) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	if err := sh.manager.Delete(name); err != nil {
		writeError(w, http.StatusInternalServerError, "Delete error: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "deleted", "name": name})
}

// CreateFileHandler creates a new ISAM file from a saved schema
func (sh *SchemaHandlers) CreateFileHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SchemaName string `json:"schema_name"`
		FilePath   string `json:"file_path"`
		Force      bool   `json:"force"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.SchemaName == "" || req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "schema_name and file_path are required")
		return
	}

	// Load schema
	saved, err := sh.manager.Load(req.SchemaName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Schema not found: "+err.Error())
		return
	}

	// Convert to isam.Schema
	isamSchema := isam.NewSchema(saved.RecordSize)
	for _, f := range saved.Fields {
		switch {
		case f.IsKey:
			isamSchema.KeyField(f.Name, f.Offset, f.Length)
		case f.Type == "int":
			isamSchema.IntField(f.Name, f.Offset, f.Length)
		case f.Type == "date":
			isamSchema.DateField(f.Name, f.Offset, f.Length)
		case f.Type == "bcd":
			isamSchema.BCDField(f.Name, f.Offset, f.Length, f.Decimals)
		default:
			isamSchema.StringField(f.Name, f.Offset, f.Length)
		}
	}

	// Create the file
	if req.Force {
		err = isam.CreateFileForce(req.FilePath, isamSchema)
	} else {
		err = isam.CreateFile(req.FilePath, isamSchema)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "Create file error: "+err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":      "created",
		"file_path":   req.FilePath,
		"record_size": saved.RecordSize,
		"fields":      len(saved.Fields),
	})
}
