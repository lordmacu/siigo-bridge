package isam

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// orm_features.go — Dirty Tracking, Timestamps, Mass Assignment, JSON
//
// Dirty Tracking:
//
//	rec, _ := isam.Clients.Find("00000000002001")
//	rec.Set("nombre", "NEW NAME")
//	rec.IsDirty()           // true
//	rec.IsDirtyField("nombre") // true
//	rec.GetDirty()          // ["nombre"]
//	rec.Changes()           // map[nombre:{old:"OLD", new:"NEW NAME"}]
//	rec.Original("nombre")  // "OLD NAME       ..."
//
// Timestamps:
//
//	isam.Clients.EnableTimestamps("fecha_creacion", "fecha_modificacion")
//	rec := isam.Clients.New()
//	rec.Set("nombre", "TEST")
//	rec.Save() // auto-sets fecha_creacion + fecha_modificacion
//
//	rec.Set("nombre", "UPDATED")
//	rec.Save() // only updates fecha_modificacion
//
// Mass Assignment:
//
//	isam.Clients.Fillable("nombre", "tipo_doc", "email")
//	rec.Fill(map[string]string{"nombre": "X", "codigo": "HACK"})
//	// codigo is NOT filled (not in fillable list)
//
//	isam.Clients.Guarded("codigo", "empresa")
//	rec.Fill(map[string]string{"nombre": "X", "codigo": "HACK"})
//	// codigo is NOT filled (guarded)
//
// JSON:
//
//	jsonBytes, _ := rec.ToJSON()
//	jsonStr := rec.ToJSONString()
//	jsonBytes, _ = rec.ToJSONSelected("codigo", "nombre")
//
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Dirty Tracking
// ---------------------------------------------------------------------------

// TrackChanges enables dirty tracking by snapshotting current field values.
// Called automatically when a Row is loaded from disk. For new records,
// all fields are considered dirty.
func (r *Row) TrackChanges() {
	if r.original == nil {
		r.original = make([]byte, len(r.data))
		copy(r.original, r.data)
	}
}

// IsDirty returns true if any field has been modified since load.
func (r *Row) IsDirty() bool {
	if r.isNew {
		return true
	}
	if r.original == nil {
		return false
	}
	for i := range r.data {
		if i < len(r.original) && r.data[i] != r.original[i] {
			return true
		}
	}
	return false
}

// IsDirtyField returns true if a specific field has been modified.
func (r *Row) IsDirtyField(name string) bool {
	if r.isNew {
		return true
	}
	f := r.table.Field(name)
	if f == nil || r.original == nil {
		return false
	}
	end := f.Offset + f.Length
	if end > len(r.data) || end > len(r.original) {
		return false
	}
	for i := f.Offset; i < end; i++ {
		if r.data[i] != r.original[i] {
			return true
		}
	}
	return false
}

// FieldChange represents a before/after change on a field.
type FieldChange struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// GetDirty returns the names of all modified fields.
func (r *Row) GetDirty() []string {
	var dirty []string
	for _, f := range r.table.Fields {
		if r.IsDirtyField(f.Name) {
			dirty = append(dirty, f.Name)
		}
	}
	return dirty
}

// Changes returns a map of field name → {old, new} for all modified fields.
func (r *Row) Changes() map[string]FieldChange {
	changes := make(map[string]FieldChange)
	if r.original == nil {
		return changes
	}
	for _, f := range r.table.Fields {
		if r.IsDirtyField(f.Name) {
			oldVal := ExtractField(r.original, f.Offset, f.Length)
			newVal := ExtractField(r.data, f.Offset, f.Length)
			changes[f.Name] = FieldChange{Old: oldVal, New: newVal}
		}
	}
	return changes
}

// Original returns the original value of a field before any changes.
func (r *Row) Original(name string) string {
	f := r.table.Field(name)
	if f == nil || r.original == nil {
		return r.Get(name)
	}
	return ExtractField(r.original, f.Offset, f.Length)
}

// ResetChanges discards tracked changes by re-snapshotting current state.
func (r *Row) ResetChanges() {
	if r.original == nil {
		r.original = make([]byte, len(r.data))
	}
	copy(r.original, r.data)
}

// Revert undoes all changes and restores original data.
func (r *Row) Revert() {
	if r.original != nil {
		copy(r.data, r.original)
	}
}

// ---------------------------------------------------------------------------
// Timestamps
// ---------------------------------------------------------------------------

var (
	timestampFields   = map[string][2]string{} // path → [createdField, updatedField]
	timestampFieldsMu sync.RWMutex
)

// EnableTimestamps activates auto-managed timestamp fields.
// Both fields must be Date type (8-char YYYYMMDD) already defined in the schema.
// On Save(): new records set both fields, existing records only update updatedField.
func (t *Table) EnableTimestamps(createdField, updatedField string) {
	timestampFieldsMu.Lock()
	defer timestampFieldsMu.Unlock()
	timestampFields[t.Path] = [2]string{createdField, updatedField}
}

// DisableTimestamps removes auto-managed timestamps for this table.
func (t *Table) DisableTimestamps() {
	timestampFieldsMu.Lock()
	defer timestampFieldsMu.Unlock()
	delete(timestampFields, t.Path)
}

// HasTimestamps returns true if timestamps are enabled for this table.
func (t *Table) HasTimestamps() bool {
	timestampFieldsMu.RLock()
	defer timestampFieldsMu.RUnlock()
	_, ok := timestampFields[t.Path]
	return ok
}

// applyTimestamps sets timestamp fields before save. Called from Save().
func applyTimestamps(r *Row) {
	timestampFieldsMu.RLock()
	fields, ok := timestampFields[r.table.Path]
	timestampFieldsMu.RUnlock()
	if !ok {
		return
	}

	now := time.Now().Format("20060102")

	if r.isNew && fields[0] != "" {
		r.Set(fields[0], now)
	}
	if fields[1] != "" {
		r.Set(fields[1], now)
	}
}

// ---------------------------------------------------------------------------
// Mass Assignment Protection
// ---------------------------------------------------------------------------

var (
	fillableFields = map[string]map[string]bool{} // path → set of allowed fields
	guardedFields  = map[string]map[string]bool{}  // path → set of blocked fields
	massAssignMu   sync.RWMutex
)

// Fillable sets the list of fields that CAN be mass-assigned via Fill().
// If set, only these fields will be accepted. Mutually exclusive with Guarded().
func (t *Table) Fillable(fields ...string) {
	massAssignMu.Lock()
	defer massAssignMu.Unlock()
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		m[f] = true
	}
	fillableFields[t.Path] = m
	delete(guardedFields, t.Path) // clear guarded if fillable is set
}

// Guarded sets the list of fields that CANNOT be mass-assigned via Fill().
// All other fields are allowed. Mutually exclusive with Fillable().
func (t *Table) Guarded(fields ...string) {
	massAssignMu.Lock()
	defer massAssignMu.Unlock()
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		m[f] = true
	}
	guardedFields[t.Path] = m
	delete(fillableFields, t.Path) // clear fillable if guarded is set
}

// ClearMassAssignment removes both fillable and guarded settings.
func (t *Table) ClearMassAssignment() {
	massAssignMu.Lock()
	defer massAssignMu.Unlock()
	delete(fillableFields, t.Path)
	delete(guardedFields, t.Path)
}

// isFillable checks if a field can be mass-assigned.
func isFillable(path, field string) bool {
	massAssignMu.RLock()
	defer massAssignMu.RUnlock()

	// If fillable list exists, field must be in it
	if fillable, ok := fillableFields[path]; ok {
		return fillable[field]
	}

	// If guarded list exists, field must NOT be in it
	if guarded, ok := guardedFields[path]; ok {
		return !guarded[field]
	}

	// No restrictions — all fields allowed
	return true
}

// Fill mass-assigns multiple field values, respecting fillable/guarded rules.
// Returns the list of fields that were actually set and any that were rejected.
//
// Example:
//
//	set, rejected := rec.Fill(map[string]string{
//	    "nombre": "TEST",
//	    "codigo": "HACK", // rejected if guarded
//	})
func (r *Row) Fill(values map[string]string) (set []string, rejected []string) {
	for field, value := range values {
		if !isFillable(r.table.Path, field) {
			rejected = append(rejected, field)
			continue
		}
		if err := r.Set(field, value); err == nil {
			set = append(set, field)
		} else {
			rejected = append(rejected, field)
		}
	}
	return
}

// FillMutated mass-assigns values applying mutators, respecting fillable/guarded.
func (r *Row) FillMutated(values map[string]string) (set []string, rejected []string) {
	for field, value := range values {
		if !isFillable(r.table.Path, field) {
			rejected = append(rejected, field)
			continue
		}
		if err := r.SetMutated(field, value); err == nil {
			set = append(set, field)
		} else {
			rejected = append(rejected, field)
		}
	}
	return
}

// ---------------------------------------------------------------------------
// JSON Serialization
// ---------------------------------------------------------------------------

// ToJSON serializes the record as JSON bytes (all fields).
func (r *Row) ToJSON() ([]byte, error) {
	return json.Marshal(r.ToMap())
}

// ToJSONString serializes the record as a JSON string (all fields).
func (r *Row) ToJSONString() string {
	b, err := r.ToJSON()
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ToJSONSelected serializes only the specified fields as JSON bytes.
func (r *Row) ToJSONSelected(fields ...string) ([]byte, error) {
	return json.Marshal(r.ToSelectedMap(fields))
}

// ToJSONPretty serializes the record as indented JSON (all fields).
func (r *Row) ToJSONPretty() ([]byte, error) {
	return json.MarshalIndent(r.ToMap(), "", "  ")
}

// ToJSONPrettyString serializes the record as an indented JSON string.
func (r *Row) ToJSONPrettyString() string {
	b, err := r.ToJSONPretty()
	if err != nil {
		return "{}"
	}
	return string(b)
}

// RowsToJSON serializes a slice of rows as a JSON array.
func RowsToJSON(rows []*Row) ([]byte, error) {
	maps := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		maps[i] = r.ToMap()
	}
	return json.Marshal(maps)
}

// RowsToJSONSelected serializes a slice of rows with only specified fields.
func RowsToJSONSelected(rows []*Row, fields ...string) ([]byte, error) {
	maps := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		maps[i] = r.ToSelectedMap(fields)
	}
	return json.Marshal(maps)
}

// FromJSON populates a Row from a JSON object, respecting fillable/guarded.
// Only string fields are supported (JSON values are converted to strings).
func (r *Row) FromJSON(data []byte) (set []string, rejected []string, err error) {
	var values map[string]interface{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}

	strValues := make(map[string]string, len(values))
	for k, v := range values {
		// Skip metadata fields
		if strings.HasPrefix(k, "_") {
			continue
		}
		strValues[k] = fmt.Sprintf("%v", v)
	}

	s, r2 := r.Fill(strValues)
	return s, r2, nil
}

// ---------------------------------------------------------------------------
// Increment / Decrement
// ---------------------------------------------------------------------------

// Increment adds amount to a numeric field (Int or Float). Does NOT auto-save.
func (r *Row) Increment(name string, amount float64) error {
	f := r.table.Field(name)
	if f == nil {
		return fmt.Errorf("field %q not found in table %s", name, r.table.Name)
	}
	current := r.GetFloat(name)
	newVal := current + amount
	if f.Type == FieldInt {
		return r.SetInt(name, int(newVal))
	}
	return r.Set(name, fmt.Sprintf("%.*f", f.Decimals, newVal))
}

// Decrement subtracts amount from a numeric field. Does NOT auto-save.
func (r *Row) Decrement(name string, amount float64) error {
	return r.Increment(name, -amount)
}

// ---------------------------------------------------------------------------
// Composite Keys
// ---------------------------------------------------------------------------

// CompositeKey adds a composite primary key (multiple fields).
// The composite key is the concatenation of all field values for lookup.
func (t *Table) CompositeKey(fields ...struct{ Name string; Offset, Length int }) *Table {
	t.compositeKeys = make([]FieldDef, len(fields))
	for i, f := range fields {
		t.compositeKeys[i] = FieldDef{Name: f.Name, Offset: f.Offset, Length: f.Length, Type: FieldString, IsKey: true}
	}
	return t
}

// FindComposite finds a record by composite key values.
// Pass values in the same order as CompositeKey was defined.
func (t *Table) FindComposite(keyValues ...string) (*Row, error) {
	if len(t.compositeKeys) == 0 {
		return nil, fmt.Errorf("table %s has no composite key defined", t.Name)
	}
	if len(keyValues) != len(t.compositeKeys) {
		return nil, fmt.Errorf("expected %d key values, got %d", len(t.compositeKeys), len(keyValues))
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}

	for i, rec := range info.Records {
		match := true
		for j, kf := range t.compositeKeys {
			v := ExtractField(rec.Data, kf.Offset, kf.Length)
			if strings.TrimSpace(v) != strings.TrimSpace(keyValues[j]) {
				match = false
				break
			}
		}
		if match {
			data := append([]byte{}, rec.Data...)
			return &Row{
				table:    t,
				data:     data,
				original: append([]byte{}, data...),
				index:    i,
			}, nil
		}
	}
	return nil, fmt.Errorf("record with composite key %v not found in %s", keyValues, t.Name)
}

// GetCompositeKey returns the composite key values for a row.
func (r *Row) GetCompositeKey() []string {
	if len(r.table.compositeKeys) == 0 {
		// Fall back to single key
		if r.table.keyField != nil {
			return []string{r.Get(r.table.keyField.Name)}
		}
		return nil
	}
	vals := make([]string, len(r.table.compositeKeys))
	for i, kf := range r.table.compositeKeys {
		vals[i] = strings.TrimSpace(ExtractField(r.data, kf.Offset, kf.Length))
	}
	return vals
}

// ---------------------------------------------------------------------------
// Query Debug / Explain
// ---------------------------------------------------------------------------

// Explain returns a human-readable description of the query that would be executed.
func (q *QueryBuilder) Explain() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("FROM %s\n", q.table.Name))

	if len(q.conditions) > 0 {
		b.WriteString("WHERE\n")
		for i, c := range q.conditions {
			if i > 0 {
				b.WriteString("  AND ")
			} else {
				b.WriteString("  ")
			}
			if c.operator == "in" {
				b.WriteString(fmt.Sprintf("%s IN [%s]\n", c.field, strings.Join(c.values, ", ")))
			} else {
				b.WriteString(fmt.Sprintf("%s %s %q\n", c.field, c.operator, c.value))
			}
		}
	}

	if q.orderField != "" {
		b.WriteString(fmt.Sprintf("ORDER BY %s %s", q.orderField, strings.ToUpper(q.orderDir)))
		for _, es := range q.extraSorts {
			b.WriteString(fmt.Sprintf(", %s %s", es.field, strings.ToUpper(es.dir)))
		}
		b.WriteString("\n")
	}

	if q.offsetN > 0 {
		b.WriteString(fmt.Sprintf("OFFSET %d\n", q.offsetN))
	}
	if q.limitN > 0 {
		b.WriteString(fmt.Sprintf("LIMIT %d\n", q.limitN))
	}
	if len(q.selectCols) > 0 {
		b.WriteString(fmt.Sprintf("SELECT %s\n", strings.Join(q.selectCols, ", ")))
	}

	return b.String()
}
