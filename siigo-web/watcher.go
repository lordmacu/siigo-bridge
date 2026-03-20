package main

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fileWatcher watches a directory for ISAM file changes and triggers
// per-table detection instead of polling all tables every cycle.
type fileWatcher struct {
	watcher   *fsnotify.Watcher
	dir       string
	debounce  time.Duration
	fileToTables map[string][]string // ISAM base name (uppercase) → table names
	pending   map[string]time.Time   // file → last event time
	mu        sync.Mutex
	onChange  func(tables []string)  // callback with list of changed table names
	stopCh   chan struct{}
}

// newFileWatcher creates a watcher for the given data directory.
// fileToTables maps uppercase ISAM base filenames (e.g. "Z17", "Z042016") to sync table names.
// debounce specifies how long to wait after the last event before triggering (groups rapid writes).
func newFileWatcher(dir string, fileToTables map[string][]string, debounce time.Duration, onChange func([]string)) (*fileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &fileWatcher{
		watcher:      w,
		dir:          dir,
		debounce:     debounce,
		fileToTables: fileToTables,
		pending:      make(map[string]time.Time),
		onChange:     onChange,
		stopCh:       make(chan struct{}),
	}

	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}

	go fw.loop()
	return fw, nil
}

// loop processes fsnotify events and fires debounced callbacks.
func (fw *fileWatcher) loop() {
	ticker := time.NewTicker(100 * time.Millisecond) // check debounce timers
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			base := strings.ToUpper(filepath.Base(event.Name))
			// Strip extension if any (some ISAM files might have .dat etc)
			if ext := filepath.Ext(base); ext != "" {
				base = strings.TrimSuffix(base, ext)
			}
			fw.mu.Lock()
			if _, mapped := fw.fileToTables[base]; mapped {
				fw.pending[base] = time.Now()
			}
			fw.mu.Unlock()

		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Errors are non-fatal (e.g. too many watches) — just continue

		case <-ticker.C:
			fw.flushReady()

		case <-fw.stopCh:
			fw.watcher.Close()
			return
		}
	}
}

// flushReady fires callbacks for files whose debounce timer has expired.
func (fw *fileWatcher) flushReady() {
	now := time.Now()
	var readyTables []string

	fw.mu.Lock()
	for file, lastEvent := range fw.pending {
		if now.Sub(lastEvent) >= fw.debounce {
			readyTables = append(readyTables, fw.fileToTables[file]...)
			delete(fw.pending, file)
		}
	}
	fw.mu.Unlock()

	if len(readyTables) > 0 {
		// Deduplicate (multiple files can map to same table, e.g. Z08 → clients + terceros_ampliados)
		seen := make(map[string]bool, len(readyTables))
		unique := make([]string, 0, len(readyTables))
		for _, t := range readyTables {
			if !seen[t] {
				seen[t] = true
				unique = append(unique, t)
			}
		}
		fw.onChange(unique)
	}
}

// Stop shuts down the watcher.
func (fw *fileWatcher) Stop() {
	close(fw.stopCh)
}

// buildFileToTablesMap creates the mapping from ISAM filenames to sync table names
// using the sync registry's ORM model definitions.
func buildFileToTablesMap(registry map[string]*SyncTableDef) map[string][]string {
	m := make(map[string][]string)
	for tableName, def := range registry {
		if def.Model == nil {
			continue
		}
		// Model.Table.Path is the resolved full path (e.g. C:\Archivos Siigo\Z042016)
		if def.Model.Table == nil || def.Model.Table.Path == "" {
			continue
		}
		base := strings.ToUpper(filepath.Base(def.Model.Table.Path))
		m[base] = append(m[base], tableName)
	}
	return m
}
