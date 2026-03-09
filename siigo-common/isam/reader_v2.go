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
// Per Micro Focus spec: 0=null, 1=system, 2=pointer, 3=deleted, 4=normal, ...
const (
	RecTypeNull    = 0x0 // Null/padding
	RecTypeSystem  = 0x1 // Free space (variable indexed)
	RecTypePointer = 0x2 // Pointer record (indexed)
	RecTypeDeleted = 0x3 // Deleted record (slot available for reuse)
	RecTypeNormal  = 0x4 // Normal user data record
	RecTypeReduced = 0x5 // Reduced record (indexed)
	RecTypePtrRef  = 0x6 // Pointer record referenced by another pointer
	RecTypeRefData = 0x7 // Data record referenced by pointer
	RecTypeRedRef  = 0x8 // Reduced record referenced by pointer
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
	HeaderSize    int  // 128 for MF stream, 0x800 for indexed marker
	Alignment     int  // 8 for IDXFORMAT 8, 4 for 3/4, 1 otherwise
	RecHeaderSize int  // 2 or 4 bytes
	IsIndexed     bool // true for 0x33FE indexed marker format
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
		// MF Indexed marker format — data zone starts at 0x800
		h.LongRecords = false
		h.IsIndexed = true
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

	if h.IsIndexed {
		// 0x33FE indexed marker format: different header layout
		// Record size at 0x38 (2 bytes), organization at 0x26 (2 bytes)
		if len(data) < 0x44 {
			return nil, fmt.Errorf("indexed header too short: %d bytes", len(data))
		}
		h.MaxRecordLen = uint32(binary.BigEndian.Uint16(data[0x38:0x3A]))
		h.MinRecordLen = h.MaxRecordLen
		h.Organization = byte(binary.BigEndian.Uint16(data[0x26:0x28]))
		h.IdxFormat = data[0x2B]
		h.HeaderSize = 0x800
		h.RecHeaderSize = 2 // indexed markers are always 2-byte status+length
		h.Alignment = 0     // no alignment padding in indexed marker format
	} else {
		// MF stream format: standard 128-byte header layout
		h.HeaderSize = 128

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
		RecTypePointer: "POINTER",
		RecTypeDeleted: "DELETED",
		RecTypeNormal:  "NORMAL",
		RecTypeReduced: "REDUCED",
		RecTypePtrRef:  "PTR_REF",
		RecTypeRefData: "REF_DATA",
		RecTypeRedRef:  "RED_REF",
	}
	if n, ok := names[t]; ok {
		return n
	}
	return fmt.Sprintf("UNKNOWN(0x%X)", t)
}

// isValidIndexedStatus returns true for known indexed marker status nibbles
func isValidIndexedStatus(s byte) bool {
	switch s {
	case 0, 1, 2, 4, 6, 8, 10, 12, 14:
		return true
	}
	return false
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

	// Use LogicalEnd to limit scan if available (avoid reading garbage/padding)
	scanEnd := len(data)
	if hdr.LogicalEnd > 0 && int(hdr.LogicalEnd) < scanEnd {
		scanEnd = int(hdr.LogicalEnd)
	}

	if hdr.IsIndexed {
		// Indexed marker format: 2-byte markers with status nibble + 12-bit length
		// Only accept records where length == recSize (exact match filters out index pages)
		recHi := byte((recSize >> 8) & 0x0F)
		recLo := byte(recSize & 0xFF)

		for pos+2+recSize <= scanEnd {
			b0 := data[pos]
			b1 := data[pos+1]

			// Fast check: bottom 12 bits must encode exactly recSize
			if (b0&0x0F) != recHi || b1 != recLo {
				pos++
				continue
			}

			// Validate status nibble
			status := byte((b0 & 0xF0) >> 4)
			if !isValidIndexedStatus(status) {
				pos++
				continue
			}

			payloadStart := pos + 2
			payloadEnd := payloadStart + recSize

			// Skip deleted records (status 0x1)
			if status == 0x1 {
				pos = payloadEnd
				continue
			}

			rec := make([]byte, recSize)
			copy(rec, data[payloadStart:payloadEnd])
			info.Records = append(info.Records, Record{
				Data:   rec,
				Offset: pos,
			})

			pos = payloadEnd
		}
	} else {
		// MF stream format: record type nibble + data length
		for pos < scanEnd {
			if pos+hdr.RecHeaderSize > scanEnd {
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
				recType = byte((marker >> 28) & 0x0F)
				dataLen = int(marker & 0x0FFFFFFF)
			}

			// NULL record type with zero length = padding
			if recType == RecTypeNull && dataLen == 0 {
				pos++ // byte-by-byte skip through padding
				continue
			}

			// Validate data length
			if dataLen < 0 || dataLen > scanEnd-pos-hdr.RecHeaderSize {
				pos++ // byte-by-byte recovery
				continue
			}

			dataStart := pos + hdr.RecHeaderSize
			dataEnd := dataStart + dataLen
			if dataEnd > scanEnd {
				break
			}

			// Classify record
			isData := recType == RecTypeNormal || recType == RecTypeReduced ||
				recType == RecTypeRefData || recType == RecTypeRedRef

			if isData && dataLen > 0 {
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

				info.Records = append(info.Records, Record{
					Data:   rec,
					Offset: pos,
				})
			}

			// Advance past record header + data + alignment padding
			consumed := hdr.RecHeaderSize + dataLen
			if hdr.Alignment > 1 {
				slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
				consumed += slack
			}
			pos += consumed
		}
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
	PtrRefCount  int
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

	// Use LogicalEnd to limit scan if available
	scanEnd := len(data)
	if hdr.LogicalEnd > 0 && int(hdr.LogicalEnd) < scanEnd {
		scanEnd = int(hdr.LogicalEnd)
	}

	if hdr.IsIndexed {
		// Indexed marker format: only accept exact recSize matches
		recHi := byte((recSize >> 8) & 0x0F)
		recLo := byte(recSize & 0xFF)

		for pos+2+recSize <= scanEnd {
			b0 := data[pos]
			b1 := data[pos+1]

			if (b0&0x0F) != recHi || b1 != recLo {
				pos++
				continue
			}

			status := byte((b0 & 0xF0) >> 4)
			if !isValidIndexedStatus(status) {
				pos++
				continue
			}

			payloadStart := pos + 2
			payloadEnd := payloadStart + recSize

			stats.DataTypes[status]++

			if status == 0x1 {
				stats.DeletedCount++
				pos = payloadEnd
				continue
			}

			stats.TotalRecords++
			rec := make([]byte, recSize)
			copy(rec, data[payloadStart:payloadEnd])
			records = append(records, rec)

			pos = payloadEnd
		}
	} else {
		// MF stream format
		for pos < scanEnd {
			if pos+hdr.RecHeaderSize > scanEnd {
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
				recType = byte((marker >> 28) & 0x0F)
				dataLen = int(marker & 0x0FFFFFFF)
			}

			if recType == RecTypeNull && dataLen == 0 {
				stats.NullCount++
				pos++
				continue
			}

			if dataLen < 0 || dataLen > scanEnd-pos-hdr.RecHeaderSize {
				pos++
				continue
			}

			stats.DataTypes[recType]++

			dataStart := pos + hdr.RecHeaderSize
			dataEnd := dataStart + dataLen
			if dataEnd > scanEnd {
				break
			}

			isData := recType == RecTypeNormal || recType == RecTypeReduced ||
				recType == RecTypeRefData || recType == RecTypeRedRef

			switch recType {
			case RecTypeDeleted:
				stats.DeletedCount++
			case RecTypeSystem:
				stats.SystemCount++
			case RecTypePtrRef:
				stats.PtrRefCount++
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
			v2stats.PtrRefCount, v2stats.PointerCount)
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
