package isam

import (
	"encoding/binary"
	"fmt"
	"log"
)

// ---------------------------------------------------------------------------
// reader_v2.go — Specification-based ISAM binary reader
//
// Based on the Micro Focus on-disk format documented in:
//   - mfcobol-export (Python): record headers, alignment, record types
//   - SimoTime: IDXFORMAT, EXTFH config, numeric formats
//   - GnuCOBOL fileio.c: magic detection, header layout
//   - Real hex dumps from 18 Siigo ISAM files (IDXFORMAT 8)
//
// Key differences from reader.go (v1):
//   - Parses the 128-byte file header per spec (org, idxformat, reclen, dates)
//   - Uses record type nibble (top 4 bits of 2-byte marker) to classify records
//   - Respects 8-byte alignment for IDXFORMAT 8
//   - No heuristic "readability check" — trusts the format
//   - Handles Z06-style variant magic (0x30xx where bytes 2-3 ≠ 0x7E00)
//   - Properly skips index pages, deleted records, and null padding
// ---------------------------------------------------------------------------

// Record type codes (top 4 bits of the 2-byte record header)
const (
	RecTypeNull     = 0x0 // Null/padding
	RecTypeSystem   = 0x1 // Free space (variable indexed)
	RecTypeDeleted  = 0x2 // Deleted record (slot available for reuse)
	RecTypeHeader   = 0x3 // File/index header record
	RecTypeNormal   = 0x4 // Normal user data record
	RecTypeReduced  = 0x5 // Reduced record (indexed)
	RecTypePointer  = 0x6 // Pointer record (indexed)
	RecTypeRefData  = 0x7 // Data record referenced by pointer
	RecTypeRedRef   = 0x8 // Reduced record referenced by pointer
)

// V2Header contains metadata parsed from the 128-byte ISAM file header
type V2Header struct {
	// Bytes 0-3: magic / long-record flag
	Magic       uint32
	LongRecords bool // true if 4-byte record headers (records ≥ 4096 bytes)

	// Bytes 4-7: sequence and integrity
	DBSequence    uint16
	IntegrityFlag uint16 // non-zero = possibly corrupt

	// Bytes 8-35: timestamps
	CreationDate string // YYMMDDHHMMSScc (14 chars)
	ModifiedDate string // YYMMDDHHMMSScc (14 chars)

	// Bytes 39, 41, 43, 48: file attributes
	Organization byte // 1=Sequential, 2=Indexed, 3=Relative
	Compression  byte // 0=None, 1=CBLDC001
	IdxFormat    byte // 0,1,2,3,4,8 = IDXFORMAT
	RecordMode   byte // 0=Fixed, 1=Variable

	// Bytes 54-61: record sizes (big-endian 32-bit)
	MaxRecordLen uint32
	MinRecordLen uint32

	// Bytes 108-111: handler version
	HandlerVersion uint32

	// Bytes 120-127: logical end offset (for indexed files)
	LogicalEnd uint64

	// Derived
	HeaderSize    int // always 128
	Alignment     int // 8 for IDXFORMAT 8, 4 for 3/4, 1 otherwise
	RecHeaderSize int // 2 or 4 bytes
}

// parseV2Header parses the 128-byte ISAM file header
func parseV2Header(data []byte) (*V2Header, error) {
	if len(data) < 128 {
		return nil, fmt.Errorf("file too small for header: %d bytes", len(data))
	}

	h := &V2Header{HeaderSize: 128}

	// Magic at offset 0-3
	h.Magic = binary.BigEndian.Uint32(data[0:4])

	// Detect record header size from magic
	switch {
	case data[0] == 0x30 && data[1] == 0x7E && data[2] == 0x00 && data[3] == 0x00:
		// Standard short-record format
		h.LongRecords = false
	case data[0] == 0x30 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x7C:
		// Long-record format (records ≥ 4096)
		h.LongRecords = true
	case data[0] == 0x33 && data[1] == 0xFE:
		// MF Indexed format — most Siigo files
		h.LongRecords = false
	case (data[0] & 0xF0) == 0x30:
		// Variant MF format (e.g., Z06 uses 0x30 0x00 0x03 0xFC)
		// Check if MaxRecLen would require long records
		maxRec := binary.BigEndian.Uint32(data[54:58])
		h.LongRecords = maxRec >= 4096
	default:
		return nil, fmt.Errorf("unrecognized ISAM magic: 0x%08X", h.Magic)
	}

	h.RecHeaderSize = 2
	if h.LongRecords {
		h.RecHeaderSize = 4
	}

	// Sequence and integrity
	h.DBSequence = binary.BigEndian.Uint16(data[4:6])
	h.IntegrityFlag = binary.BigEndian.Uint16(data[6:8])

	// Timestamps (14 chars each)
	h.CreationDate = string(data[8:22])
	h.ModifiedDate = string(data[22:36])

	// File attributes
	h.Organization = data[39]
	h.Compression = data[41]
	h.IdxFormat = data[43]
	h.RecordMode = data[48]

	// Record sizes
	h.MaxRecordLen = binary.BigEndian.Uint32(data[54:58])
	h.MinRecordLen = binary.BigEndian.Uint32(data[58:62])

	// Handler version (for indexed)
	if h.Organization == 2 {
		h.HandlerVersion = binary.BigEndian.Uint32(data[108:112])
		h.LogicalEnd = binary.BigEndian.Uint64(data[120:128])
	}

	// Alignment depends on IDXFORMAT
	switch h.IdxFormat {
	case 8:
		h.Alignment = 8
	case 3, 4:
		h.Alignment = 4
	default:
		h.Alignment = 1
	}

	// Sanity checks
	if h.MaxRecordLen == 0 || h.MaxRecordLen > 100000 {
		return nil, fmt.Errorf("invalid MaxRecordLen: %d", h.MaxRecordLen)
	}

	return h, nil
}

// recTypeName returns a human-readable name for a record type
func recTypeName(t byte) string {
	names := map[byte]string{
		RecTypeNull:    "NULL",
		RecTypeSystem:  "SYSTEM",
		RecTypeDeleted: "DELETED",
		RecTypeHeader:  "HEADER",
		RecTypeNormal:  "NORMAL",
		RecTypeReduced: "REDUCED",
		RecTypePointer: "POINTER",
		RecTypeRefData: "REF_DATA",
		RecTypeRedRef:  "RED_REF",
	}
	if n, ok := names[t]; ok {
		return n
	}
	return fmt.Sprintf("UNKNOWN(0x%X)", t)
}

// V2Record represents a record parsed by the v2 reader
type V2Record struct {
	Data       []byte
	Offset     int  // file offset where the record header starts
	RecType    byte // record type (top nibble)
	DataLen    int  // data length from record header
	IsDeleted  bool
	IsDataRec  bool // true if this is a user data record
}

// ReadFileV2 reads an ISAM file using the specification-based v2 parser.
// Returns records, header info, and error.
func ReadFileV2(path string) (*FileInfo, *V2Header, error) {
	f, err := openWithRetry(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	fileSize := stat.Size()
	data := make([]byte, fileSize)
	_, err = f.Read(data)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	// Parse header
	hdr, err := parseV2Header(data)
	if err != nil {
		return nil, nil, fmt.Errorf("header parse %s: %w", path, err)
	}

	recSize := int(hdr.MaxRecordLen)
	info := &FileInfo{
		Path:       path,
		RecordSize: recSize,
		Header: IsamHeader{
			Magic:      uint16(hdr.Magic >> 16),
			RecordSize: recSize,
			IsValid:    true,
		},
	}

	// Parse records starting after header
	pos := hdr.HeaderSize

	// For IDXFORMAT 8 indexed files, we need to scan through the data
	// respecting record headers, types, and alignment
	for pos < len(data) {
		// Check if we have enough space for a record header
		if pos+hdr.RecHeaderSize > len(data) {
			break
		}

		var recType byte
		var dataLen int

		if hdr.RecHeaderSize == 2 {
			// 2-byte header: top 4 bits = type, bottom 12 bits = length
			marker := binary.BigEndian.Uint16(data[pos : pos+2])
			recType = byte(marker >> 12)
			dataLen = int(marker & 0x0FFF)
		} else {
			// 4-byte header: top 5 bits = type, bottom 27 bits = length
			marker := binary.BigEndian.Uint32(data[pos : pos+4])
			recType = byte(marker >> 27)
			dataLen = int(marker & 0x07FFFFFF)
		}

		// NULL record type with zero length = end of useful data or padding
		if recType == RecTypeNull && dataLen == 0 {
			// Skip one alignment unit and try again
			pos += hdr.Alignment
			if hdr.Alignment <= 0 {
				pos++
			}
			continue
		}

		// Validate data length
		if dataLen < 0 || dataLen > len(data)-pos-hdr.RecHeaderSize {
			// Invalid length — try advancing by alignment
			pos += hdr.Alignment
			if hdr.Alignment <= 0 {
				pos++
			}
			continue
		}

		// Extract record data
		dataStart := pos + hdr.RecHeaderSize
		dataEnd := dataStart + dataLen

		if dataEnd > len(data) {
			break
		}

		// Classify record
		isData := recType == RecTypeNormal || recType == RecTypeReduced ||
			recType == RecTypeRefData || recType == RecTypeRedRef
		isDeleted := recType == RecTypeDeleted

		if isData && dataLen > 0 {
			// For fixed-length files, the data length should match MaxRecordLen
			// But sometimes the marker encodes less. Use MaxRecordLen for extraction.
			extractLen := recSize
			if dataLen < recSize {
				// Record header says shorter — use what it says
				extractLen = dataLen
			}

			rec := make([]byte, recSize)
			copyLen := extractLen
			if dataStart+copyLen > len(data) {
				copyLen = len(data) - dataStart
			}
			copy(rec, data[dataStart:dataStart+copyLen])

			info.Records = append(info.Records, Record{
				Data:   rec,
				Offset: pos,
			})
		} else if isDeleted && dataLen > 0 {
			// Count deleted but don't include
			// (could add a flag to include deleted records)
		}

		// Advance past record header + data + alignment padding
		consumed := hdr.RecHeaderSize + dataLen
		if hdr.Alignment > 1 {
			slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
			consumed += slack
		}
		pos += consumed
	}

	return info, hdr, nil
}

// ReadFileV2All reads all records using the v2 parser, returning raw byte slices.
// Compatible with the ReadIsamFile return signature.
func ReadFileV2All(path string) ([][]byte, int, error) {
	info, _, err := ReadFileV2(path)
	if err != nil {
		return nil, 0, err
	}
	records := make([][]byte, len(info.Records))
	for i, r := range info.Records {
		records[i] = r.Data
	}
	return records, info.RecordSize, nil
}

// ReadFileV2WithStats reads all records and returns diagnostic stats
type V2Stats struct {
	Header       *V2Header
	TotalRecords int
	DeletedCount int
	NullCount    int
	SystemCount  int
	HeaderCount  int
	PointerCount int
	DataTypes    map[byte]int // count by record type
}

func ReadFileV2WithStats(path string) ([][]byte, *V2Stats, error) {
	f, err := openWithRetry(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	fileSize := stat.Size()
	data := make([]byte, fileSize)
	_, err = f.Read(data)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	hdr, err := parseV2Header(data)
	if err != nil {
		return nil, nil, fmt.Errorf("header parse %s: %w", path, err)
	}

	recSize := int(hdr.MaxRecordLen)
	stats := &V2Stats{
		Header:    hdr,
		DataTypes: make(map[byte]int),
	}

	var records [][]byte
	pos := hdr.HeaderSize

	for pos < len(data) {
		if pos+hdr.RecHeaderSize > len(data) {
			break
		}

		var recType byte
		var dataLen int

		if hdr.RecHeaderSize == 2 {
			marker := binary.BigEndian.Uint16(data[pos : pos+2])
			recType = byte(marker >> 12)
			dataLen = int(marker & 0x0FFF)
		} else {
			marker := binary.BigEndian.Uint32(data[pos : pos+4])
			recType = byte(marker >> 27)
			dataLen = int(marker & 0x07FFFFFF)
		}

		if recType == RecTypeNull && dataLen == 0 {
			stats.NullCount++
			pos += hdr.Alignment
			if hdr.Alignment <= 0 {
				pos++
			}
			continue
		}

		if dataLen < 0 || dataLen > len(data)-pos-hdr.RecHeaderSize {
			pos += hdr.Alignment
			if hdr.Alignment <= 0 {
				pos++
			}
			continue
		}

		stats.DataTypes[recType]++

		dataStart := pos + hdr.RecHeaderSize
		dataEnd := dataStart + dataLen
		if dataEnd > len(data) {
			break
		}

		isData := recType == RecTypeNormal || recType == RecTypeReduced ||
			recType == RecTypeRefData || recType == RecTypeRedRef

		switch recType {
		case RecTypeDeleted:
			stats.DeletedCount++
		case RecTypeSystem:
			stats.SystemCount++
		case RecTypeHeader:
			stats.HeaderCount++
		case RecTypePointer:
			stats.PointerCount++
		}

		if isData && dataLen > 0 {
			stats.TotalRecords++
			extractLen := recSize
			if dataLen < recSize {
				extractLen = dataLen
			}
			rec := make([]byte, recSize)
			copyLen := extractLen
			if dataStart+copyLen > len(data) {
				copyLen = len(data) - dataStart
			}
			copy(rec, data[dataStart:dataStart+copyLen])
			records = append(records, rec)
		}

		consumed := hdr.RecHeaderSize + dataLen
		if hdr.Alignment > 1 {
			slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
			consumed += slack
		}
		pos += consumed
	}

	return records, stats, nil
}

// CompareV1V2 reads a file with both v1 and v2 readers and logs differences
func CompareV1V2(path string) {
	// V1
	v1info, v1err := ReadFile(path)
	v1count := 0
	if v1err == nil {
		v1count = len(v1info.Records)
	}

	// V2
	v2records, v2stats, v2err := ReadFileV2WithStats(path)
	v2count := len(v2records)

	log.Printf("[COMPARE] %s:", path)
	if v1err != nil {
		log.Printf("  V1 ERROR: %v", v1err)
	} else {
		log.Printf("  V1: %d records (recSize=%d)", v1count, v1info.RecordSize)
	}
	if v2err != nil {
		log.Printf("  V2 ERROR: %v", v2err)
	} else {
		log.Printf("  V2: %d records (recSize=%d, idxfmt=%d, org=%d)",
			v2count, v2stats.Header.MaxRecordLen, v2stats.Header.IdxFormat, v2stats.Header.Organization)
		log.Printf("  V2 record types: %v", v2stats.DataTypes)
		log.Printf("  V2 deleted=%d null=%d system=%d header=%d pointer=%d",
			v2stats.DeletedCount, v2stats.NullCount, v2stats.SystemCount,
			v2stats.HeaderCount, v2stats.PointerCount)
	}

	if v1err == nil && v2err == nil {
		if v1count == v2count {
			log.Printf("  MATCH: both found %d records", v1count)
			// Compare first record data
			if v1count > 0 {
				v1rec := v1info.Records[0].Data
				v2rec := v2records[0]
				same := true
				for i := 0; i < len(v1rec) && i < len(v2rec); i++ {
					if v1rec[i] != v2rec[i] {
						same = false
						log.Printf("  DIFF at byte %d: v1=0x%02X v2=0x%02X", i, v1rec[i], v2rec[i])
						break
					}
				}
				if same {
					log.Printf("  First record data: IDENTICAL")
				}
			}
		} else {
			log.Printf("  MISMATCH: v1=%d vs v2=%d (delta=%d)", v1count, v2count, v2count-v1count)
		}
	}
}
