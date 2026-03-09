package isam

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// softdelete.go — Soft delete support for ISAM tables
//
// Instead of physically deleting records from ISAM files, marks them as
// "soft deleted" in a sidecar file ({path}.softdel). Records can be
// restored later.
//
// Usage:
//
//	isam.Clients.EnableSoftDelete()
//
//	rec, _ := isam.Clients.Find("00000000002001")
//	rec.SoftDelete()  // marks as deleted, doesn't touch ISAM
//
//	// All/Query automatically excludes soft-deleted records
//	all, _ := isam.Clients.All()  // won't include soft-deleted
//
//	// Include soft-deleted records
//	all, _ := isam.Clients.AllWithTrashed()
//
//	// Restore
//	rec.Restore()
//
//	// Get only trashed
//	trashed, _ := isam.Clients.OnlyTrashed()
//
// ---------------------------------------------------------------------------

// SoftDeleteEnabled tracks which tables have soft delete enabled
var (
	softDeleteEnabled = map[string]bool{}
	softDeletedKeys   = map[string]map[string]bool{} // path → set of deleted keys
	softDeleteMu      sync.RWMutex
)

// EnableSoftDelete activates soft delete for this table.
// Loads existing soft-deleted keys from sidecar file if present.
func (t *Table) EnableSoftDelete() {
	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()

	softDeleteEnabled[t.Path] = true
	if _, ok := softDeletedKeys[t.Path]; !ok {
		softDeletedKeys[t.Path] = make(map[string]bool)
		loadSoftDeleted(t.Path)
	}
}

// DisableSoftDelete deactivates soft delete for this table.
func (t *Table) DisableSoftDelete() {
	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()
	delete(softDeleteEnabled, t.Path)
}

// IsSoftDeleteEnabled returns true if soft delete is active for this table.
func (t *Table) IsSoftDeleteEnabled() bool {
	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()
	return softDeleteEnabled[t.Path]
}

// SoftDelete marks a record as deleted without touching the ISAM file.
func (r *Row) SoftDelete() error {
	if r.table.keyField == nil {
		return fmt.Errorf("table %s has no primary key for soft delete", r.table.Name)
	}

	key := strings.TrimSpace(r.Get(r.table.keyField.Name))
	if key == "" {
		return fmt.Errorf("cannot soft delete record with empty key")
	}

	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()

	if softDeletedKeys[r.table.Path] == nil {
		softDeletedKeys[r.table.Path] = make(map[string]bool)
	}
	softDeletedKeys[r.table.Path][key] = true
	return saveSoftDeleted(r.table.Path)
}

// Restore un-marks a soft-deleted record.
func (r *Row) Restore() error {
	if r.table.keyField == nil {
		return fmt.Errorf("table %s has no primary key", r.table.Name)
	}

	key := strings.TrimSpace(r.Get(r.table.keyField.Name))

	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()

	if m, ok := softDeletedKeys[r.table.Path]; ok {
		delete(m, key)
		return saveSoftDeleted(r.table.Path)
	}
	return nil
}

// IsSoftDeleted returns true if this record is soft-deleted.
func (r *Row) IsSoftDeleted() bool {
	if r.table.keyField == nil {
		return false
	}
	key := strings.TrimSpace(r.Get(r.table.keyField.Name))

	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()

	if m, ok := softDeletedKeys[r.table.Path]; ok {
		return m[key]
	}
	return false
}

// AllWithTrashed returns all records including soft-deleted ones.
func (t *Table) AllWithTrashed() ([]*Row, error) {
	// Temporarily bypass soft delete filter
	return t.readAllFromDisk()
}

// OnlyTrashed returns only soft-deleted records.
func (t *Table) OnlyTrashed() ([]*Row, error) {
	all, err := t.readAllFromDisk()
	if err != nil {
		return nil, err
	}

	softDeleteMu.RLock()
	deleted := softDeletedKeys[t.Path]
	softDeleteMu.RUnlock()

	if len(deleted) == 0 {
		return nil, nil
	}

	var trashed []*Row
	for _, r := range all {
		if t.keyField != nil {
			key := strings.TrimSpace(ExtractField(r.data, t.keyField.Offset, t.keyField.Length))
			if deleted[key] {
				trashed = append(trashed, r)
			}
		}
	}
	return trashed, nil
}

// TrashedCount returns the number of soft-deleted records.
func (t *Table) TrashedCount() int {
	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()
	if m, ok := softDeletedKeys[t.Path]; ok {
		return len(m)
	}
	return 0
}

// RestoreAll restores all soft-deleted records for this table.
func (t *Table) RestoreAll() error {
	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()
	softDeletedKeys[t.Path] = make(map[string]bool)
	return saveSoftDeleted(t.Path)
}

// isSoftDeleted checks if a key is soft-deleted (called by All/Query filtering).
func isSoftDeleted(path string, key string) bool {
	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()

	if !softDeleteEnabled[path] {
		return false
	}
	if m, ok := softDeletedKeys[path]; ok {
		return m[strings.TrimSpace(key)]
	}
	return false
}

// ---------------------------------------------------------------------------
// Sidecar file persistence (.softdel)
// ---------------------------------------------------------------------------

func softDeletePath(isamPath string) string {
	return isamPath + ".softdel"
}

func loadSoftDeleted(path string) {
	data, err := os.ReadFile(softDeletePath(path))
	if err != nil {
		return // file doesn't exist yet
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return
	}
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	softDeletedKeys[path] = m
}

func saveSoftDeleted(path string) error {
	m := softDeletedKeys[path]
	if len(m) == 0 {
		os.Remove(softDeletePath(path))
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return os.WriteFile(softDeletePath(path), data, 0644)
}
