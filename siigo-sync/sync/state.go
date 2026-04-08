package sync

import (
	"encoding/json"
	"os"
	"time"
)

// FileState tracks the last known state of an ISAM file
type FileState struct {
	Path         string            `json:"path"`
	LastModified int64             `json:"last_modified"` // UnixNano
	LastSync     time.Time         `json:"last_sync"`
	RecordHashes map[string]string `json:"record_hashes"` // key -> hash
	RecordCount  int               `json:"record_count"`
}

// SyncState holds the state of all monitored files
type SyncState struct {
	Files   map[string]*FileState `json:"files"`   // filename -> state
	Version int                   `json:"version"`
}

// NewSyncState creates an empty sync state
func NewSyncState() *SyncState {
	return &SyncState{
		Files:   make(map[string]*FileState),
		Version: 1,
	}
}

// LoadState reads the state from disk
func LoadState(path string) (*SyncState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewSyncState(), nil
		}
		return nil, err
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return NewSyncState(), nil
	}

	if state.Files == nil {
		state.Files = make(map[string]*FileState)
	}

	return &state, nil
}

// Save writes the state to disk
func (s *SyncState) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetFileState returns the state for a file, creating it if needed
func (s *SyncState) GetFileState(filename string) *FileState {
	if fs, ok := s.Files[filename]; ok {
		return fs
	}
	fs := &FileState{
		RecordHashes: make(map[string]string),
	}
	s.Files[filename] = fs
	return fs
}

// UpdateFileState updates the state after a successful sync
func (s *SyncState) UpdateFileState(filename string, modTime int64, hashes map[string]string, count int) {
	fs := s.GetFileState(filename)
	fs.LastModified = modTime
	fs.LastSync = time.Now()
	fs.RecordHashes = hashes
	fs.RecordCount = count
}
