package sync

import (
	"fmt"
	"log"
	"siigo-app/isam"
	"siigo-app/parsers"
)

// ChangeType indicates what kind of change was detected
type ChangeType string

const (
	ChangeNew     ChangeType = "new"
	ChangeUpdated ChangeType = "updated"
	ChangeDeleted ChangeType = "deleted"
)

// Change represents a detected change in a record
type Change struct {
	Type     ChangeType
	Key      string // unique identifier for the record
	Hash     string // current hash
	OldHash  string // previous hash (for updates)
}

// DetectResult holds the results of change detection for a file
type DetectResult struct {
	FileName    string
	HasChanges  bool
	Changes     []Change
	NewHashes   map[string]string
	RecordCount int
}

// DetectChanges checks if a file has changed and returns the differences
func DetectChanges(dataPath string, filename string, state *SyncState) (*DetectResult, error) {
	path := dataPath + filename
	fileState := state.GetFileState(filename)

	// Check modification time first (cheap check)
	modTime, err := isam.GetModTime(path)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	result := &DetectResult{
		FileName:  filename,
		NewHashes: make(map[string]string),
	}

	// If file hasn't been modified since last sync, skip
	if modTime == fileState.LastModified && fileState.RecordCount > 0 {
		return result, nil
	}

	log.Printf("[detector] File %s changed (mod=%d, prev=%d)", filename, modTime, fileState.LastModified)

	// Read and parse based on file type
	switch filename {
	case "Z17":
		return detectTercerosChanges(dataPath, fileState, modTime)
	case "Z06":
		return detectProductosChanges(dataPath, fileState, modTime)
	case "Z49":
		return detectMovimientosChanges(dataPath, fileState, modTime)
	default:
		return nil, fmt.Errorf("unknown file type: %s", filename)
	}
}

func detectTercerosChanges(dataPath string, fileState *FileState, modTime int64) (*DetectResult, error) {
	terceros, err := parsers.ParseTerceros(dataPath)
	if err != nil {
		return nil, err
	}

	result := &DetectResult{
		FileName:    "Z17",
		NewHashes:   make(map[string]string),
		RecordCount: len(terceros),
	}

	// Build current hash map
	currentHashes := make(map[string]string)
	for _, t := range terceros {
		key := t.TipoClave + "-" + t.Empresa + "-" + t.Codigo
		currentHashes[key] = t.Hash
	}

	// Compare with previous state
	result.Changes = compareHashes(fileState.RecordHashes, currentHashes)
	result.HasChanges = len(result.Changes) > 0
	result.NewHashes = currentHashes

	return result, nil
}

func detectProductosChanges(dataPath string, fileState *FileState, modTime int64) (*DetectResult, error) {
	productos, err := parsers.ParseProductos(dataPath)
	if err != nil {
		return nil, err
	}

	result := &DetectResult{
		FileName:    "Z06",
		NewHashes:   make(map[string]string),
		RecordCount: len(productos),
	}

	currentHashes := make(map[string]string)
	for _, p := range productos {
		key := p.Codigo
		if key == "" {
			key = p.Hash // use hash as key if no code found
		}
		currentHashes[key] = p.Hash
	}

	result.Changes = compareHashes(fileState.RecordHashes, currentHashes)
	result.HasChanges = len(result.Changes) > 0
	result.NewHashes = currentHashes

	return result, nil
}

func detectMovimientosChanges(dataPath string, fileState *FileState, modTime int64) (*DetectResult, error) {
	movimientos, err := parsers.ParseMovimientos(dataPath)
	if err != nil {
		return nil, err
	}

	result := &DetectResult{
		FileName:    "Z49",
		NewHashes:   make(map[string]string),
		RecordCount: len(movimientos),
	}

	currentHashes := make(map[string]string)
	for _, m := range movimientos {
		key := m.TipoComprobante + "-" + m.NumeroDoc + "-" + m.NitTercero
		if key == "--" {
			key = m.Hash
		}
		currentHashes[key] = m.Hash
	}

	result.Changes = compareHashes(fileState.RecordHashes, currentHashes)
	result.HasChanges = len(result.Changes) > 0
	result.NewHashes = currentHashes

	return result, nil
}

// compareHashes compares old and new hash maps, returning the changes
func compareHashes(oldHashes, newHashes map[string]string) []Change {
	var changes []Change

	// Find new and updated records
	for key, newHash := range newHashes {
		oldHash, exists := oldHashes[key]
		if !exists {
			changes = append(changes, Change{
				Type: ChangeNew,
				Key:  key,
				Hash: newHash,
			})
		} else if oldHash != newHash {
			changes = append(changes, Change{
				Type:    ChangeUpdated,
				Key:     key,
				Hash:    newHash,
				OldHash: oldHash,
			})
		}
	}

	// Find deleted records
	for key, oldHash := range oldHashes {
		if _, exists := newHashes[key]; !exists {
			changes = append(changes, Change{
				Type:    ChangeDeleted,
				Key:     key,
				OldHash: oldHash,
			})
		}
	}

	return changes
}
