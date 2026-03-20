package isam

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// orm.go — ORM-like layer for ISAM CRUD operations
//
// Usage:
//
//	// Register a table once at init
//	clients := isam.NewTable("clients", `C:\DEMOS01\Z17`, 1438).
//	    Key("codigo", 4, 14).
//	    String("tipo", 0, 1).
//	    String("empresa", 1, 3).
//	    String("tipo_doc", 18, 2).
//	    String("nombre", 36, 40).
//	    Date("fecha", 28, 8)
//
//	// CRUD
//	all, _ := clients.All()               // []Record
//	rec, _ := clients.Find("00000000002001") // *Row
//	rec.Set("nombre", "NUEVO NOMBRE")
//	_ = rec.Save()                         // REWRITE
//	_ = rec.Delete()                       // DELETE
//	newRec := clients.New()
//	newRec.Set("codigo", "99999999999999")
//	newRec.Set("nombre", "TEST")
//	_ = newRec.Save()                      // INSERT
//
// ---------------------------------------------------------------------------

// FieldType defines the data type of a field
type FieldType int

const (
	FieldString FieldType = iota
	FieldInt
	FieldFloat
	FieldDate
	FieldBCD
)

// FieldDef defines a single field in a table schema
type FieldDef struct {
	Name     string
	Offset   int
	Length   int
	Type     FieldType
	Decimals int  // For BCD fields
	IsKey    bool // Primary key field
}

// Table represents an ISAM table with its schema
type Table struct {
	Name          string
	Path          string
	RecSize       int
	Fields        []FieldDef
	keyField      *FieldDef  // cached reference to primary key
	compositeKeys []FieldDef // composite key fields (optional)
	SafeMode      bool       // when true, writes check Siigo process + file locks (default: true)
	RecordFilter  func([]byte) bool // optional: only include records where this returns true
}

// Row represents a single record from an ISAM table (named Row to avoid
// conflict with the lower-level Record struct in reader.go)
type Row struct {
	table     *Table
	data      []byte // raw ISAM record bytes
	original  []byte // snapshot for dirty tracking (nil = not tracked)
	index     int    // record index in file (-1 = new/unsaved)
	isNew     bool
	isDeleted bool
}

// NewTable creates a new table definition
func NewTable(name, path string, recSize int) *Table {
	return &Table{
		Name:     name,
		Path:     path,
		RecSize:  recSize,
		SafeMode: true, // safe by default
	}
}

// Key adds the primary key field
func (t *Table) Key(name string, offset, length int) *Table {
	f := FieldDef{Name: name, Offset: offset, Length: length, Type: FieldString, IsKey: true}
	t.Fields = append(t.Fields, f)
	t.keyField = &t.Fields[len(t.Fields)-1]
	return t
}

// String adds a string field
func (t *Table) String(name string, offset, length int) *Table {
	t.Fields = append(t.Fields, FieldDef{Name: name, Offset: offset, Length: length, Type: FieldString})
	return t
}

// Int adds an integer field (ASCII digits)
func (t *Table) Int(name string, offset, length int) *Table {
	t.Fields = append(t.Fields, FieldDef{Name: name, Offset: offset, Length: length, Type: FieldInt})
	return t
}

// Float adds a float field (ASCII digits with decimals)
func (t *Table) Float(name string, offset, length int) *Table {
	t.Fields = append(t.Fields, FieldDef{Name: name, Offset: offset, Length: length, Type: FieldFloat})
	return t
}

// Date adds a date field (8-char YYYYMMDD)
func (t *Table) Date(name string, offset, length int) *Table {
	t.Fields = append(t.Fields, FieldDef{Name: name, Offset: offset, Length: length, Type: FieldDate})
	return t
}

// BCD adds a packed decimal field
func (t *Table) BCD(name string, offset, length, decimals int) *Table {
	t.Fields = append(t.Fields, FieldDef{Name: name, Offset: offset, Length: length, Type: FieldBCD, Decimals: decimals})
	return t
}

// Field returns a field definition by name, or nil
func (t *Table) Field(name string) *FieldDef {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// All reads all records from the ISAM file.
// If cache is enabled (via EnableCache), returns cached data when valid.
// If soft delete is enabled, excludes soft-deleted records.
func (t *Table) All() ([]*Row, error) {
	// Try cache first
	if cached, err := t.cachedAll(); cached != nil || err != nil {
		if t.IsSoftDeleteEnabled() {
			return t.filterSoftDeleted(cached), err
		}
		return cached, err
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}

	hasSoftDelete := t.IsSoftDeleteEnabled()
	records := make([]*Row, 0, len(info.Records))
	for i, rec := range info.Records {
		if t.RecordFilter != nil && !t.RecordFilter(rec.Data) {
			continue
		}
		if hasSoftDelete && t.keyField != nil {
			key := ExtractField(rec.Data, t.keyField.Offset, t.keyField.Length)
			if isSoftDeleted(t.Path, key) {
				continue
			}
		}
		data := append([]byte{}, rec.Data...)
		r := &Row{
			table:    t,
			data:     data,
			original: append([]byte{}, data...), // snapshot for dirty tracking
			index:    i,
		}
		records = append(records, r)
	}
	return records, nil
}

// filterSoftDeleted removes soft-deleted records from a slice.
func (t *Table) filterSoftDeleted(rows []*Row) []*Row {
	if t.keyField == nil || len(rows) == 0 {
		return rows
	}
	result := make([]*Row, 0, len(rows))
	for _, r := range rows {
		key := ExtractField(r.data, t.keyField.Offset, t.keyField.Length)
		if !isSoftDeleted(t.Path, key) {
			result = append(result, r)
		}
	}
	return result
}

// Find finds a record by primary key value
func (t *Table) Find(keyValue string) (*Row, error) {
	if t.keyField == nil {
		return nil, fmt.Errorf("table %s has no primary key defined", t.Name)
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}

	for i, rec := range info.Records {
		k := ExtractField(rec.Data, t.keyField.Offset, t.keyField.Length)
		if k == keyValue {
			data := append([]byte{}, rec.Data...)
			return &Row{
				table:    t,
				data:     data,
				original: append([]byte{}, data...),
				index:    i,
			}, nil
		}
	}
	return nil, fmt.Errorf("record with %s=%q not found in %s", t.keyField.Name, keyValue, t.Name)
}

// FindAll finds all records matching a field value
func (t *Table) FindAll(fieldName, value string) ([]*Row, error) {
	f := t.Field(fieldName)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", fieldName, t.Name)
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}

	var results []*Row
	for i, rec := range info.Records {
		v := ExtractField(rec.Data, f.Offset, f.Length)
		if v == value {
			results = append(results, &Row{
				table: t,
				data:  append([]byte{}, rec.Data...),
				index: i,
			})
		}
	}
	return results, nil
}

// Where finds records where a field matches a predicate
func (t *Table) Where(fieldName string, predicate func(string) bool) ([]*Row, error) {
	f := t.Field(fieldName)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", fieldName, t.Name)
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}

	var results []*Row
	for i, rec := range info.Records {
		v := ExtractField(rec.Data, f.Offset, f.Length)
		if predicate(v) {
			results = append(results, &Row{
				table: t,
				data:  append([]byte{}, rec.Data...),
				index: i,
			})
		}
	}
	return results, nil
}

// Count returns the number of records
func (t *Table) Count() (int, error) {
	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return 0, err
	}
	return len(info.Records), nil
}

// New creates a new empty record (not saved until Save() is called)
func (t *Table) New() *Row {
	return &Row{
		table: t,
		data:  make([]byte, t.RecSize),
		index: -1,
		isNew: true,
	}
}

// ---------------------------------------------------------------------------
// Record — field access
// ---------------------------------------------------------------------------

// Get reads a field value as string
func (r *Row) Get(name string) string {
	f := r.table.Field(name)
	if f == nil {
		return ""
	}
	return ExtractField(r.data, f.Offset, f.Length)
}

// GetInt reads a field as integer
func (r *Row) GetInt(name string) int {
	s := strings.TrimSpace(r.Get(name))
	v, _ := strconv.Atoi(s)
	return v
}

// GetFloat reads a field as float64 (for BCD fields, decodes packed decimal)
func (r *Row) GetFloat(name string) float64 {
	f := r.table.Field(name)
	if f == nil {
		return 0
	}
	if f.Type == FieldBCD {
		end := f.Offset + f.Length
		if end > len(r.data) {
			return 0
		}
		return decodePacked(r.data[f.Offset:end], f.Decimals)
	}
	s := strings.TrimSpace(r.Get(name))
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// Set writes a string value to a field (COBOL-style space-padded)
func (r *Row) Set(name, value string) error {
	f := r.table.Field(name)
	if f == nil {
		return fmt.Errorf("field %q not found in table %s", name, r.table.Name)
	}

	buf := make([]byte, f.Length)
	for i := range buf {
		buf[i] = ' ' // COBOL space fill
	}
	copy(buf, []byte(value))

	if f.Offset+f.Length > len(r.data) {
		return fmt.Errorf("field %q offset %d+%d exceeds record size %d", name, f.Offset, f.Length, len(r.data))
	}
	copy(r.data[f.Offset:f.Offset+f.Length], buf)
	return nil
}

// SetInt writes an integer value to a field (zero-padded)
func (r *Row) SetInt(name string, value int) error {
	f := r.table.Field(name)
	if f == nil {
		return fmt.Errorf("field %q not found in table %s", name, r.table.Name)
	}
	s := fmt.Sprintf("%0*d", f.Length, value)
	return r.Set(name, s)
}

// SetBytes writes raw bytes to a field
func (r *Row) SetBytes(name string, value []byte) error {
	f := r.table.Field(name)
	if f == nil {
		return fmt.Errorf("field %q not found in table %s", name, r.table.Name)
	}
	if f.Offset+f.Length > len(r.data) {
		return fmt.Errorf("field %q exceeds record size", name)
	}
	// Zero-fill then copy
	for i := 0; i < f.Length; i++ {
		r.data[f.Offset+i] = 0
	}
	n := f.Length
	if len(value) < n {
		n = len(value)
	}
	copy(r.data[f.Offset:], value[:n])
	return nil
}

// Data returns the raw record bytes
func (r *Row) Data() []byte {
	return r.data
}

// Hash returns SHA256 hash of the record (first 8 bytes hex, matches parser pattern)
func (r *Row) Hash() string {
	h := sha256.Sum256(r.data)
	return fmt.Sprintf("%x", h[:8])
}

// Index returns the record index in the file (-1 if new/unsaved)
func (r *Row) Index() int {
	return r.index
}

// IsNew returns true if this record hasn't been saved yet
func (r *Row) IsNew() bool {
	return r.isNew
}

// ToMap converts the record to a map of field name → string value
func (r *Row) ToMap() map[string]interface{} {
	m := make(map[string]interface{}, len(r.table.Fields))
	for _, f := range r.table.Fields {
		switch f.Type {
		case FieldBCD:
			m[f.Name] = r.GetFloat(f.Name)
		case FieldInt:
			m[f.Name] = r.GetInt(f.Name)
		default:
			m[f.Name] = r.Get(f.Name)
		}
	}
	m["_hash"] = r.Hash()
	m["_index"] = r.index
	return m
}

// ToSelectedMap converts the record to a map with only the specified fields.
func (r *Row) ToSelectedMap(fields []string) map[string]interface{} {
	m := make(map[string]interface{}, len(fields))
	for _, name := range fields {
		f := r.table.Field(name)
		if f == nil {
			continue
		}
		switch f.Type {
		case FieldBCD:
			m[f.Name] = r.GetFloat(f.Name)
		case FieldInt:
			m[f.Name] = r.GetInt(f.Name)
		default:
			m[f.Name] = r.Get(f.Name)
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Write operations
// ---------------------------------------------------------------------------

// Save persists the record to the ISAM file.
// Runs: validation → BeforeSave hooks → SafeMode check → write → AfterSave hooks.
func (r *Row) Save() (*WriteResult, error) {
	if r.isDeleted {
		return nil, fmt.Errorf("cannot save a deleted record")
	}
	if err := runValidation(r); err != nil {
		return nil, err
	}
	if err := runBeforeSave(r); err != nil {
		return nil, err
	}
	applyTimestamps(r)
	if r.table.SafeMode {
		if err := CheckWriteSafe(r.table.Path); err != nil {
			return nil, fmt.Errorf("safe mode: %w", err)
		}
	}

	var result *WriteResult
	var err error
	if r.isNew {
		result, err = r.insert()
	} else {
		result, err = r.rewrite()
	}
	if err != nil {
		return nil, err
	}
	runAfterSave(r)
	return result, nil
}

func (r *Row) rewrite() (*WriteResult, error) {
	t := r.table
	if t.keyField == nil {
		return nil, fmt.Errorf("table %s has no primary key", t.Name)
	}
	keyOffsets := [][2]int{{t.keyField.Offset, t.keyField.Length}}
	result, err := RewriteRecord(t.Path, r.index, r.data, keyOffsets)
	if err != nil {
		return nil, fmt.Errorf("rewrite %s record #%d: %w", t.Name, r.index, err)
	}
	invalidateCache(t.Path)
	return result, nil
}

func (r *Row) insert() (*WriteResult, error) {
	t := r.table
	if t.keyField == nil {
		return nil, fmt.Errorf("table %s has no primary key", t.Name)
	}
	result, err := InsertRecord(t.Path, r.data, t.keyField.Offset, t.keyField.Length)
	if err != nil {
		return nil, fmt.Errorf("insert into %s: %w", t.Name, err)
	}
	r.isNew = false
	r.index = result.RecordIndex
	invalidateCache(t.Path)
	return result, nil
}

// Delete marks the record as deleted in the ISAM file.
// Runs: BeforeDelete hooks → SafeMode check → delete → AfterDelete hooks.
func (r *Row) Delete() (*WriteResult, error) {
	if r.isNew {
		return nil, fmt.Errorf("cannot delete an unsaved record")
	}
	if r.isDeleted {
		return nil, fmt.Errorf("record already deleted")
	}
	if err := runBeforeDelete(r); err != nil {
		return nil, err
	}
	if r.table.SafeMode {
		if err := CheckWriteSafe(r.table.Path); err != nil {
			return nil, fmt.Errorf("safe mode: %w", err)
		}
	}

	result, err := DeleteRecord(r.table.Path, r.index)
	if err != nil {
		return nil, fmt.Errorf("delete %s record #%d: %w", r.table.Name, r.index, err)
	}
	r.isDeleted = true
	invalidateCache(r.table.Path)
	runAfterDelete(r)
	return result, nil
}

// ---------------------------------------------------------------------------
// Batch operations
// ---------------------------------------------------------------------------

// DeleteByKey finds and deletes a record by primary key.
// In SafeMode (default), checks that Siigo is not running and the file is not locked.
func (t *Table) DeleteByKey(keyValue string) (*WriteResult, error) {
	if t.keyField == nil {
		return nil, fmt.Errorf("table %s has no primary key", t.Name)
	}
	if t.SafeMode {
		if err := CheckWriteSafe(t.Path); err != nil {
			return nil, fmt.Errorf("safe mode: %w", err)
		}
	}
	return DeleteRecordByKey(t.Path, t.keyField.Offset, t.keyField.Length, keyValue)
}

// UpdateByKey finds a record by key, applies changes via a callback, and saves
func (t *Table) UpdateByKey(keyValue string, updater func(*Row)) (*WriteResult, error) {
	rec, err := t.Find(keyValue)
	if err != nil {
		return nil, err
	}
	updater(rec)
	return rec.Save()
}

// ReplaceByKey deletes the old record and inserts a new one (key change)
func (t *Table) ReplaceByKey(oldKey string, newRecord *Row) (*WriteResult, error) {
	_, err := t.DeleteByKey(oldKey)
	if err != nil {
		return nil, fmt.Errorf("delete old key %q: %w", oldKey, err)
	}
	return newRecord.Save()
}

// ---------------------------------------------------------------------------
// BCD helper (avoid importing parsers from isam package)
// ---------------------------------------------------------------------------

func decodePacked(data []byte, decimals int) float64 {
	if len(data) == 0 {
		return 0
	}
	var digits []byte
	for i := 0; i < len(data)-1; i++ {
		digits = append(digits, data[i]>>4, data[i]&0x0F)
	}
	lastByte := data[len(data)-1]
	digits = append(digits, lastByte>>4)
	sign := lastByte & 0x0F

	var result float64
	for _, d := range digits {
		result = result*10 + float64(d)
	}
	if decimals > 0 {
		divisor := 1.0
		for i := 0; i < decimals; i++ {
			divisor *= 10
		}
		result /= divisor
	}
	if sign == 0x0D {
		result = -result
	}
	return result
}
