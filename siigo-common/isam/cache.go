package isam

import (
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// cache.go — In-memory cache for ISAM table reads
//
// Avoids re-reading ISAM files on every query. Configurable TTL per table.
//
// Usage:
//
//	isam.Clients.EnableCache(30 * time.Second)
//	all, _ := isam.Clients.All()  // reads ISAM
//	all, _ = isam.Clients.All()   // returns cached (within 30s)
//	isam.Clients.ClearCache()     // force next read from disk
//
// ---------------------------------------------------------------------------

type tableCache struct {
	mu      sync.RWMutex
	rows    []*Row
	loadedAt time.Time
	ttl     time.Duration
}

// caches stores per-table caches keyed by file path
var (
	caches   = map[string]*tableCache{}
	cachesMu sync.Mutex
)

// EnableCache activates read caching for this table with the given TTL.
func (t *Table) EnableCache(ttl time.Duration) {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	caches[t.Path] = &tableCache{ttl: ttl}
}

// DisableCache removes caching for this table.
func (t *Table) DisableCache() {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	delete(caches, t.Path)
}

// ClearCache forces the next read to go to disk.
func (t *Table) ClearCache() {
	cachesMu.Lock()
	c := caches[t.Path]
	cachesMu.Unlock()
	if c != nil {
		c.mu.Lock()
		c.rows = nil
		c.loadedAt = time.Time{}
		c.mu.Unlock()
	}
}

// IsCached returns true if this table has a valid cache.
func (t *Table) IsCached() bool {
	c := getCache(t.Path)
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rows != nil && time.Since(c.loadedAt) < c.ttl
}

// cachedAll returns cached rows if available, otherwise reads from disk and caches.
// Used internally by All() when cache is enabled.
func (t *Table) cachedAll() ([]*Row, error) {
	c := getCache(t.Path)
	if c == nil {
		return nil, nil // no cache configured
	}

	// Try read from cache
	c.mu.RLock()
	if c.rows != nil && time.Since(c.loadedAt) < c.ttl {
		rows := make([]*Row, len(c.rows))
		copy(rows, c.rows)
		c.mu.RUnlock()
		return rows, nil
	}
	c.mu.RUnlock()

	// Cache miss — read from disk
	rows, err := t.readAllFromDisk()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.mu.Lock()
	c.rows = rows
	c.loadedAt = time.Now()
	c.mu.Unlock()

	// Return a copy
	result := make([]*Row, len(rows))
	copy(result, rows)
	return result, nil
}

// readAllFromDisk reads all records bypassing cache (used by cache layer itself).
func (t *Table) readAllFromDisk() ([]*Row, error) {
	info, _, err := ReadFileV2(t.Path)
	if err != nil {
		return nil, err
	}

	records := make([]*Row, 0, len(info.Records))
	for i, rec := range info.Records {
		r := &Row{
			table: t,
			data:  append([]byte{}, rec.Data...),
			index: i,
		}
		records = append(records, r)
	}
	return records, nil
}

func getCache(path string) *tableCache {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	return caches[path]
}

// ClearAllCaches clears all table caches.
func ClearAllCaches() {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	for _, c := range caches {
		c.mu.Lock()
		c.rows = nil
		c.loadedAt = time.Time{}
		c.mu.Unlock()
	}
}

// invalidateCache clears cache for a table after a write operation.
func invalidateCache(path string) {
	c := getCache(path)
	if c != nil {
		c.mu.Lock()
		c.rows = nil
		c.loadedAt = time.Time{}
		c.mu.Unlock()
	}
}
