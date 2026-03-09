package isam

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// ---------------------------------------------------------------------------
// create_file.go — Create new ISAM files from scratch
//
// Generates a valid Micro Focus ISAM IDXFORMAT 8 file with:
//   - 128-byte header (0x33FE magic for indexed files)
//   - Initial empty B-tree root node (leaf, 0 entries)
//   - Ready for InsertRecord() to add data
//
// Usage:
//
//	schema := isam.NewSchema(256).
//	    KeyField("codigo", 0, 5).
//	    StringField("nombre", 5, 40).
//	    StringField("ciudad", 45, 30).
//	    BCDField("saldo", 100, 8, 2)
//
//	err := isam.CreateFile(`C:\DATA\MYFILE`, schema)
//	// File is now ready for use with the ORM
//
//	table := isam.NewTable("myfile", `C:\DATA\MYFILE`, 256).
//	    Key("codigo", 0, 5).
//	    String("nombre", 5, 40)
//
//	rec := table.New()
//	rec.Set("codigo", "00001")
//	rec.Set("nombre", "TEST")
//	rec.Save()
//
// ---------------------------------------------------------------------------

// Schema defines the structure of a new ISAM file to be created.
type Schema struct {
	RecordSize int          // Fixed record size in bytes
	KeyOffset  int          // Primary key offset within record
	KeyLength  int          // Primary key length in bytes
	Fields     []SchemaField // Field definitions (informational)
}

// SchemaField describes a field in the new file's schema.
type SchemaField struct {
	Name     string
	Offset   int
	Length   int
	Type     FieldType
	Decimals int
	IsKey    bool
}

// NewSchema creates a new file schema with the given record size.
func NewSchema(recordSize int) *Schema {
	return &Schema{
		RecordSize: recordSize,
	}
}

// KeyField adds the primary key field to the schema.
// This is mandatory — every ISAM file needs a primary key.
func (s *Schema) KeyField(name string, offset, length int) *Schema {
	s.KeyOffset = offset
	s.KeyLength = length
	s.Fields = append(s.Fields, SchemaField{
		Name: name, Offset: offset, Length: length, Type: FieldString, IsKey: true,
	})
	return s
}

// StringField adds a string field definition.
func (s *Schema) StringField(name string, offset, length int) *Schema {
	s.Fields = append(s.Fields, SchemaField{
		Name: name, Offset: offset, Length: length, Type: FieldString,
	})
	return s
}

// IntField adds an integer field definition.
func (s *Schema) IntField(name string, offset, length int) *Schema {
	s.Fields = append(s.Fields, SchemaField{
		Name: name, Offset: offset, Length: length, Type: FieldInt,
	})
	return s
}

// DateField adds a date field (YYYYMMDD).
func (s *Schema) DateField(name string, offset, length int) *Schema {
	s.Fields = append(s.Fields, SchemaField{
		Name: name, Offset: offset, Length: length, Type: FieldDate,
	})
	return s
}

// BCDField adds a packed decimal field.
func (s *Schema) BCDField(name string, offset, length, decimals int) *Schema {
	s.Fields = append(s.Fields, SchemaField{
		Name: name, Offset: offset, Length: length, Type: FieldBCD, Decimals: decimals,
	})
	return s
}

// Validate checks the schema for consistency.
func (s *Schema) Validate() error {
	if s.RecordSize <= 0 || s.RecordSize > 65535 {
		return fmt.Errorf("record size must be 1-65535, got %d", s.RecordSize)
	}
	if s.KeyLength <= 0 {
		return fmt.Errorf("key field is required (call KeyField)")
	}
	if s.KeyOffset+s.KeyLength > s.RecordSize {
		return fmt.Errorf("key field (offset %d + length %d = %d) exceeds record size %d",
			s.KeyOffset, s.KeyLength, s.KeyOffset+s.KeyLength, s.RecordSize)
	}
	// Validate all fields fit within record
	for _, f := range s.Fields {
		if f.Offset+f.Length > s.RecordSize {
			return fmt.Errorf("field %q (offset %d + length %d = %d) exceeds record size %d",
				f.Name, f.Offset, f.Length, f.Offset+f.Length, s.RecordSize)
		}
	}
	return nil
}

// ToTable creates an ORM Table from this schema, ready for CRUD operations.
func (s *Schema) ToTable(name, path string) *Table {
	t := NewTable(name, path, s.RecordSize)
	for _, f := range s.Fields {
		switch {
		case f.IsKey:
			t.Key(f.Name, f.Offset, f.Length)
		case f.Type == FieldBCD:
			t.BCD(f.Name, f.Offset, f.Length, f.Decimals)
		case f.Type == FieldInt:
			t.Int(f.Name, f.Offset, f.Length)
		case f.Type == FieldFloat:
			t.Float(f.Name, f.Offset, f.Length)
		case f.Type == FieldDate:
			t.Date(f.Name, f.Offset, f.Length)
		default:
			t.String(f.Name, f.Offset, f.Length)
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// CreateFile generates a new ISAM file on disk.
//
// File format: Micro Focus IDXFORMAT 8 (0x33FE magic)
//   - 128-byte header
//   - Initial B-tree root node (empty leaf)
//   - Ready for InsertRecord() to populate
//
// Will NOT overwrite an existing file — returns error if file exists.
// ---------------------------------------------------------------------------
func CreateFile(path string, schema *Schema) error {
	if err := schema.Validate(); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	// Safety: don't overwrite existing files
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s (use CreateFileForce to overwrite)", path)
	}

	fileBytes := buildISAMFile(schema)

	if err := os.WriteFile(path, fileBytes, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// CreateFileForce creates a new ISAM file, overwriting if it exists.
// Use with caution — this destroys existing data.
func CreateFileForce(path string, schema *Schema) error {
	if err := schema.Validate(); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	// Backup existing file if present
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".bak"
		if data, err := os.ReadFile(path); err == nil {
			os.WriteFile(backupPath, data, 0644)
		}
	}

	fileBytes := buildISAMFile(schema)

	if err := os.WriteFile(path, fileBytes, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// buildISAMFile constructs the raw bytes for a new ISAM file.
func buildISAMFile(schema *Schema) []byte {
	recSize := schema.RecordSize
	keyLen := schema.KeyLength
	alignment := 8 // IDXFORMAT 8

	// Calculate B-tree node data length
	// Standard: use 1022 for small keys, or calculate based on key length
	// Entry = keyLen + 6 (pointer)
	// nodeDataLen must hold at least 4 entries for a viable B-tree
	entrySize := keyLen + 6
	minEntries := 4
	nodeDataLen := 2 + (entrySize * minEntries) // 2 bytes header + entries

	// Round up to common sizes (multiples of entrySize that fit well)
	// Try to use a reasonable node size (512-2048 range)
	for nodeDataLen < 512 {
		nodeDataLen += entrySize
	}
	// Ensure (nodeDataLen - 2) is divisible by entrySize for clean packing
	remainder := (nodeDataLen - 2) % entrySize
	if remainder != 0 {
		nodeDataLen += entrySize - remainder
	}

	// --- Build header (128 bytes) ---
	header := make([]byte, 128)

	// Magic: 0x33FE for IDXFORMAT 8 indexed
	header[0] = 0x33
	header[1] = 0xFE
	header[2] = 0x00
	header[3] = 0x00

	// DB sequence (bytes 4-5): start at 1
	binary.BigEndian.PutUint16(header[4:6], 1)

	// Integrity flag (bytes 6-7): 0 = OK
	binary.BigEndian.PutUint16(header[6:8], 0)

	// Creation timestamp (bytes 8-21): YYMMDDHHMMSScc (14 chars)
	now := time.Now()
	ts := now.Format("060102150405") + "00"
	copy(header[8:22], ts)

	// Modified timestamp (bytes 22-35): same as creation
	copy(header[22:36], ts)

	// Organization (byte 39): 2 = Indexed
	header[39] = 2

	// Compression (byte 41): 0 = None
	header[41] = 0

	// IdxFormat (byte 43): 8
	header[43] = 8

	// Record mode (byte 48): 0 = Fixed
	header[48] = 0

	// MaxRecordLen (bytes 54-57): big-endian 32-bit
	binary.BigEndian.PutUint32(header[54:58], uint32(recSize))

	// MinRecordLen (bytes 58-61): same as max for fixed
	binary.BigEndian.PutUint32(header[58:62], uint32(recSize))

	// Also write recSize at 0x38 (2 bytes) for 0x33FE format compatibility
	binary.BigEndian.PutUint16(header[0x38:0x3A], uint16(recSize))

	// Handler version (bytes 108-111)
	binary.BigEndian.PutUint32(header[108:112], 90) // MF COBOL Server 9.0

	// --- Build initial B-tree root node (empty leaf) ---
	// B-tree nodes use type 3 (DELETED) marker — this is how MF stores index nodes
	nodeMarkerSize := 2 // RecHeaderSize for non-long records

	// Align header to 8 bytes
	nodeStart := 128
	if nodeStart%alignment != 0 {
		nodeStart += alignment - (nodeStart % alignment)
	}

	// Marker: type=DELETED(3), dataLen=nodeDataLen
	nodeMarker := (uint16(RecTypeDeleted) << 12) | (uint16(nodeDataLen) & 0x0FFF)

	// Node data: first 2 bytes = header (0x80 = leaf flag, 0x00 = 0 entries)
	nodeData := make([]byte, nodeDataLen)
	nodeData[0] = 0x80 // leaf flag set, 0 entries
	nodeData[1] = 0x00 // 0 entries

	// Calculate total file size (header + padding + node marker + node data)
	totalSize := nodeStart + nodeMarkerSize + nodeDataLen

	// Align total to 8 bytes
	if totalSize%alignment != 0 {
		totalSize += alignment - (totalSize % alignment)
	}

	// Update LogicalEnd in header (bytes 120-127)
	binary.BigEndian.PutUint64(header[120:128], uint64(totalSize))

	// --- Assemble file ---
	fileBytes := make([]byte, totalSize)

	// Write header
	copy(fileBytes[0:128], header)

	// Write B-tree root node
	binary.BigEndian.PutUint16(fileBytes[nodeStart:nodeStart+2], nodeMarker)
	copy(fileBytes[nodeStart+nodeMarkerSize:], nodeData)

	return fileBytes
}

// ---------------------------------------------------------------------------
// CreateModel creates a new ISAM file AND returns a connected Model.
//
// Usage:
//
//	schema := isam.NewSchema(256).
//	    KeyField("codigo", 0, 5).
//	    StringField("nombre", 5, 40)
//
//	model, err := isam.CreateModel("my_table", `/tmp/MYFILE`, schema)
//	if err != nil { ... }
//
//	rec := model.New()
//	rec.Set("codigo", "00001")
//	rec.Set("nombre", "TEST")
//	rec.Save()
//
// ---------------------------------------------------------------------------
func CreateModel(name, path string, schema *Schema) (*Table, error) {
	if err := CreateFile(path, schema); err != nil {
		return nil, err
	}
	return schema.ToTable(name, path), nil
}

// CreateAndPopulate creates a new ISAM file and inserts initial records.
// Each record is a map[string]string of field name → value.
func CreateAndPopulate(path string, schema *Schema, records []map[string]string) (*Table, error) {
	table, err := CreateModel("temp", path, schema)
	if err != nil {
		return nil, err
	}

	table.SafeMode = false // no need for safety checks on brand new file

	for i, recMap := range records {
		row := table.New()
		for field, value := range recMap {
			row.Set(field, value)
		}
		if _, err := row.Save(); err != nil {
			return nil, fmt.Errorf("insert record %d: %w", i, err)
		}
	}

	return table, nil
}
