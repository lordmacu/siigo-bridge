package schemas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// manager.go — Schema persistence manager
//
// Saves and loads user-defined ISAM schemas as JSON files in the data/schemas
// directory. Each schema describes the structure of an ISAM file (record size,
// fields, key position) so it can be reused to create new files or import data.
// ---------------------------------------------------------------------------

// FieldSchema describes a single field in a saved schema
type FieldSchema struct {
	Name     string `json:"name"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Type     string `json:"type"` // "string", "int", "date", "bcd", "float"
	Decimals int    `json:"decimals,omitempty"`
	IsKey    bool   `json:"is_key,omitempty"`
}

// SavedSchema is a complete schema definition persisted to disk
type SavedSchema struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	RecordSize  int           `json:"record_size"`
	KeyOffset   int           `json:"key_offset"`
	KeyLength   int           `json:"key_length"`
	Fields      []FieldSchema `json:"fields"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
	SourceFile  string        `json:"source_file,omitempty"` // original file this was detected from
}

// Manager handles schema persistence
type Manager struct {
	dir string
	mu  sync.RWMutex
}

// NewManager creates a schema manager for the given directory
func NewManager(dir string) *Manager {
	os.MkdirAll(dir, 0755)
	return &Manager{dir: dir}
}

// Save writes a schema to disk as JSON
func (m *Manager) Save(schema *SavedSchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	if schema.CreatedAt == "" {
		schema.CreatedAt = now
	}
	schema.UpdatedAt = now

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	filename := sanitizeName(schema.Name) + ".json"
	path := filepath.Join(m.dir, filename)
	return os.WriteFile(path, data, 0644)
}

// Load reads a schema from disk by name
func (m *Manager) Load(name string) (*SavedSchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filename := sanitizeName(name) + ".json"
	path := filepath.Join(m.dir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema %q: %w", name, err)
	}

	var schema SavedSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse schema %q: %w", name, err)
	}

	return &schema, nil
}

// List returns all saved schema names
func (m *Manager) List() ([]SavedSchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}

	var schemas []SavedSchema
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var s SavedSchema
		if json.Unmarshal(data, &s) == nil {
			schemas = append(schemas, s)
		}
	}

	return schemas, nil
}

// Delete removes a schema by name
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filename := sanitizeName(name) + ".json"
	path := filepath.Join(m.dir, filename)
	return os.Remove(path)
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	// Remove unsafe path chars
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, name)
	if safe == "" {
		safe = "unnamed"
	}
	return safe
}
