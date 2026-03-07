package isam

import (
	"fmt"
	"os"

	"golang.org/x/text/encoding/charmap"
)

// Record represents a single ISAM record with its raw bytes and offset in the file
type Record struct {
	Data   []byte
	Offset int
}

// FileInfo contains metadata about an ISAM file
type FileInfo struct {
	Path       string
	RecordSize int
	Records    []Record
}

// ReadFile reads an ISAM file and extracts all records
func ReadFile(path string) (*FileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	if len(data) < 0x40 {
		return nil, fmt.Errorf("file too small: %s (%d bytes)", path, len(data))
	}

	// Record size is at offset 0x38, big-endian 16-bit
	recSize := int(data[0x38])<<8 | int(data[0x39])
	if recSize <= 0 || recSize > 60000 {
		return nil, fmt.Errorf("invalid record size %d in %s", recSize, path)
	}

	info := &FileInfo{
		Path:       path,
		RecordSize: recSize,
	}

	// Marker bytes for finding records
	recHi := byte((recSize >> 8) & 0xFF)
	recLo := byte(recSize & 0xFF)

	// Scan for records starting at offset 0x800 (after header + first index page)
	for pos := 0x800; pos < len(data)-recSize-2; pos++ {
		// Record marker: [flags|recHi][recLo]
		if data[pos+1] != recLo || (data[pos]&0x0F) != recHi {
			continue
		}

		// Verify it contains readable text (not index/empty pages)
		textStart := pos + 2
		readableCount := 0
		checkLen := 30
		if textStart+checkLen > len(data) {
			checkLen = len(data) - textStart
		}
		for i := textStart; i < textStart+checkLen; i++ {
			if data[i] >= 0x20 && data[i] < 0xFF {
				readableCount++
			}
		}

		if readableCount > 15 {
			rec := make([]byte, recSize)
			copyLen := recSize
			if textStart+copyLen > len(data) {
				copyLen = len(data) - textStart
			}
			copy(rec, data[textStart:textStart+copyLen])

			info.Records = append(info.Records, Record{
				Data:   rec,
				Offset: pos,
			})
			pos += recSize // skip past this record
		}
	}

	return info, nil
}

// DecodeText decodes a byte slice from Windows-1252 to UTF-8 string
func DecodeText(data []byte) string {
	decoder := charmap.Windows1252.NewDecoder()
	result, err := decoder.Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(result)
}

// ExtractField extracts a fixed-length text field, trimming spaces and nulls
func ExtractField(rec []byte, offset, length int) string {
	if offset >= len(rec) {
		return ""
	}
	end := offset + length
	if end > len(rec) {
		end = len(rec)
	}

	field := rec[offset:end]

	// Trim trailing spaces and nulls
	trimEnd := len(field)
	for trimEnd > 0 && (field[trimEnd-1] == ' ' || field[trimEnd-1] == 0) {
		trimEnd--
	}

	return DecodeText(field[:trimEnd])
}

// ExtractNumericField extracts a numeric field (stored as text with leading zeros)
func ExtractNumericField(rec []byte, offset, length int) string {
	s := ExtractField(rec, offset, length)
	// Trim leading zeros but keep at least one digit
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}

// GetModTime returns the modification time of a file
func GetModTime(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.ModTime().UnixNano(), nil
}
