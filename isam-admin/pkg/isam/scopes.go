package isam

import (
	"fmt"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// scopes.go — Reusable query scopes, Accessors/Mutators, and Raw queries
//
// Scopes (reusable filters registered on the table):
//
//	isam.Clients.Scope("active", func(q *isam.QueryBuilder) {
//	    q.Where("activa", "=", "S")
//	})
//
//	results, _ := isam.Clients.Query().WithScope("active").Get()
//
// Accessors (transform values on read):
//
//	isam.Clients.Accessor("nombre", func(raw string) string {
//	    return strings.Title(strings.ToLower(strings.TrimSpace(raw)))
//	})
//
//	client.GetAccessed("nombre") // returns formatted name
//
// Mutators (transform values on write):
//
//	isam.Clients.Mutator("nombre", func(input string) string {
//	    return strings.ToUpper(input)
//	})
//
//	client.SetMutated("nombre", "test") // stores "TEST"
//
// Raw bytes query (ultra-fast, no field decoding):
//
//	raw, _ := isam.Clients.RawAll()
//	for _, data := range raw {
//	    // data is []byte of the full record
//	}
//
// ---------------------------------------------------------------------------

// ScopeFn is a function that modifies a QueryBuilder to apply reusable filters.
type ScopeFn func(q *QueryBuilder)

// AccessorFn transforms a raw field value when reading.
type AccessorFn func(raw string) string

// MutatorFn transforms an input value before writing to the field.
type MutatorFn func(input string) string

var (
	scopeRegistry    = map[string]map[string]ScopeFn{}    // path → name → fn
	accessorRegistry = map[string]map[string]AccessorFn{} // path → field → fn
	mutatorRegistry  = map[string]map[string]MutatorFn{}  // path → field → fn
	scopeMu          sync.RWMutex
)

// ---------------------------------------------------------------------------
// Scopes
// ---------------------------------------------------------------------------

// Scope registers a reusable named scope on this table.
func (t *Table) Scope(name string, fn ScopeFn) {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	if scopeRegistry[t.Path] == nil {
		scopeRegistry[t.Path] = make(map[string]ScopeFn)
	}
	scopeRegistry[t.Path][name] = fn
}

// getScope returns a registered scope by name.
func getScope(path, name string) (ScopeFn, bool) {
	scopeMu.RLock()
	defer scopeMu.RUnlock()
	if m, ok := scopeRegistry[path]; ok {
		fn, found := m[name]
		return fn, found
	}
	return nil, false
}

// ClearScopes removes all scopes for this table.
func (t *Table) ClearScopes() {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	delete(scopeRegistry, t.Path)
}

// ---------------------------------------------------------------------------
// Accessors (transform on read)
// ---------------------------------------------------------------------------

// Accessor registers a function that transforms a field value when reading.
// Use GetAccessed() instead of Get() to apply the accessor.
func (t *Table) Accessor(fieldName string, fn AccessorFn) {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	if accessorRegistry[t.Path] == nil {
		accessorRegistry[t.Path] = make(map[string]AccessorFn)
	}
	accessorRegistry[t.Path][fieldName] = fn
}

// GetAccessed reads a field value and applies any registered accessor.
// If no accessor is registered, behaves like Get().
func (r *Row) GetAccessed(name string) string {
	raw := r.Get(name)

	scopeMu.RLock()
	defer scopeMu.RUnlock()

	if m, ok := accessorRegistry[r.table.Path]; ok {
		if fn, found := m[name]; found {
			return fn(raw)
		}
	}
	return raw
}

// ClearAccessors removes all accessors for this table.
func (t *Table) ClearAccessors() {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	delete(accessorRegistry, t.Path)
}

// ---------------------------------------------------------------------------
// Mutators (transform on write)
// ---------------------------------------------------------------------------

// Mutator registers a function that transforms a value before writing.
// Use SetMutated() instead of Set() to apply the mutator.
func (t *Table) Mutator(fieldName string, fn MutatorFn) {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	if mutatorRegistry[t.Path] == nil {
		mutatorRegistry[t.Path] = make(map[string]MutatorFn)
	}
	mutatorRegistry[t.Path][fieldName] = fn
}

// SetMutated writes a value to a field after applying any registered mutator.
// If no mutator is registered, behaves like Set().
func (r *Row) SetMutated(name, value string) error {
	scopeMu.RLock()
	if m, ok := mutatorRegistry[r.table.Path]; ok {
		if fn, found := m[name]; found {
			value = fn(value)
		}
	}
	scopeMu.RUnlock()

	return r.Set(name, value)
}

// ClearMutators removes all mutators for this table.
func (t *Table) ClearMutators() {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	delete(mutatorRegistry, t.Path)
}

// ---------------------------------------------------------------------------
// Raw bytes query (ultra-fast, no Row wrapping)
// ---------------------------------------------------------------------------

// RawAll reads all records as raw byte slices without creating Row objects.
// This is the fastest way to scan an ISAM table when you only need raw bytes.
func (t *Table) RawAll() ([][]byte, error) {
	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("raw read %s: %w", t.Name, err)
	}

	result := make([][]byte, len(info.Records))
	for i, rec := range info.Records {
		result[i] = rec.Data
	}
	return result, nil
}

// RawFind searches raw records by primary key without creating Row objects.
func (t *Table) RawFind(keyValue string) ([]byte, error) {
	if t.keyField == nil {
		return nil, fmt.Errorf("table %s has no primary key", t.Name)
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("raw read %s: %w", t.Name, err)
	}

	for _, rec := range info.Records {
		k := ExtractField(rec.Data, t.keyField.Offset, t.keyField.Length)
		if k == keyValue {
			return rec.Data, nil
		}
	}
	return nil, fmt.Errorf("record with key %q not found", keyValue)
}

// RawWhere scans raw records and returns those matching a predicate on raw bytes.
// The predicate receives the full raw record bytes.
func (t *Table) RawWhere(predicate func([]byte) bool) ([][]byte, error) {
	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("raw read %s: %w", t.Name, err)
	}

	var result [][]byte
	for _, rec := range info.Records {
		if predicate(rec.Data) {
			result = append(result, rec.Data)
		}
	}
	return result, nil
}

// RawExtract reads all records and extracts a single field as raw bytes (no string conversion).
func (t *Table) RawExtract(fieldName string) ([][]byte, error) {
	f := t.Field(fieldName)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", fieldName, t.Name)
	}

	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, fmt.Errorf("raw read %s: %w", t.Name, err)
	}

	result := make([][]byte, len(info.Records))
	for i, rec := range info.Records {
		end := f.Offset + f.Length
		if end > len(rec.Data) {
			end = len(rec.Data)
		}
		if f.Offset < len(rec.Data) {
			result[i] = rec.Data[f.Offset:end]
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Built-in accessor helpers
// ---------------------------------------------------------------------------

// TrimUpperAccessor returns an accessor that trims and uppercases the value.
func TrimUpperAccessor() AccessorFn {
	return func(raw string) string {
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

// TrimAccessor returns an accessor that just trims whitespace.
func TrimAccessor() AccessorFn {
	return func(raw string) string {
		return strings.TrimSpace(raw)
	}
}

// UpperMutator returns a mutator that uppercases the value before writing.
func UpperMutator() MutatorFn {
	return func(input string) string {
		return strings.ToUpper(input)
	}
}

// PadLeftMutator returns a mutator that left-pads with a character to a given length.
func PadLeftMutator(length int, pad byte) MutatorFn {
	return func(input string) string {
		for len(input) < length {
			input = string(pad) + input
		}
		return input
	}
}
