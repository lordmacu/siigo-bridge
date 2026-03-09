package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"siigo-common/isam"
	_ "siigo-common/storage" // registers modernc sqlite driver
)

const (
	defaultDataPath         = `C:\DEMOS01`
	defaultDBPath           = "isam_v2.db"
	defaultReportPath       = "isam_v2_report.json"
	defaultSamplePerFile    = 250
	defaultGroupSampleCap   = 1200
	defaultMaxAnalyzedBytes = 4096
	defaultMaxFields        = 180
	defaultReadMode         = "auto"
)

const (
	readModeAuto     = "auto"
	readModeBinary   = "binary"
	readModePhysical = "physical"
)

const (
	recTypeNull             = 0
	recTypeSystem           = 1
	recTypePointer          = 2
	recTypeDeleted          = 3
	recTypeNormal           = 4
	recTypeReduced          = 5
	recTypePointerRefer     = 6
	recTypeReferenced       = 7
	recTypeReducedReference = 8
)

type byteClass int

const (
	classEmpty byteClass = iota
	classDigit
	classAlpha
	classAlnum
	classBinary
	classMixed
	classPacked
)

type posStat struct {
	nullCount   int
	spaceCount  int
	digitCount  int
	alphaCount  int
	punctCount  int
	binaryCount int
	values      [256]int
}

type FieldDef struct {
	Name       string   `json:"name"`
	Offset     int      `json:"offset"`
	Length     int      `json:"length"`
	Type       string   `json:"type"`
	Confidence float64  `json:"confidence"`
	Distinct   int      `json:"distinct_values"`
	NullRate   float64  `json:"null_rate"`
	Samples    []string `json:"samples,omitempty"`
	Exported   bool     `json:"exported"`
}

type FileReport struct {
	FileName       string              `json:"file_name"`
	TableName      string              `json:"table_name"`
	GroupKey       string              `json:"group_key"`
	ReadMode       string              `json:"read_mode"`
	RecordSize     int                 `json:"record_size"`
	TotalRecords   int                 `json:"total_records"`
	UsableRecords  int                 `json:"usable_records"`
	ExportedRows   int                 `json:"exported_rows"`
	AnalyzedFields int                 `json:"analyzed_fields"`
	ExportedFields int                 `json:"exported_fields"`
	Physical       *PhysicalInfoReport `json:"physical,omitempty"`
	Fields         []FieldDef          `json:"fields"`
}

type RunReport struct {
	GeneratedAt string       `json:"generated_at"`
	DataPath    string       `json:"data_path"`
	Database    string       `json:"database"`
	TotalFiles  int          `json:"total_files"`
	Processed   int          `json:"processed"`
	Files       []FileReport `json:"files"`
}

type fileMeta struct {
	Name         string
	Path         string
	GroupKey     string
	ReadMode     string
	RecordSize   int
	TotalRecords int
	Usable       int
	Physical     *physicalInfo
}

type groupBucket struct {
	Key     string
	RecSize int
	Samples [][]byte
}

type loadOptions struct {
	mode              string
	includeDeleted    bool
	includeReduced    bool
	includeSystem     bool
	includePointer    bool
	recoveryScan      bool
	maxRecoveryShifts int
}

type loadResult struct {
	ModeUsed   string
	RecordSize int
	Records    []isam.Record
	Physical   *physicalInfo
}

type physicalInfo struct {
	Format        string
	LongRecords   bool
	Organization  int
	RecordingMode int
	IndexType     int
	Alignment     int
	HeaderSize    int
	RecordHdrSize int
	ParsedBlocks  int
	Included      int
	Recoveries    int
	Skipped       int
	TypeCounts    map[int]int
}

type PhysicalInfoReport struct {
	Format        string         `json:"format"`
	LongRecords   bool           `json:"long_records"`
	Organization  int            `json:"organization"`
	RecordingMode int            `json:"recording_mode"`
	IndexType     int            `json:"index_type"`
	Alignment     int            `json:"alignment"`
	HeaderSize    int            `json:"header_size"`
	RecordHdrSize int            `json:"record_header_size"`
	ParsedBlocks  int            `json:"parsed_blocks"`
	Included      int            `json:"included_blocks"`
	Recoveries    int            `json:"recoveries"`
	Skipped       int            `json:"skipped_blocks"`
	TypeCounts    map[string]int `json:"type_counts"`
}

func main() {
	var (
		dataPath      string
		dbPath        string
		reportPath    string
		include       string
		maxFiles      int
		samplePerFile int
		maxFields     int
		readMode      string
		incDeleted    bool
		incReduced    bool
		incSystem     bool
		incPointer    bool
		noRecovery    bool
	)

	flag.StringVar(&dataPath, "data", defaultDataPath, "ISAM file directory")
	flag.StringVar(&dbPath, "db", defaultDBPath, "Ruta de salida SQLite")
	flag.StringVar(&reportPath, "report", defaultReportPath, "Ruta de reporte JSON")
	flag.StringVar(&include, "include", "*", "Patrones separados por coma (ej: Z17,Z03*,C03)")
	flag.IntVar(&maxFiles, "max-files", 0, "Max files to process (0 = all)")
	flag.IntVar(&samplePerFile, "sample", defaultSamplePerFile, "Max samples per file for inference")
	flag.IntVar(&maxFields, "max-fields", defaultMaxFields, "Maximo de campos inferidos por esquema")
	flag.StringVar(&readMode, "mode", defaultReadMode, "Modo de lectura: auto|binary|physical")
	flag.BoolVar(&incDeleted, "physical-include-deleted", false, "Incluir bloques deleted (tipo 3) en modo physical")
	flag.BoolVar(&incReduced, "physical-include-reduced", false, "Incluir bloques reduced/reduced_referenced (tipos 5/8) en modo physical")
	flag.BoolVar(&incSystem, "physical-include-system", false, "Incluir bloques system (tipo 1) en modo physical")
	flag.BoolVar(&incPointer, "physical-include-pointer", false, "Incluir bloques pointer/pointer_referenced (tipos 2/6) en modo physical")
	flag.BoolVar(&noRecovery, "physical-no-recovery", false, "Desactivar re-sincronizacion por desplazamiento en modo physical")
	flag.Parse()

	if samplePerFile < 50 {
		samplePerFile = 50
	}
	if maxFields < 20 {
		maxFields = 20
	}
	readMode = strings.ToLower(strings.TrimSpace(readMode))
	if readMode != readModeAuto && readMode != readModeBinary && readMode != readModePhysical {
		log.Fatalf("modo invalido %q (usa auto|binary|physical)", readMode)
	}

	loadCfg := loadOptions{
		mode:              readMode,
		includeDeleted:    incDeleted,
		includeReduced:    incReduced,
		includeSystem:     incSystem,
		includePointer:    incPointer,
		recoveryScan:      !noRecovery,
		maxRecoveryShifts: 16,
	}

	patterns := parsePatterns(include)
	files, err := listIsamFiles(dataPath, patterns, maxFiles)
	if err != nil {
		log.Fatalf("listing files: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("no ISAM files found in %s", dataPath)
	}

	metas := make([]fileMeta, 0, len(files))
	groups := map[string]*groupBucket{}

	log.Printf("[v2] Initial scan: %d candidate files", len(files))

	for idx, p := range files {
		name := filepath.Base(p)
		loaded, err := loadRecords(p, loadCfg)
		if err != nil {
			log.Printf("[v2] [%d/%d] %s: skip (%v)", idx+1, len(files), name, err)
			continue
		}
		if len(loaded.Records) == 0 || loaded.RecordSize <= 0 {
			log.Printf("[v2] [%d/%d] %s: skip (sin registros)", idx+1, len(files), name)
			continue
		}

		usable := filterUsableRecords(loaded.Records, loaded.RecordSize)
		if len(usable) == 0 {
			log.Printf("[v2] [%d/%d] %s: skip (registros no usables)", idx+1, len(files), name)
			continue
		}

		groupKey := schemaGroupKey(name, loaded.RecordSize)
		meta := fileMeta{
			Name:         name,
			Path:         p,
			GroupKey:     groupKey,
			ReadMode:     loaded.ModeUsed,
			RecordSize:   loaded.RecordSize,
			TotalRecords: len(loaded.Records),
			Usable:       len(usable),
			Physical:     loaded.Physical,
		}
		metas = append(metas, meta)

		b := groups[groupKey]
		if b == nil {
			b = &groupBucket{Key: groupKey, RecSize: loaded.RecordSize}
			groups[groupKey] = b
		}
		addSampleRecords(&b.Samples, usable, samplePerFile, defaultGroupSampleCap)

		log.Printf("[v2] [%d/%d] %s: mode=%s recSize=%d usable=%d/%d group=%s",
			idx+1, len(files), name, loaded.ModeUsed, loaded.RecordSize, len(usable), len(loaded.Records), groupKey)
	}

	if len(metas) == 0 {
		log.Fatalf("no processable files remaining")
	}

	groupSchemas := map[string][]FieldDef{}
	for key, b := range groups {
		groupSchemas[key] = inferFields(b.Samples, b.RecSize, maxFields)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		log.Fatalf("abriendo sqlite: %v", err)
	}
	defer db.Close()

	if err := ensureMetadataTables(db); err != nil {
		log.Fatalf("creando metadata tables: %v", err)
	}

	reports := make([]FileReport, 0, len(metas))
	for idx, meta := range metas {
		fields := cloneFields(groupSchemas[meta.GroupKey])
		exportedFields := chooseExportFields(fields)

		loaded, err := loadRecords(meta.Path, loadCfg)
		if err != nil {
			log.Printf("[v2] [%d/%d] %s: error releyendo (%v)", idx+1, len(metas), meta.Name, err)
			continue
		}
		usable := filterUsableRecords(loaded.Records, loaded.RecordSize)
		tableName := sqliteTableName(meta.Name)
		inserted, err := exportFileToSQLite(db, tableName, usable, exportedFields)
		if err != nil {
			log.Printf("[v2] [%d/%d] %s: error exportando sqlite (%v)", idx+1, len(metas), meta.Name, err)
			continue
		}

		if err := writeMetadata(db, meta, tableName, fields, exportedFields, inserted); err != nil {
			log.Printf("[v2] [%d/%d] %s: warning metadata (%v)", idx+1, len(metas), meta.Name, err)
		}

		reports = append(reports, FileReport{
			FileName:       meta.Name,
			TableName:      tableName,
			GroupKey:       meta.GroupKey,
			ReadMode:       loaded.ModeUsed,
			RecordSize:     meta.RecordSize,
			TotalRecords:   meta.TotalRecords,
			UsableRecords:  len(usable),
			ExportedRows:   inserted,
			AnalyzedFields: len(fields),
			ExportedFields: len(exportedFields),
			Physical:       toPhysicalInfoReport(loaded.Physical),
			Fields:         fields,
		})

		log.Printf("[v2] [%d/%d] %s -> %s rows=%d fields=%d",
			idx+1, len(metas), meta.Name, tableName, inserted, len(exportedFields))
	}

	sort.Slice(reports, func(i, j int) bool { return reports[i].FileName < reports[j].FileName })

	final := RunReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DataPath:    dataPath,
		Database:    dbPath,
		TotalFiles:  len(files),
		Processed:   len(reports),
		Files:       reports,
	}

	data, err := json.MarshalIndent(final, "", "  ")
	if err != nil {
		log.Fatalf("serializando reporte: %v", err)
	}
	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		log.Fatalf("guardando reporte: %v", err)
	}

	log.Printf("[v2] OK: %d files processed -> DB=%s | Reporte=%s",
		len(reports), dbPath, reportPath)
}

func parsePatterns(raw string) []string {
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func listIsamFiles(dataPath string, patterns []string, maxFiles int) ([]string, error) {
	entries, err := os.ReadDir(dataPath)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToUpper(e.Name())
		if filepath.Ext(name) != "" {
			continue
		}
		if !looksLikeIsamName(name) {
			continue
		}
		if !matchesAny(name, patterns) {
			continue
		}
		out = append(out, filepath.Join(dataPath, e.Name()))
	}
	sort.Strings(out)
	if maxFiles > 0 && len(out) > maxFiles {
		out = out[:maxFiles]
	}
	return out, nil
}

func loadRecords(path string, opts loadOptions) (*loadResult, error) {
	switch opts.mode {
	case readModeBinary:
		return loadBinary(path)
	case readModePhysical:
		return loadPhysical(path, opts)
	case readModeAuto:
		p, err := loadPhysical(path, opts)
		if err == nil && len(p.Records) > 0 {
			return p, nil
		}
		b, berr := loadBinary(path)
		if berr == nil {
			return b, nil
		}
		if err != nil {
			return nil, fmt.Errorf("physical: %v | binary: %v", err, berr)
		}
		return nil, berr
	default:
		return nil, fmt.Errorf("modo no soportado: %s", opts.mode)
	}
}

func loadBinary(path string) (*loadResult, error) {
	info, err := isam.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &loadResult{
		ModeUsed:   readModeBinary,
		RecordSize: info.RecordSize,
		Records:    info.Records,
	}, nil
}

func loadPhysical(path string, opts loadOptions) (*loadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	if len(data) < 128 {
		return nil, fmt.Errorf("file too small for physical parse (%d bytes)", len(data))
	}

	if hasMFDataSignature(data) {
		return loadPhysicalMF(data, opts)
	}
	if hasIndexedHeaderSignature(data) {
		return loadPhysicalIndexed(data, opts)
	}
	return nil, fmt.Errorf("firma de cabecera no reconocida para modo physical")
}

func hasMFDataSignature(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if data[0] == 0x30 && data[1] == 0x7E && data[2] == 0x00 && data[3] == 0x00 {
		return true
	}
	if data[0] == 0x30 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x7C {
		return true
	}
	return false
}

func hasIndexedHeaderSignature(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	magic := binary.BigEndian.Uint16(data[:2])
	if magic == 0x33FE {
		return true
	}
	return (magic & 0xFF00) == 0x3000
}

func loadPhysicalMF(data []byte, opts loadOptions) (*loadResult, error) {

	info, err := parsePhysicalHeader(data[:128])
	if err != nil {
		return nil, err
	}

	includeTypes := map[int]bool{
		recTypeNormal:     true,
		recTypeReferenced: true,
	}
	if opts.includeDeleted {
		includeTypes[recTypeDeleted] = true
	}
	if opts.includeReduced {
		includeTypes[recTypeReduced] = true
		includeTypes[recTypeReducedReference] = true
	}
	if opts.includeSystem {
		includeTypes[recTypeSystem] = true
	}
	if opts.includePointer {
		includeTypes[recTypePointer] = true
		includeTypes[recTypePointerRefer] = true
	}

	pos := info.HeaderSize
	records := make([]isam.Record, 0, 1024)
	typeCounts := map[int]int{}
	parsed := 0
	recoveries := 0
	skipped := 0
	consecutiveInvalid := 0

	for pos+info.RecordHdrSize <= len(data) {
		recType, dataLen, ok := decodePhysicalRecordHeader(data[pos:], info.RecordHdrSize, info.LongRecords)
		if !ok || recType < 0 || recType > recTypeReducedReference {
			if opts.recoveryScan && opts.maxRecoveryShifts > 0 && consecutiveInvalid < opts.maxRecoveryShifts {
				pos++
				recoveries++
				consecutiveInvalid++
				continue
			}
			break
		}

		payloadStart := pos + info.RecordHdrSize
		payloadEnd := payloadStart + dataLen
		if dataLen < 0 || payloadEnd > len(data) {
			if opts.recoveryScan && opts.maxRecoveryShifts > 0 && consecutiveInvalid < opts.maxRecoveryShifts {
				pos++
				recoveries++
				consecutiveInvalid++
				continue
			}
			break
		}
		consecutiveInvalid = 0

		typeCounts[recType]++
		parsed++

		if includeTypes[recType] {
			recData := make([]byte, dataLen)
			copy(recData, data[payloadStart:payloadEnd])
			records = append(records, isam.Record{
				Data:   recData,
				Offset: pos,
			})
		} else {
			skipped++
		}

		pos = payloadEnd
		if info.Alignment > 1 {
			pad := (info.Alignment - ((dataLen + info.RecordHdrSize) % info.Alignment)) % info.Alignment
			pos += pad
		}
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("physical parser no encontró registros de datos incluidos")
	}

	recSize := info.MaxRecordLength
	if recSize <= 0 {
		recSize = maxObservedLength(records)
	}
	if recSize <= 0 {
		recSize = maxObservedLength(records)
	}

	return &loadResult{
		ModeUsed:   readModePhysical,
		RecordSize: recSize,
		Records:    records,
		Physical: &physicalInfo{
			Format:        "mf_stream",
			LongRecords:   info.LongRecords,
			Organization:  info.Organization,
			RecordingMode: info.RecordingMode,
			IndexType:     info.IndexType,
			Alignment:     info.Alignment,
			HeaderSize:    info.HeaderSize,
			RecordHdrSize: info.RecordHdrSize,
			ParsedBlocks:  parsed,
			Included:      len(records),
			Recoveries:    recoveries,
			Skipped:       skipped,
			TypeCounts:    typeCounts,
		},
	}, nil
}

func loadPhysicalIndexed(data []byte, opts loadOptions) (*loadResult, error) {
	if len(data) < 0x44 {
		return nil, fmt.Errorf("header indexado incompleto")
	}

	recSize := int(binary.BigEndian.Uint16(data[0x38:0x3A]))
	if recSize <= 0 || recSize > 65535 {
		return nil, fmt.Errorf("record_size invalido en header indexado: %d", recSize)
	}
	if recSize < 8 {
		return nil, fmt.Errorf("record_size demasiado pequeno para parseo indexado: %d", recSize)
	}
	organization := int(binary.BigEndian.Uint16(data[0x26:0x28]))
	indexType := int(data[0x2B])
	start := 0x800
	if len(data) <= start+2 {
		return nil, fmt.Errorf("indexed file without data zone (len=%d)", len(data))
	}

	validStatus := map[int]bool{0: true, 1: true, 2: true, 4: true, 6: true, 8: true, 10: true, 12: true, 14: true}
	pos := start
	records := make([]isam.Record, 0, 1024)
	typeCounts := map[int]int{}
	parsed := 0
	recoveries := 0
	skipped := 0

	for pos+2 <= len(data) {
		b0 := data[pos]
		b1 := data[pos+1]
		status := int((b0 & 0xF0) >> 4)
		length := int((int(b0&0x0F) << 8) | int(b1))

		if !validStatus[status] || length <= 0 {
			pos++
			recoveries++
			continue
		}
		if !opts.includeReduced && length != recSize {
			pos++
			recoveries++
			continue
		}
		if opts.includeReduced && length > recSize {
			pos++
			recoveries++
			continue
		}

		payloadStart := pos + 2
		payloadEnd := payloadStart + length
		if payloadEnd > len(data) {
			pos++
			recoveries++
			continue
		}

		typeCounts[status]++
		parsed++

		include := true
		if status == 1 && !opts.includeDeleted {
			include = false
		}
		if status == 0 && !opts.includeSystem && !opts.includePointer {
			include = false
		}

		if include {
			recData := make([]byte, length)
			copy(recData, data[payloadStart:payloadEnd])
			records = append(records, isam.Record{
				Data:   recData,
				Offset: pos,
			})
		} else {
			skipped++
		}

		pos = payloadEnd
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("physical/indexed no produjo registros incluidos")
	}

	return &loadResult{
		ModeUsed:   readModePhysical,
		RecordSize: recSize,
		Records:    records,
		Physical: &physicalInfo{
			Format:        "indexed_marker",
			LongRecords:   false,
			Organization:  organization,
			RecordingMode: 0,
			IndexType:     indexType,
			Alignment:     0,
			HeaderSize:    start,
			RecordHdrSize: 2,
			ParsedBlocks:  parsed,
			Included:      len(records),
			Recoveries:    recoveries,
			Skipped:       skipped,
			TypeCounts:    typeCounts,
		},
	}, nil
}

type physicalHeader struct {
	LongRecords     bool
	Organization    int
	RecordingMode   int
	IndexType       int
	MaxRecordLength int
	MinRecordLength int
	Alignment       int
	HeaderSize      int
	RecordHdrSize   int
}

func parsePhysicalHeader(hdr []byte) (*physicalHeader, error) {
	if len(hdr) < 128 {
		return nil, fmt.Errorf("header menor a 128 bytes")
	}

	ph := &physicalHeader{
		HeaderSize: 128,
	}

	switch {
	case hdr[0] == 0x30 && hdr[1] == 0x7E && hdr[2] == 0x00 && hdr[3] == 0x00:
		ph.LongRecords = false
	case hdr[0] == 0x30 && hdr[1] == 0x00 && hdr[2] == 0x00 && hdr[3] == 0x7C:
		ph.LongRecords = true
	default:
		return nil, fmt.Errorf("header no coincide con firma esperada de indexed data file")
	}

	ph.Organization = int(hdr[39])
	ph.IndexType = int(hdr[43])
	ph.RecordingMode = int(hdr[48])
	ph.MaxRecordLength = int(binary.BigEndian.Uint32(hdr[54:58]))
	ph.MinRecordLength = int(binary.BigEndian.Uint32(hdr[58:62]))
	ph.RecordHdrSize = 2
	if ph.LongRecords {
		ph.RecordHdrSize = 4
	}

	switch ph.Organization {
	case 1:
		if ph.RecordingMode == 1 {
			ph.Alignment = 4
		}
	case 2:
		switch ph.IndexType {
		case 1, 2:
			ph.Alignment = 1
		case 3, 4:
			ph.Alignment = 4
		case 8:
			ph.Alignment = 8
		default:
			ph.Alignment = 4
		}
	}

	return ph, nil
}

func decodePhysicalRecordHeader(data []byte, hdrSize int, longRecords bool) (recType int, dataLen int, ok bool) {
	if hdrSize == 4 {
		if len(data) < 4 {
			return 0, 0, false
		}
		h := binary.BigEndian.Uint32(data[:4])
		recType = int((h >> 28) & 0x0F)
		dataLen = int(h & 0x0FFFFFFF)
		if dataLen < 0 || dataLen > 1<<24 {
			return 0, 0, false
		}
		return recType, dataLen, true
	}

	if len(data) < 2 {
		return 0, 0, false
	}
	h := binary.BigEndian.Uint16(data[:2])
	recType = int((h >> 12) & 0x0F)
	dataLen = int(h & 0x0FFF)
	if !longRecords && dataLen > 4095 {
		return 0, 0, false
	}
	return recType, dataLen, true
}

func maxObservedLength(records []isam.Record) int {
	maxLen := 0
	for _, r := range records {
		if len(r.Data) > maxLen {
			maxLen = len(r.Data)
		}
	}
	return maxLen
}

func toPhysicalInfoReport(p *physicalInfo) *PhysicalInfoReport {
	if p == nil {
		return nil
	}
	counts := map[string]int{}
	for t, n := range p.TypeCounts {
		counts[physicalTypeName(p.Format, t)] = n
	}
	return &PhysicalInfoReport{
		Format:        p.Format,
		LongRecords:   p.LongRecords,
		Organization:  p.Organization,
		RecordingMode: p.RecordingMode,
		IndexType:     p.IndexType,
		Alignment:     p.Alignment,
		HeaderSize:    p.HeaderSize,
		RecordHdrSize: p.RecordHdrSize,
		ParsedBlocks:  p.ParsedBlocks,
		Included:      p.Included,
		Recoveries:    p.Recoveries,
		Skipped:       p.Skipped,
		TypeCounts:    counts,
	}
}

func physicalTypeName(format string, t int) string {
	if format == "indexed_marker" {
		switch t {
		case 0:
			return "status_0"
		case 1:
			return "status_1_deleted"
		case 2:
			return "status_2"
		case 4:
			return "status_4_active"
		case 6:
			return "status_6"
		case 8:
			return "status_8"
		case 10:
			return "status_a"
		case 12:
			return "status_c"
		case 14:
			return "status_e"
		default:
			return fmt.Sprintf("status_%X", t)
		}
	}

	switch t {
	case recTypeNull:
		return "null"
	case recTypeSystem:
		return "system"
	case recTypePointer:
		return "pointer"
	case recTypeDeleted:
		return "deleted"
	case recTypeNormal:
		return "normal"
	case recTypeReduced:
		return "reduced"
	case recTypePointerRefer:
		return "pointer_referenced"
	case recTypeReferenced:
		return "referenced"
	case recTypeReducedReference:
		return "reduced_referenced"
	default:
		return fmt.Sprintf("type_%d", t)
	}
}

func looksLikeIsamName(name string) bool {
	if name == "" {
		return false
	}
	// Archivos de datos de Siigo suelen ser Z*, C*, N*, INF*, CHGFM*
	if strings.HasPrefix(name, "Z") || strings.HasPrefix(name, "C") || strings.HasPrefix(name, "N") {
		return true
	}
	return strings.HasPrefix(name, "INF") || strings.HasPrefix(name, "CHGFM")
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		ok, _ := filepath.Match(p, strings.ToUpper(name))
		if ok {
			return true
		}
	}
	return false
}

func schemaGroupKey(fileName string, recSize int) string {
	base := stripYearSuffix(strings.ToUpper(fileName))
	return fmt.Sprintf("%s|%d", base, recSize)
}

func stripYearSuffix(name string) string {
	// Z032016 -> Z03, Z082016A -> Z08A, Z0620171114 -> Z06
	for i := 2; i+4 <= len(name); i++ {
		s := name[i:]
		if len(s) < 4 {
			continue
		}
		if !(strings.HasPrefix(s, "19") || strings.HasPrefix(s, "20")) {
			continue
		}
		if !isAllDigits(s[:4]) {
			continue
		}
		base := name[:i]
		rest := name[i+4:]
		for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			rest = rest[1:]
		}
		return base + rest
	}
	return name
}

func filterUsableRecords(records []isam.Record, recSize int) []isam.Record {
	out := make([]isam.Record, 0, len(records))
	for _, r := range records {
		if recSize <= 0 || len(r.Data) == 0 {
			continue
		}
		if isLikelyUsable(r.Data, recSize) {
			out = append(out, r)
		}
	}
	return out
}

func isLikelyUsable(rec []byte, recSize int) bool {
	if len(rec) == 0 {
		return false
	}
	limit := recSize
	if limit > len(rec) {
		limit = len(rec)
	}
	if limit <= 0 {
		return false
	}

	nonEmpty := 0
	printable := 0
	for i := 0; i < limit; i++ {
		b := rec[i]
		if b != 0 && b != ' ' {
			nonEmpty++
		}
		if (b >= 0x20 && b <= 0x7E) || b >= 0xA0 {
			printable++
		}
	}

	nonEmptyRate := float64(nonEmpty) / float64(limit)
	printableRate := float64(printable) / float64(limit)
	if nonEmptyRate < 0.02 {
		return false
	}
	if printableRate < 0.12 {
		return false
	}

	head := minInt(limit, 160)
	headPrintable := 0
	headControl := 0
	headZero := 0
	for i := 0; i < head; i++ {
		b := rec[i]
		if (b >= 0x20 && b <= 0x7E) || b >= 0xA0 {
			headPrintable++
		}
		if b < 0x20 && b != 0 && b != '\t' && b != '\n' && b != '\r' {
			headControl++
		}
		if i < 8 && b == 0 {
			headZero++
		}
	}
	if head > 0 {
		headPrintableRate := float64(headPrintable) / float64(head)
		if headPrintableRate < 0.35 {
			return false
		}
	}
	if headControl > head/6 {
		return false
	}
	if headZero >= 6 {
		return false
	}
	return true
}

func addSampleRecords(dst *[][]byte, records []isam.Record, maxPerFile, capTotal int) {
	if maxPerFile <= 0 {
		maxPerFile = 1
	}
	step := 1
	if len(records) > maxPerFile {
		step = int(math.Ceil(float64(len(records)) / float64(maxPerFile)))
	}
	for i := 0; i < len(records); i += step {
		if len(*dst) >= capTotal {
			return
		}
		recCopy := make([]byte, len(records[i].Data))
		copy(recCopy, records[i].Data)
		*dst = append(*dst, recCopy)
	}
}

func inferFields(samples [][]byte, recSize, maxFields int) []FieldDef {
	if len(samples) == 0 || recSize <= 0 {
		return nil
	}

	analyzedBytes := recSize
	if analyzedBytes > defaultMaxAnalyzedBytes {
		analyzedBytes = defaultMaxAnalyzedBytes
	}

	stats := make([]posStat, analyzedBytes)
	total := float64(len(samples))

	for _, rec := range samples {
		limit := analyzedBytes
		if limit > len(rec) {
			limit = len(rec)
		}
		for i := 0; i < limit; i++ {
			b := rec[i]
			s := &stats[i]
			s.values[b]++

			switch {
			case b == 0:
				s.nullCount++
			case b == ' ':
				s.spaceCount++
			case b >= '0' && b <= '9':
				s.digitCount++
			case (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b >= 0xA0:
				s.alphaCount++
			case b == '.' || b == ',' || b == '-' || b == '/' || b == ':' || b == '_' || b == '(' || b == ')' || b == '&':
				s.punctCount++
			default:
				s.binaryCount++
			}
		}
	}

	lastData := analyzedBytes - 1
	for i := analyzedBytes - 1; i >= 0; i-- {
		empty := float64(stats[i].nullCount+stats[i].spaceCount) / total
		if empty < 0.98 {
			lastData = i
			break
		}
	}
	if lastData+64 < analyzedBytes {
		analyzedBytes = lastData + 64
		if analyzedBytes < 1 {
			analyzedBytes = 1
		}
		stats = stats[:analyzedBytes]
	}

	classes := make([]byteClass, analyzedBytes)
	entropy := make([]float64, analyzedBytes)
	for i := 0; i < analyzedBytes; i++ {
		classes[i] = classifyPosition(stats[i], total)
		entropy[i] = calcEntropy(stats[i].values[:], total)
	}

	boundaries := make([]bool, analyzedBytes+1)
	boundaries[0] = true
	boundaries[analyzedBytes] = true
	for i := 1; i < analyzedBytes; i++ {
		prev := stats[i-1]
		curr := stats[i]
		prevEmpty := float64(prev.nullCount+prev.spaceCount) / total
		currEmpty := float64(curr.nullCount+curr.spaceCount) / total
		prevDigit := float64(prev.digitCount) / total
		currDigit := float64(curr.digitCount) / total

		if classes[i] != classes[i-1] {
			boundaries[i] = true
		}
		if math.Abs(prevEmpty-currEmpty) > 0.50 {
			boundaries[i] = true
		}
		if math.Abs(prevDigit-currDigit) > 0.60 {
			boundaries[i] = true
		}
		if math.Abs(entropy[i]-entropy[i-1]) > 1.6 {
			boundaries[i] = true
		}
	}

	positions := make([]int, 0, analyzedBytes/2)
	for i, b := range boundaries {
		if b {
			positions = append(positions, i)
		}
	}
	if len(positions) < 2 {
		positions = []int{0, analyzedBytes}
	}

	positions = normalizeBoundaries(positions, analyzedBytes)

	fields := make([]FieldDef, 0, len(positions))
	for i := 0; i < len(positions)-1; i++ {
		start := positions[i]
		end := positions[i+1]
		if end <= start {
			continue
		}
		f := buildField(samples, stats, classes, start, end)
		fields = append(fields, f)
	}

	fields = mergeTinyFields(fields, samples)
	if len(fields) > maxFields {
		fields = compressFields(fields, maxFields)
	}

	for i := range fields {
		fields[i].Name = inferFieldName(fields[i].Offset, fields[i].Type)
	}
	return fields
}

func classifyPosition(s posStat, total float64) byteClass {
	empty := float64(s.nullCount+s.spaceCount) / total
	digit := float64(s.digitCount) / total
	alpha := float64(s.alphaCount+s.punctCount) / total
	binary := float64(s.binaryCount) / total

	switch {
	case empty > 0.94:
		return classEmpty
	case binary > 0.55:
		if isPackedCandidate(s, total) {
			return classPacked
		}
		return classBinary
	case digit > 0.85 && alpha < 0.12:
		return classDigit
	case alpha > 0.75 && digit < 0.20:
		return classAlpha
	case alpha > 0.30 && digit > 0.25:
		return classAlnum
	default:
		return classMixed
	}
}

func isPackedCandidate(s posStat, total float64) bool {
	bcd := 0
	for val, cnt := range s.values {
		if cnt == 0 {
			continue
		}
		v := byte(val)
		hi := v >> 4
		lo := v & 0x0F
		isBCD := hi <= 9 && lo <= 9
		isSign := v == 0x0C || v == 0x0D || v == 0x0F
		if isBCD || isSign {
			bcd += cnt
		}
	}
	return float64(bcd)/total > 0.70
}

func calcEntropy(values []int, total float64) float64 {
	if total == 0 {
		return 0
	}
	var h float64
	for _, c := range values {
		if c == 0 {
			continue
		}
		p := float64(c) / total
		h -= p * math.Log2(p)
	}
	return h
}

func normalizeBoundaries(bounds []int, analyzedBytes int) []int {
	if len(bounds) == 0 {
		return []int{0, analyzedBytes}
	}
	sort.Ints(bounds)
	out := make([]int, 0, len(bounds))
	last := -1
	for _, b := range bounds {
		if b < 0 {
			b = 0
		}
		if b > analyzedBytes {
			b = analyzedBytes
		}
		if b == last {
			continue
		}
		out = append(out, b)
		last = b
	}
	if out[0] != 0 {
		out = append([]int{0}, out...)
	}
	if out[len(out)-1] != analyzedBytes {
		out = append(out, analyzedBytes)
	}
	return out
}

func buildField(samples [][]byte, stats []posStat, classes []byteClass, start, end int) FieldDef {
	length := end - start
	classCount := map[byteClass]int{}
	nullAcc := 0
	digitAcc := 0
	alphaAcc := 0
	binAcc := 0
	totalRows := len(samples)

	for i := start; i < end && i < len(classes); i++ {
		classCount[classes[i]]++
		nullAcc += stats[i].nullCount + stats[i].spaceCount
		digitAcc += stats[i].digitCount
		alphaAcc += stats[i].alphaCount + stats[i].punctCount
		binAcc += stats[i].binaryCount
	}

	dominantClass := classEmpty
	domCount := 0
	for c, n := range classCount {
		if n > domCount {
			dominantClass = c
			domCount = n
		}
	}

	fType := "text"
	switch dominantClass {
	case classEmpty:
		fType = "pad"
	case classDigit:
		fType = "numeric"
	case classAlpha:
		fType = "text"
	case classAlnum:
		fType = "code"
	case classBinary:
		fType = "binary"
	case classPacked:
		fType = "packed_decimal"
	default:
		fType = "text"
	}

	samplesOut, distinct, _ := collectSegmentSamples(samples, start, length, fType, 6)
	if fType == "numeric" || fType == "code" || fType == "text" {
		if length == 1 && distinct <= 24 {
			fType = "flag"
		}
		if looksDateColumn(samplesOut) {
			fType = "date"
		}
		if fType == "code" && length > 12 {
			fType = "text"
		}
		if fType == "numeric" && length >= 10 {
			fType = "amount"
		}
	}

	cellCount := float64(totalRows * maxInt(length, 1))
	nullRate := 1.0
	if cellCount > 0 {
		nullRate = float64(nullAcc) / cellCount
	}
	domRatio := float64(domCount) / float64(maxInt(length, 1))
	contentRatio := 0.0
	if cellCount > 0 {
		contentRatio = float64(digitAcc+alphaAcc+binAcc) / cellCount
	}

	conf := 0.35 + 0.45*domRatio + 0.20*contentRatio
	if fType == "date" {
		conf += 0.08
	}
	if fType == "packed_decimal" {
		conf += 0.05
	}
	if conf > 0.99 {
		conf = 0.99
	}
	if conf < 0.25 {
		conf = 0.25
	}

	return FieldDef{
		Offset:     start,
		Length:     length,
		Type:       fType,
		Confidence: round2(conf),
		Distinct:   distinct,
		NullRate:   round3(nullRate),
		Samples:    samplesOut,
		Exported:   true,
	}
}

func mergeTinyFields(fields []FieldDef, samples [][]byte) []FieldDef {
	if len(fields) <= 1 {
		return fields
	}
	out := make([]FieldDef, 0, len(fields))
	i := 0
	for i < len(fields) {
		f := fields[i]
		if f.Length <= 1 && i+1 < len(fields) {
			n := fields[i+1]
			merged := FieldDef{
				Offset: f.Offset,
				Length: (n.Offset + n.Length) - f.Offset,
				Type:   n.Type,
			}
			merged.Samples, merged.Distinct, _ = collectSegmentSamples(samples, merged.Offset, merged.Length, merged.Type, 6)
			merged.Confidence = round2((f.Confidence + n.Confidence) / 2)
			merged.NullRate = round3((f.NullRate + n.NullRate) / 2)
			merged.Exported = true
			out = append(out, merged)
			i += 2
			continue
		}
		out = append(out, f)
		i++
	}
	return out
}

func compressFields(fields []FieldDef, maxFields int) []FieldDef {
	if len(fields) <= maxFields {
		return fields
	}
	// Merge lowest-confidence neighbors until maxFields.
	for len(fields) > maxFields {
		bestIdx := -1
		bestScore := 10.0
		for i := 0; i < len(fields)-1; i++ {
			score := fields[i].Confidence + fields[i+1].Confidence
			if score < bestScore {
				bestScore = score
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break
		}
		a := fields[bestIdx]
		b := fields[bestIdx+1]
		m := FieldDef{
			Offset:     a.Offset,
			Length:     (b.Offset + b.Length) - a.Offset,
			Type:       preferType(a.Type, b.Type),
			Confidence: round2((a.Confidence + b.Confidence) / 2),
			Distinct:   maxInt(a.Distinct, b.Distinct),
			NullRate:   round3((a.NullRate + b.NullRate) / 2),
			Samples:    mergeSampleLists(a.Samples, b.Samples, 6),
			Exported:   a.Exported || b.Exported,
		}
		fields = append(fields[:bestIdx], append([]FieldDef{m}, fields[bestIdx+2:]...)...)
	}
	return fields
}

func preferType(a, b string) string {
	priority := map[string]int{
		"date":           9,
		"flag":           8,
		"code":           7,
		"numeric":        6,
		"amount":         6,
		"text":           5,
		"packed_decimal": 4,
		"binary":         3,
		"pad":            1,
	}
	if priority[a] >= priority[b] {
		return a
	}
	return b
}

func mergeSampleLists(a, b []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, max)
	for _, v := range a {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= max {
			return out
		}
	}
	for _, v := range b {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= max {
			return out
		}
	}
	return out
}

func collectSegmentSamples(records [][]byte, offset, length int, fieldType string, maxSamples int) ([]string, int, int) {
	if length <= 0 {
		return nil, 0, 0
	}

	freq := map[string]int{}
	nonEmpty := 0

	step := 1
	if len(records) > 600 {
		step = len(records) / 600
	}
	for i := 0; i < len(records); i += step {
		v := extractValue(records[i], offset, length, fieldType)
		if v != "" {
			nonEmpty++
			freq[v]++
		}
	}

	type kv struct {
		Key string
		Val int
	}
	list := make([]kv, 0, len(freq))
	for k, v := range freq {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Val == list[j].Val {
			return list[i].Key < list[j].Key
		}
		return list[i].Val > list[j].Val
	})

	samples := make([]string, 0, minInt(maxSamples, len(list)))
	for i := 0; i < len(list) && i < maxSamples; i++ {
		samples = append(samples, list[i].Key)
	}
	return samples, len(freq), nonEmpty
}

func extractValue(rec []byte, offset, length int, fieldType string) string {
	if offset >= len(rec) || length <= 0 {
		return ""
	}
	end := offset + length
	if end > len(rec) {
		end = len(rec)
	}
	raw := rec[offset:end]

	switch fieldType {
	case "binary", "packed_decimal":
		if isAllZeroOrSpace(raw) {
			return ""
		}
		return strings.ToUpper(hex.EncodeToString(raw))
	case "pad":
		return ""
	default:
		trimmed := trimRightNullSpace(raw)
		if len(trimmed) == 0 {
			return ""
		}
		s := strings.TrimSpace(stripControl(isam.DecodeText(trimmed)))
		if s == "" {
			return ""
		}
		// Keep values compact for metadata preview.
		if len(s) > 120 {
			s = s[:120]
		}
		return s
	}
}

func stripControl(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func trimRightNullSpace(b []byte) []byte {
	end := len(b)
	for end > 0 && (b[end-1] == 0 || b[end-1] == ' ') {
		end--
	}
	return b[:end]
}

func isAllZeroOrSpace(b []byte) bool {
	for _, v := range b {
		if v != 0 && v != ' ' {
			return false
		}
	}
	return true
}

func looksDateColumn(samples []string) bool {
	if len(samples) == 0 {
		return false
	}
	valid := 0
	for _, s := range samples {
		if len(s) < 8 {
			continue
		}
		p := s
		if len(p) > 8 {
			p = p[:8]
		}
		if isAllDigits(p) && looksLikeDate(p) {
			valid++
		}
	}
	return valid > 0 && float64(valid)/float64(len(samples)) >= 0.70
}

func looksLikeDate(s string) bool {
	if len(s) < 8 {
		return false
	}
	year := s[:4]
	month := s[4:6]
	day := s[6:8]
	if !isAllDigits(year) || !isAllDigits(month) || !isAllDigits(day) {
		return false
	}
	y := atoi(year)
	m := atoi(month)
	d := atoi(day)
	if y < 1990 || y > 2035 {
		return false
	}
	if m < 1 || m > 12 {
		return false
	}
	if d < 1 || d > 31 {
		return false
	}
	return true
}

func inferFieldName(offset int, fieldType string) string {
	switch fieldType {
	case "date":
		return fmt.Sprintf("fecha_%03d", offset)
	case "flag":
		return fmt.Sprintf("flag_%03d", offset)
	case "code":
		return fmt.Sprintf("cod_%03d", offset)
	case "numeric":
		return fmt.Sprintf("num_%03d", offset)
	case "amount":
		return fmt.Sprintf("val_%03d", offset)
	case "text":
		return fmt.Sprintf("txt_%03d", offset)
	case "packed_decimal":
		return fmt.Sprintf("bcd_%03d", offset)
	case "binary":
		return fmt.Sprintf("bin_%03d", offset)
	default:
		return fmt.Sprintf("pad_%03d", offset)
	}
}

func cloneFields(in []FieldDef) []FieldDef {
	out := make([]FieldDef, len(in))
	copy(out, in)
	for i := range out {
		out[i].Samples = append([]string(nil), out[i].Samples...)
	}
	return out
}

func chooseExportFields(fields []FieldDef) []FieldDef {
	out := make([]FieldDef, 0, len(fields))
	for _, f := range fields {
		export := true
		if f.Type == "pad" {
			export = false
		}
		if f.Type == "binary" && f.Length > 24 && f.Confidence < 0.72 {
			export = false
		}
		if f.NullRate > 0.995 && f.Type != "flag" {
			export = false
		}
		f.Exported = export
		if export {
			out = append(out, f)
		}
	}
	return out
}

func sqliteTableName(fileName string) string {
	base := normalizeIdentifier(strings.ToLower(fileName))
	return "isam_" + base
}

func normalizeIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	id := b.String()
	if id == "" {
		id = "table"
	}
	// compact repeated underscores
	for strings.Contains(id, "__") {
		id = strings.ReplaceAll(id, "__", "_")
	}
	id = strings.Trim(id, "_")
	if id == "" {
		id = "table"
	}
	return id
}

func ensureMetadataTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS isam_v2_files (
			file_name TEXT PRIMARY KEY,
			table_name TEXT NOT NULL,
			group_key TEXT NOT NULL,
			record_size INTEGER NOT NULL,
			total_records INTEGER NOT NULL,
			usable_records INTEGER NOT NULL,
			exported_records INTEGER NOT NULL,
			analyzed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS isam_v2_columns (
			file_name TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			column_name TEXT NOT NULL,
			offset INTEGER NOT NULL,
			length INTEGER NOT NULL,
			inferred_type TEXT NOT NULL,
			confidence REAL NOT NULL,
			distinct_values INTEGER NOT NULL,
			null_rate REAL NOT NULL,
			sample_values TEXT NOT NULL,
			exported INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY(file_name, ordinal)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_isam_v2_columns_file ON isam_v2_columns(file_name)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func exportFileToSQLite(db *sql.DB, tableName string, records []isam.Record, fields []FieldDef) (int, error) {
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		col := normalizeIdentifier(f.Name)
		cols = append(cols, col)
	}

	createCols := []string{
		`"id" INTEGER PRIMARY KEY AUTOINCREMENT`,
		`"record_no" INTEGER NOT NULL`,
		`"file_offset" INTEGER NOT NULL`,
		`"record_hash" TEXT NOT NULL`,
	}
	for _, c := range cols {
		createCols = append(createCols, fmt.Sprintf(`"%s" TEXT`, c))
	}

	createSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s)`, tableName, strings.Join(createCols, ","))
	if _, err := db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, tableName)); err != nil {
		return 0, err
	}
	if _, err := db.Exec(createSQL); err != nil {
		return 0, err
	}

	insertCols := []string{`"record_no"`, `"file_offset"`, `"record_hash"`}
	for _, c := range cols {
		insertCols = append(insertCols, fmt.Sprintf(`"%s"`, c))
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(insertCols)), ",")
	insertSQL := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		tableName, strings.Join(insertCols, ","), placeholders)

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	inserted := 0
	for i, rec := range records {
		hash := sha256.Sum256(rec.Data)
		args := make([]any, 0, 3+len(fields))
		args = append(args, i+1, rec.Offset, fmt.Sprintf("%x", hash[:8]))
		for _, f := range fields {
			args = append(args, extractValue(rec.Data, f.Offset, f.Length, f.Type))
		}
		if _, err := stmt.Exec(args...); err != nil {
			return inserted, err
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func writeMetadata(db *sql.DB, meta fileMeta, tableName string, allFields, exportedFields []FieldDef, inserted int) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO isam_v2_files
		 (file_name, table_name, group_key, record_size, total_records, usable_records, exported_records, analyzed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		meta.Name, tableName, meta.GroupKey, meta.RecordSize, meta.TotalRecords, meta.Usable, inserted, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	exported := map[string]bool{}
	for _, f := range exportedFields {
		exported[normalizeIdentifier(f.Name)] = true
	}

	if _, err := db.Exec(`DELETE FROM isam_v2_columns WHERE file_name = ?`, meta.Name); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO isam_v2_columns
		 (file_name, ordinal, column_name, offset, length, inferred_type, confidence, distinct_values, null_rate, sample_values, exported)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, f := range allFields {
		colName := normalizeIdentifier(f.Name)
		samplesJSON, _ := json.Marshal(f.Samples)
		isExp := 0
		if exported[colName] {
			isExp = 1
		}
		if _, err := stmt.Exec(
			meta.Name,
			i,
			colName,
			f.Offset,
			f.Length,
			f.Type,
			f.Confidence,
			f.Distinct,
			f.NullRate,
			string(samplesJSON),
			isExp,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
