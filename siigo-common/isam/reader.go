package isam

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// openWithRetry tries to open a file, retrying on Windows sharing violations.
func openWithRetry(path string) (*os.File, error) {
	var lastErr error
	for attempt := 0; attempt <= MaxLockRetries; attempt++ {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		// Check for Windows sharing violation (ERROR_SHARING_VIOLATION = 32)
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == 32 {
				lastErr = err
				if attempt < MaxLockRetries {
					log.Printf("[ISAM] File locked %s, retry %d/%d", path, attempt+1, MaxLockRetries)
					time.Sleep(LockRetryDelay)
					continue
				}
			}
		}
		return nil, err
	}
	return nil, fmt.Errorf("file locked after %d retries: %w", MaxLockRetries, lastErr)
}

// Record represents a single ISAM record with its raw bytes and offset in the file
type Record struct {
	Data   []byte
	Offset int
}

// IsamHeader contains metadata parsed from the ISAM file header (first 1024 bytes)
type IsamHeader struct {
	Magic           uint16 // 0x33FE (standard) or 0x30xx (variant)
	RecordSize      int    // Record size from offset 0x38 (big-endian 16-bit)
	ExpectedRecords int    // Expected record count from offset 0x40 (big-endian 32-bit)
	HasIndex        bool   // True if .idx file exists alongside data file
	IsValid         bool   // True if magic signature is recognized
}

// FileInfo contains metadata about an ISAM file
type FileInfo struct {
	Path       string
	RecordSize int
	Records    []Record
	Header     IsamHeader // Parsed header metadata
}

// Known valid record status flag nibbles (upper nibble of first marker byte)
// These indicate whether a record is active, deleted, etc.
// Observed values: 0x40 (active), 0x80, 0xC0, 0xE0
var validStatusNibbles = map[byte]bool{
	0x00: true, // no flags
	0x10: true,
	0x20: true,
	0x40: true, // active record (most common)
	0x60: true,
	0x80: true, // marked/modified
	0xA0: true,
	0xC0: true, // alternate status
	0xE0: true, // alternate status
}

// parseIsamHeader reads and validates the ISAM file header
func parseIsamHeader(data []byte, path string) (IsamHeader, error) {
	if len(data) < 0x44 {
		return IsamHeader{}, fmt.Errorf("file too small: %s (%d bytes)", path, len(data))
	}

	hdr := IsamHeader{}

	// Magic signature at offset 0x00 (big-endian 16-bit)
	hdr.Magic = binary.BigEndian.Uint16(data[0x00:0x02])

	// Validate magic: 0x33FE (standard indexed) or 0x30xx (variant, e.g., Z06)
	switch {
	case hdr.Magic == 0x33FE:
		hdr.IsValid = true
	case (hdr.Magic & 0xFF00) == 0x3000:
		hdr.IsValid = true // variant format (Z06 uses 0x3000)
	default:
		return hdr, fmt.Errorf("unrecognized ISAM magic 0x%04X in %s (expected 0x33FE or 0x30xx)", hdr.Magic, path)
	}

	// Record size at offset 0x38 (big-endian 16-bit)
	hdr.RecordSize = int(binary.BigEndian.Uint16(data[0x38:0x3A]))
	if hdr.RecordSize <= 0 || hdr.RecordSize > 60000 {
		return hdr, fmt.Errorf("invalid record size %d in %s", hdr.RecordSize, path)
	}

	// Expected record count at offset 0x40 (big-endian 32-bit)
	hdr.ExpectedRecords = int(binary.BigEndian.Uint32(data[0x40:0x44]))

	// Check for .idx file
	if _, err := os.Stat(path + ".idx"); err == nil {
		hdr.HasIndex = true
	}

	return hdr, nil
}

// ReadFile reads an ISAM file and extracts all records.
// Uses header validation, stricter record marker detection, and record count verification.
func ReadFile(path string) (*FileInfo, error) {
	// Open with retry on sharing violations (Siigo may have file locked)
	f, err := openWithRetry(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	data := make([]byte, stat.Size())
	_, err = f.Read(data)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	// Parse and validate header
	hdr, err := parseIsamHeader(data, path)
	if err != nil {
		return nil, err
	}

	recSize := hdr.RecordSize

	info := &FileInfo{
		Path:       path,
		RecordSize: recSize,
		Header:     hdr,
	}

	// Marker bytes for finding records
	recHi := byte((recSize >> 8) & 0xFF)
	recLo := byte(recSize & 0xFF)

	// Scan for records starting at offset 0x800 (after header + first index page)
	for pos := 0x800; pos < len(data)-recSize-2; pos++ {
		// Record marker: [statusNibble|recHi][recLo]
		if data[pos+1] != recLo || (data[pos]&0x0F) != recHi {
			continue
		}

		// Validate status nibble (upper 4 bits of first marker byte)
		statusNibble := data[pos] & 0xF0
		if !validStatusNibbles[statusNibble] {
			continue
		}

		// Verify it contains readable data (not index/empty pages)
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

	// Validate record count against header expectation
	if hdr.ExpectedRecords > 0 && len(info.Records) != hdr.ExpectedRecords {
		log.Printf("[ISAM] WARNING: %s header says %d records, found %d (delta=%d)",
			path, hdr.ExpectedRecords, len(info.Records),
			len(info.Records)-hdr.ExpectedRecords)
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
