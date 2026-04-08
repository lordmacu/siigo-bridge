package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"isam-admin/pkg/isam"
)

// ---------------------------------------------------------------------------
// files.go — File browser and ISAM file discovery handlers
// ---------------------------------------------------------------------------

// FileBrowseHandler lists files/directories in a given path
func FileBrowseHandler(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = "C:\\"
	}

	// Security: clean the path
	dir = filepath.Clean(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cannot read directory: "+err.Error())
		return
	}

	type FileEntry struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		IsDir       bool   `json:"is_dir"`
		IsEmpty     bool   `json:"is_empty,omitempty"`
		HasISAM     bool   `json:"has_isam,omitempty"`
		Size        int64  `json:"size"`
		IsISAM      bool   `json:"is_isam"`
		RecSize     int    `json:"rec_size,omitempty"`
		Records     int    `json:"records,omitempty"`
	}

	var result []FileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(dir, e.Name())
		entry := FileEntry{
			Name:  e.Name(),
			Path:  fullPath,
			IsDir: e.IsDir(),
			Size:  info.Size(),
		}

		// Check if directory has ISAM-compatible files
		if e.IsDir() {
			subEntries, err := os.ReadDir(fullPath)
			if err == nil {
				if len(subEntries) == 0 {
					entry.IsEmpty = true
				} else {
					// Check if any file inside could be ISAM (no extension or Z-prefix, size > 128)
					for _, sub := range subEntries {
						if sub.IsDir() {
							continue
						}
						subInfo, err := sub.Info()
						if err != nil {
							continue
						}
						if subInfo.Size() > 128 {
							ext := filepath.Ext(sub.Name())
							if ext == "" || strings.HasPrefix(strings.ToUpper(sub.Name()), "Z") {
								entry.HasISAM = true
								break
							}
						}
					}
				}
			}
		}

		// Check if it's a potential ISAM file (no extension, or known patterns)
		if !e.IsDir() && info.Size() > 128 {
			ext := filepath.Ext(e.Name())
			if ext == "" || strings.HasPrefix(strings.ToUpper(e.Name()), "Z") {
				// Try to parse as ISAM
				if fi, hdr, err := isam.ReadIsamFile(fullPath); err == nil {
					entry.IsISAM = true
					entry.RecSize = int(hdr.MaxRecordLen)
					entry.Records = len(fi.Records)
				}
			}
		}

		result = append(result, entry)
	}

	writeJSON(w, map[string]interface{}{
		"path":    dir,
		"parent":  filepath.Dir(dir),
		"entries": result,
	})
}

// FileInfoHandler returns detailed info about a specific ISAM file
func FileInfoHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path parameter required")
		return
	}

	path = filepath.Clean(path)

	fi, hdr, err := isam.ReadIsamFile(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Not a valid ISAM file: "+err.Error())
		return
	}

	stat, _ := os.Stat(path)

	writeJSON(w, map[string]interface{}{
		"path":          path,
		"name":          filepath.Base(path),
		"size":          stat.Size(),
		"record_size":   hdr.MaxRecordLen,
		"record_count":  len(fi.Records),
		"organization":  hdr.Organization,
		"idx_format":    hdr.IdxFormat,
		"alignment":     hdr.Alignment,
		"compression":   hdr.Compression,
		"created":       hdr.CreationDate,
		"modified":      hdr.ModifiedDate,
		"long_records":  hdr.LongRecords,
		"rec_hdr_size":  hdr.RecHeaderSize,
	})
}

// FileHexHandler returns hex dump of sample records for the import wizard
func FileHexHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path parameter required")
		return
	}

	path = filepath.Clean(path)

	fi, hdr, err := isam.ReadIsamFile(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Not a valid ISAM file: "+err.Error())
		return
	}

	// Return up to 20 sample records with hex + ASCII
	maxRecords := 20
	if len(fi.Records) < maxRecords {
		maxRecords = len(fi.Records)
	}

	type HexRecord struct {
		Index int      `json:"index"`
		Hex   []string `json:"hex"`   // hex bytes per line (16 bytes each)
		ASCII string   `json:"ascii"` // full ASCII representation
	}

	var records []HexRecord
	for i := 0; i < maxRecords; i++ {
		rec := fi.Records[i]
		hr := HexRecord{
			Index: i,
		}

		// Build hex lines (16 bytes per line)
		for offset := 0; offset < len(rec.Data); offset += 16 {
			end := offset + 16
			if end > len(rec.Data) {
				end = len(rec.Data)
			}
			line := ""
			for j := offset; j < end; j++ {
				if j > offset {
					line += " "
				}
				line += fmt.Sprintf("%02X", rec.Data[j])
			}
			hr.Hex = append(hr.Hex, line)
		}

		// Build ASCII (replace non-printable with '.')
		ascii := make([]byte, len(rec.Data))
		for j, b := range rec.Data {
			if b >= 32 && b < 127 {
				ascii[j] = b
			} else {
				ascii[j] = '.'
			}
		}
		hr.ASCII = string(ascii)

		records = append(records, hr)
	}

	writeJSON(w, map[string]interface{}{
		"path":         path,
		"record_size":  hdr.MaxRecordLen,
		"record_count": len(fi.Records),
		"samples":      records,
	})
}

// FieldDetectHandler auto-detects potential field boundaries in ISAM records
func FieldDetectHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path parameter required")
		return
	}

	path = filepath.Clean(path)

	fi, hdr, err := isam.ReadIsamFile(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Not a valid ISAM file: "+err.Error())
		return
	}

	recSize := int(hdr.MaxRecordLen)
	maxSamples := 50
	if len(fi.Records) < maxSamples {
		maxSamples = len(fi.Records)
	}

	// Analyze character types at each byte position across samples

	stats := make([]ByteStats, recSize)
	for i := 0; i < maxSamples; i++ {
		data := fi.Records[i].Data
		for j := 0; j < recSize && j < len(data); j++ {
			b := data[j]
			stats[j].Samples++
			switch {
			case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z':
				stats[j].AlphaCount++
			case b >= '0' && b <= '9':
				stats[j].DigitCount++
			case b == ' ':
				stats[j].SpaceCount++
			case b == 0:
				stats[j].NullCount++
			default:
				stats[j].BinaryCount++
			}
		}
	}

	// Detect field boundaries: transitions between character types
	type DetectedField struct {
		Offset   int    `json:"offset"`
		Length   int    `json:"length"`
		Type     string `json:"type"`     // "text", "numeric", "date", "binary", "empty"
		Sample   string `json:"sample"`   // sample value from first record
		Confidence float64 `json:"confidence"`
	}

	var fields []DetectedField
	pos := 0
	for pos < recSize {
		// Determine dominant type at this position
		fieldType := classifyPosition(stats[pos])
		if fieldType == "empty" {
			pos++
			continue
		}

		// Extend field while type stays the same
		end := pos + 1
		for end < recSize && classifyPosition(stats[end]) == fieldType {
			end++
		}

		// Get sample value from first record
		sample := ""
		if len(fi.Records) > 0 && end <= len(fi.Records[0].Data) {
			raw := fi.Records[0].Data[pos:end]
			sampleBytes := make([]byte, len(raw))
			for k, b := range raw {
				if b >= 32 && b < 127 {
					sampleBytes[k] = b
				} else {
					sampleBytes[k] = '.'
				}
			}
			sample = string(sampleBytes)
		}

		// Check if it looks like a date (8 digits, values in date range)
		if fieldType == "numeric" && (end-pos) == 8 {
			fieldType = "date"
		}

		confidence := 0.7
		if maxSamples >= 10 {
			confidence = 0.9
		}

		fields = append(fields, DetectedField{
			Offset:     pos,
			Length:     end - pos,
			Type:       fieldType,
			Sample:     sample,
			Confidence: confidence,
		})

		pos = end
	}

	writeJSON(w, map[string]interface{}{
		"path":         path,
		"record_size":  recSize,
		"record_count": len(fi.Records),
		"detected_fields": fields,
		"byte_stats":   stats,
	})
}

// ByteStats tracks character type statistics at a byte position
type ByteStats struct {
	AlphaCount  int `json:"alpha"`
	DigitCount  int `json:"digit"`
	SpaceCount  int `json:"space"`
	NullCount   int `json:"null"`
	BinaryCount int `json:"binary"`
	Samples     int `json:"samples"`
}

func classifyPosition(s ByteStats) string {
	if s.Samples == 0 {
		return "empty"
	}
	total := s.Samples
	if s.NullCount*100/total > 80 {
		return "empty"
	}
	if s.BinaryCount*100/total > 50 {
		return "binary"
	}
	if s.DigitCount*100/total > 60 {
		return "numeric"
	}
	if (s.AlphaCount+s.SpaceCount)*100/total > 50 {
		return "text"
	}
	if s.DigitCount*100/total > 30 {
		return "numeric"
	}
	return "text"
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
