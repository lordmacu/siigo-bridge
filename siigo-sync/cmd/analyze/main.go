package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// FieldInfo describes a detected field within a record
type FieldInfo struct {
	Name     string   `json:"name"`
	Offset   int      `json:"offset"`
	Length   int      `json:"length"`
	Type     string   `json:"type"` // "text", "numeric", "date", "code", "flag", "amount", "binary", "empty"
	Samples  []string `json:"samples,omitempty"`
	Distinct int      `json:"distinct_values,omitempty"`
}

// KeyComponentInfo describes a key component
type KeyComponentInfo struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

// KeyDef describes a key definition from KDB
type KeyDef struct {
	Index      int                `json:"index"`
	IsPrimary  bool               `json:"is_primary"`
	AllowDups  bool               `json:"allow_dups"`
	Components []KeyComponentInfo `json:"components"`
	TotalLen   int                `json:"total_length"`
	SampleKey  string             `json:"sample_key,omitempty"`
}

// TableInfo describes a single ISAM table/file
type TableInfo struct {
	FileName    string      `json:"file_name"`
	Description string      `json:"description,omitempty"`
	RecordSize  int         `json:"record_size"`
	RecordCount int         `json:"record_count"`
	NumKeys     int         `json:"num_keys"`
	UsedEXTFH   bool        `json:"used_extfh"`
	Keys        []KeyDef    `json:"keys,omitempty"`
	Fields      []FieldInfo `json:"fields"`
}

// posClass represents the classification of a byte position
type posClass int

const (
	pcEmpty  posClass = iota // >85% null/space
	pcDigit                  // >50% digits
	pcAlpha                  // >40% letters
	pcMixed                  // mix of digits + letters
	pcBinary                 // >40% binary
	pcPackedDec              // COBOL packed decimal (BCD)
)

// posProfile tracks byte distribution at a single position across records
type posProfile struct {
	nullCount   int
	spaceCount  int
	digitCount  int
	upperCount  int
	lowerCount  int
	punctCount  int
	binaryCount int
	values      map[byte]int
}

// AnalysisResult is the top-level output
type AnalysisResult struct {
	DataPath   string      `json:"data_path"`
	TotalFiles int         `json:"total_files"`
	Tables     []TableInfo `json:"tables"`
}

// ---------------------------------------------------------------------------
// Known layouts from reverse-engineered parsers
// ---------------------------------------------------------------------------

type knownField struct {
	Name   string
	Offset int
	Length int
	Type   string
}

// knownLayoutDef holds a layout plus an optional record filter
type knownLayoutDef struct {
	fields []knownField
	// filterFn optionally filters which records match this layout.
	// If nil, all records are used.
	filterFn func(rec []byte) bool
}

var knownLayouts = map[string]knownLayoutDef{
	"Z17": {
		fields: []knownField{
			{"tipo_clave", 0, 1, "flag"},       // G/L/N/R
			{"empresa", 1, 3, "code"},           // 001
			{"codigo", 4, 14, "code"},           // 00000000002001
			{"secuencial", 18, 2, "numeric"},    // 13
			{"tipo_doc", 20, 2, "code"},         // 05 (NIT=13, CC=11)
			{"numero_doc", 22, 6, "numeric"},    // 000020
			{"fecha_creacion", 28, 8, "date"},   // 20121030
			{"tipo_tercero", 36, 2, "flag"},     // A
			{"nombre", 38, 40, "text"},          // PROVEEDORES
			{"tipo_cta_pref", 78, 1, "flag"},    // D/C
		},
		// Z17 layout applies to type 'G' (general/master) records
		filterFn: func(rec []byte) bool {
			if len(rec) < 80 {
				return false
			}
			return rec[0] == 'G'
		},
	},
	// Z49 EXTFH: single-letter type + code + number + tercero + description
	"Z49": {
		fields: []knownField{
			{"tipo", 0, 1, "flag"},             // R/T/F/E/N/C/P/L/H/J
			{"codigo_comprobante", 1, 3, "code"}, // 001, 010, 100, etc.
			{"numero_doc", 4, 11, "numeric"},    // zero-padded document number
			{"nombre_tercero", 15, 35, "text"},  // NIT name or blank
			{"descripcion", 50, 80, "text"},     // transaction description
		},
		filterFn: func(rec []byte) bool {
			if len(rec) < 50 {
				return false
			}
			// Only records with letter type at [0]
			return rec[0] >= 'A' && rec[0] <= 'Z'
		},
	},
	// Z06 EXTFH: multi-type master file (A=sucursales, B=bodegas, etc.)
	"Z06": {
		fields: []knownField{
			{"tipo", 0, 1, "flag"},         // A/B/I/V/X/Z/etc.
			{"codigo", 2, 7, "code"},       // 0001000
			{"tipo_repeat", 30, 1, "flag"}, // same as tipo
			{"nombre", 31, 20, "text"},     // primary name
			{"responsable", 70, 20, "text"}, // contact person (A/B types)
			{"direccion", 90, 30, "text"},   // address (A/B types)
		},
		filterFn: func(rec []byte) bool {
			if len(rec) < 50 {
				return false
			}
			// Only type A and B have consistent structure
			return rec[0] == 'A' || rec[0] == 'B'
		},
	},
	// Z09YYYY EXTFH: cartera/accounting entries
	"Z092013": {
		fields: []knownField{
			{"tipo_registro", 0, 1, "flag"},       // F/G/L/P/R
			{"empresa", 1, 3, "code"},              // 001
			{"secuencia", 10, 5, "numeric"},         // 00001
			{"tipo_doc", 15, 1, "flag"},             // N=NIT
			{"nit_tercero", 16, 13, "numeric"},      // 0000900019401
			{"cuenta_contable", 29, 13, "code"},     // 0004135050100
			{"fecha", 42, 8, "date"},                // 20130131
			{"descripcion", 93, 40, "text"},         // PORTATIL TOSHIBA...
			{"tipo_mov", 143, 1, "flag"},            // D/C
		},
		filterFn: func(rec []byte) bool {
			if len(rec) < 144 {
				return false
			}
			return rec[0] == 'F' || rec[0] == 'G' || rec[0] == 'L' || rec[0] == 'P' || rec[0] == 'R'
		},
	},
	"Z092014": {
		fields: []knownField{
			{"tipo_registro", 0, 1, "flag"},
			{"empresa", 1, 3, "code"},
			{"secuencia", 10, 5, "numeric"},
			{"tipo_doc", 15, 1, "flag"},
			{"nit_tercero", 16, 13, "numeric"},
			{"cuenta_contable", 29, 13, "code"},
			{"fecha", 42, 8, "date"},
			{"descripcion", 93, 40, "text"},
			{"tipo_mov", 143, 1, "flag"},
		},
		filterFn: func(rec []byte) bool {
			if len(rec) < 144 {
				return false
			}
			return rec[0] == 'F' || rec[0] == 'G' || rec[0] == 'L' || rec[0] == 'P' || rec[0] == 'R'
		},
	},
}

// Table descriptions for known files
var tableDescriptions = map[string]string{
	"C03":    "Chart of Accounts (PUC)",
	"C05":    "Company Parameters",
	"C06":    "POS/Cash Register Config",
	"INF":    "Report Definitions",
	"Z001":   "Company Configuration",
	"Z003":   "System Users",
	"Z06":    "Master Config (Branches/Warehouses/Inventory/Salespeople)",
	"Z06CP":  "Product Prices",
	"Z06MCCO": "Product Codes",
	"Z07":    "Vouchers/Documents",
	"Z07S":   "Special Vouchers",
	"Z07T":   "Voucher Types",
	"Z08":    "Account Balances",
	"Z09":    "Receivables/Payables (AR/AP)",
	"Z09CL":  "Receivables Classification",
	"Z09H":   "Receivables History",
	"Z11":    "Taxes",
	"Z11I":   "Tax Detail",
	"Z11K":   "ICA Withholding",
	"Z11L":   "Tax Settlement",
	"Z11N":   "Tax Notes",
	"Z11P":   "Tax Parameters",
	"Z11U":   "Unified Taxes",
	"Z120":   "Employees",
	"Z121":   "Employee Detail",
	"Z122":   "Payroll Concepts",
	"Z123":   "Payroll Groups",
	"Z12201": "Additional Payroll",
	"Z03":    "Accounting Entries",
	"Z04":    "Entry Detail",
	"Z05":    "Accounting Entry Detail",
	"Z14":    "Budget",
	"Z15":    "Document Cross-Reference",
	"Z16":    "Checks",
	"Z17":    "Third Parties (Clients/Vendors)",
	"Z18":    "Treasury Flows",
	"Z19":    "Salespeople",
	"Z19A":   "Salespeople Auxiliary",
	"Z232":   "Payroll Consolidated",
	"Z23":    "Payroll Consolidated",
	"Z244T":  "Fixed Asset Types",
	"Z25":    "Kardex/Inventory Movements",
	"Z26":    "Costs",
	"Z27":    "Invoicing",
	"Z27R":   "Invoice Resolutions",
	"Z28":    "Orders",
	"Z29":    "Quotes",
	"Z34":    "Warehouses",
	"Z41":    "Price Lists",
	"Z49":    "Movements/Transactions",
	"Z51":    "DIAN Resolution",
	"Z70":    "Collection Agenda",
	"Z73":    "Cost Centers",
	"Z80":    "Magnetic Media (DIAN)",
	"Z9001ES": "System Configuration",
	"Z90ES":  "Modules and Permissions",
	"Z90PO":  "Option-Level Permissions",
	"Z91PRO": "Program Catalog",
	"Z92":    "Deleted Third Parties",
	"ZCONN":  "Connections",
	"ZCTA!00": "Account Catalog",
	"ZDANE":  "DANE Codes (Colombian Cities)",
	"ZICA":   "ICA Withholding",
	"ZIND":   "Indicators",
	"ZM06":   "Inventory Master Movements",
	"ZPILA":  "Social Security Contributions",
	"ZPRE":   "Budgets",
	"ZPRGN":  "Programming",
	"ZRES":   "Resolutions",
	"ZSUBCTA": "Sub-accounts",
	"ZVEH":   "Vehicles",
	"N03":    "Payroll Entries",
	"N04":    "Payroll Detail",
	"N15":    "Payroll Document Cross-Reference",
	"N16":    "Payroll Checks",
	"N25":    "Payroll Kardex",
	"N26":    "Payroll Costs",
	"N27":    "Payroll Invoicing",
	"N28":    "Payroll Orders",
}

func main() {
	dir := `C:\SIIWI02`
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	outFile := "isam_tables.json"
	if len(os.Args) > 2 {
		outFile = os.Args[2]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading dir: %v\n", err)
		os.Exit(1)
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(name)
		upper := strings.ToUpper(name)

		skipExts := map[string]bool{
			".IDX": true, ".LOG": true, ".TXT": true, ".CFG": true,
			".BAK": true, ".EXE": true, ".DLL": true, ".INI": true,
			".DAT": true, ".CSV": true, ".XLS": true, ".PDF": true,
		}
		if skipExts[strings.ToUpper(ext)] {
			continue
		}

		if ext == "" && (strings.HasPrefix(upper, "Z") || strings.HasPrefix(upper, "C") ||
			strings.HasPrefix(upper, "N") || strings.HasPrefix(upper, "INF") ||
			strings.HasPrefix(upper, "CHGFM")) {
			files = append(files, name)
		}
	}
	sort.Strings(files)

	fmt.Printf("Found %d ISAM files in %s\n", len(files), dir)

	result := AnalysisResult{
		DataPath:   dir,
		TotalFiles: len(files),
		Tables:     make([]TableInfo, 0, len(files)),
	}

	for i, name := range files {
		fmt.Printf("[%d/%d] %s ", i+1, len(files), name)
		path := filepath.Join(dir, name)
		table := analyzeFile(path, name)
		if table != nil {
			result.Tables = append(result.Tables, *table)
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outFile, err)
		os.Exit(1)
	}

	fmt.Printf("\nDone! %d tables -> %s\n", len(result.Tables), outFile)
}

func analyzeFile(path, name string) *TableInfo {
	records, meta, err := isam.ReadIsamFileWithMeta(path)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return nil
	}
	if len(records) == 0 {
		fmt.Printf("SKIP (0 records)\n")
		return nil
	}

	table := &TableInfo{
		FileName:    name,
		Description: lookupDescription(name),
		RecordSize:  meta.RecSize,
		RecordCount: len(records),
		NumKeys:     meta.NumKeys,
		UsedEXTFH:   meta.UsedEXTFH,
	}

	// Extract key definitions
	if len(meta.Keys) > 0 {
		for _, k := range meta.Keys {
			kd := KeyDef{
				Index:     k.Index,
				IsPrimary: k.IsPrimary,
				AllowDups: k.AllowDups,
				TotalLen:  k.TotalLen,
			}
			for _, c := range k.Components {
				kd.Components = append(kd.Components, KeyComponentInfo{
					Offset: c.Offset,
					Length: c.Length,
				})
			}
			if len(records) > 0 {
				kd.SampleKey = k.ExtractKeyString(records[0])
			}
			table.Keys = append(table.Keys, kd)
		}
	}

	// Use known layout if available, otherwise auto-detect
	baseName := stripYearSuffix(name)
	if layoutDef, ok := knownLayouts[baseName]; ok {
		// Filter records if layout has a filter function
		layoutRecords := records
		if layoutDef.filterFn != nil {
			var filtered [][]byte
			for _, rec := range records {
				if layoutDef.filterFn(rec) {
					filtered = append(filtered, rec)
				}
			}
			if len(filtered) > 0 {
				layoutRecords = filtered
			}
			fmt.Printf("(%d/%d match filter) ", len(filtered), len(records))
		}
		table.Fields = applyKnownLayout(layoutDef.fields, layoutRecords, meta.RecSize)
		fmt.Printf("-> %d recs, %d fields (KNOWN layout)\n", len(records), len(table.Fields))
	} else {
		table.Fields = detectFieldsSmart(records, meta.RecSize, meta.Keys)
		fmt.Printf("-> %d recs, %d fields\n", len(records), len(table.Fields))
	}

	return table
}

// ---------------------------------------------------------------------------
// Known layout application
// ---------------------------------------------------------------------------

func applyKnownLayout(layout []knownField, records [][]byte, recSize int) []FieldInfo {
	var fields []FieldInfo

	// Apply known fields
	lastEnd := 0
	for _, kf := range layout {
		// Add gap field if there's unmapped space
		if kf.Offset > lastEnd {
			gapFields := detectRegionFields(records, lastEnd, kf.Offset)
			fields = append(fields, gapFields...)
		}

		fi := FieldInfo{
			Name:   kf.Name,
			Offset: kf.Offset,
			Length: kf.Length,
			Type:   kf.Type,
		}

		fi.Samples, fi.Distinct = collectSamples(records, kf.Offset, kf.Length, 5)
		fields = append(fields, fi)
		lastEnd = kf.Offset + kf.Length
	}

	// Detect remaining fields after known layout ends
	if lastEnd < recSize {
		remaining := detectRegionFields(records, lastEnd, recSize)
		fields = append(fields, remaining...)
	}

	return fields
}

// findGoodRecords filters records that actually match the expected known layout patterns.
// A "good" record MUST have valid dates in date fields (strongest signal) and
// printable text in text/code fields.
func findGoodRecords(layout []knownField, records [][]byte) [][]byte {
	// Find date fields - these are mandatory validators
	var dateFields []knownField
	for _, kf := range layout {
		if kf.Type == "date" {
			dateFields = append(dateFields, kf)
		}
	}

	var good [][]byte
	for _, rec := range records {
		// MANDATORY: ALL date fields must contain valid dates
		allDatesOK := true
		for _, df := range dateFields {
			if df.Offset+df.Length > len(rec) {
				allDatesOK = false
				break
			}
			val := extractRawValue(rec, df.Offset, df.Length)
			if !isAllDigits(val) || !looksLikeDate(val) {
				allDatesOK = false
				break
			}
		}
		if !allDatesOK {
			continue
		}

		// Additional scoring for other fields
		score := 0
		checks := 0
		for _, kf := range layout {
			if kf.Type == "date" || kf.Offset+kf.Length > len(rec) {
				continue
			}
			val := extractValue(rec, kf.Offset, kf.Length)
			checks++

			switch kf.Type {
			case "flag":
				if len(val) == 1 && val[0] >= 'A' && val[0] <= 'Z' {
					score++
				}
			case "code":
				if len(val) > 0 && isPrintable(val) {
					score++
				}
			case "text":
				if len(val) > 0 && isPrintable(val) {
					score++
				}
			case "numeric", "amount":
				trimmed := strings.TrimLeft(val, "0 ")
				if trimmed == "" || isAllDigits(trimmed) {
					score++
				}
			}
		}
		// Accept if date is valid + at least 40% of other fields match
		if checks == 0 || float64(score)/float64(checks) >= 0.4 {
			good = append(good, rec)
		}
	}
	// Cap for performance
	if len(good) > 200 {
		good = good[:200]
	}
	return good
}

// extractRawValue extracts the raw string at offset/length without trimming
func extractRawValue(rec []byte, offset, length int) string {
	if offset >= len(rec) {
		return ""
	}
	end := offset + length
	if end > len(rec) {
		end = len(rec)
	}
	return string(rec[offset:end])
}

// isPrintable returns true if the string contains only printable characters
func isPrintable(s string) bool {
	for _, b := range []byte(s) {
		if b < 0x20 && b != 0 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Smart field detection (improved heuristics)
// ---------------------------------------------------------------------------

func detectFieldsSmart(records [][]byte, recSize int, keys []isam.KeyInfo) []FieldInfo {
	if len(records) == 0 || recSize == 0 {
		return nil
	}

	sampleSize := len(records)
	if sampleSize > 100 {
		sampleSize = 100
	}
	samples := records[:sampleSize]
	n := float64(sampleSize)

	// Step 1: Build per-position profile
	profile := make([]posProfile, recSize)
	for i := range profile {
		profile[i].values = make(map[byte]int)
	}

	for _, rec := range samples {
		for pos := 0; pos < recSize && pos < len(rec); pos++ {
			b := rec[pos]
			p := &profile[pos]
			p.values[b]++
			switch {
			case b == 0:
				p.nullCount++
			case b == ' ':
				p.spaceCount++
			case b >= '0' && b <= '9':
				p.digitCount++
			case b >= 'A' && b <= 'Z':
				p.upperCount++
			case b >= 'a' && b <= 'z':
				p.lowerCount++
			case b == '.' || b == ',' || b == '-' || b == '/' || b == '@' || b == '_' || b == '(' || b == ')':
				p.punctCount++
			case b >= 0xC0: // accented Latin chars
				p.upperCount++
			default:
				p.binaryCount++
			}
		}
	}

	// Step 2: Classify each position with finer granularity
	classify := make([]posClass, recSize)
	for i := 0; i < recSize; i++ {
		p := profile[i]
		emptyRate := float64(p.nullCount+p.spaceCount) / n
		digitRate := float64(p.digitCount) / n
		alphaRate := float64(p.upperCount+p.lowerCount+p.punctCount) / n
		binaryRate := float64(p.binaryCount) / n

		switch {
		case emptyRate > 0.85:
			classify[i] = pcEmpty
		case binaryRate > 0.4:
			classify[i] = pcBinary
		case digitRate > 0.5 && alphaRate < 0.15:
			classify[i] = pcDigit
		case alphaRate > 0.4 && digitRate < 0.15:
			classify[i] = pcAlpha
		default:
			classify[i] = pcMixed
		}
	}

	// Step 2b: Detect COBOL packed decimal (BCD) fields
	// BCD bytes have high nibble 0-9, low nibble 0-9 (or sign: 0xC/0xD/0xF for last byte)
	// Common sign bytes: 0x0C (positive), 0x0D (negative), 0x0F (unsigned)
	for i := 0; i < recSize; i++ {
		if classify[i] != pcBinary {
			continue
		}
		// Check if this position has BCD-like values
		p := profile[i]
		bcdCount := 0
		for val, cnt := range p.values {
			hiNib := val >> 4
			loNib := val & 0x0F
			isBCD := hiNib <= 9 && loNib <= 9
			isSign := (val == 0x0C || val == 0x0D || val == 0x0F)
			if isBCD || isSign {
				bcdCount += cnt
			}
		}
		if float64(bcdCount)/n > 0.7 {
			classify[i] = pcPackedDec
		}
	}

	// Step 3: Detect field boundaries using "transition score"
	// A boundary exists where adjacent positions have very different byte distributions
	boundaries := make([]bool, recSize)
	boundaries[0] = true // always a boundary at start

	// Mark key component boundaries (from KDB - these are authoritative)
	for _, k := range keys {
		for _, c := range k.Components {
			if c.Offset < recSize {
				boundaries[c.Offset] = true
			}
			end := c.Offset + c.Length
			if end < recSize {
				boundaries[end] = true
			}
		}
	}

	// Detect transitions in classification
	for i := 1; i < recSize; i++ {
		if classify[i] != classify[i-1] {
			boundaries[i] = true
			continue
		}

		// Within same class, look for "value discontinuity":
		// If position i-1 is consistently space/null across records but position i is not,
		// that's a strong boundary (end of a right-padded field)
		prevEmpty := float64(profile[i-1].nullCount+profile[i-1].spaceCount) / n
		currEmpty := float64(profile[i].nullCount+profile[i].spaceCount) / n
		if prevEmpty > 0.5 && currEmpty < 0.3 {
			boundaries[i] = true
			continue
		}
		// Opposite: data then sudden empty = end of field
		if currEmpty > 0.5 && prevEmpty < 0.3 {
			boundaries[i] = true
			continue
		}

		// For digit fields: detect where "leading zeros" pattern changes
		// e.g., position where it goes from always '0' to variable digits
		if classify[i] == pcDigit && i > 0 {
			prevDistinct := len(profile[i-1].values)
			currDistinct := len(profile[i].values)
			// Big jump in value diversity signals different field
			if prevDistinct <= 2 && currDistinct > 5 {
				boundaries[i] = true
			} else if currDistinct <= 2 && prevDistinct > 5 {
				boundaries[i] = true
			}
		}
	}

	// Step 4: Group positions between boundaries into fields
	var fields []FieldInfo
	fieldStart := 0
	for i := 1; i <= recSize; i++ {
		if i < recSize && !boundaries[i] {
			continue
		}
		length := i - fieldStart
		if length <= 0 {
			fieldStart = i
			continue
		}

		fi := buildFieldInfo(samples, fieldStart, length, classify, profile, n)
		fields = append(fields, fi)
		fieldStart = i
	}

	// Step 5: Post-process - merge tiny fragments, refine types
	fields = postProcessFields(fields, samples)

	return fields
}

func detectRegionFields(records [][]byte, startOffset, endOffset int) []FieldInfo {
	// Create sub-records for just this region
	regionSize := endOffset - startOffset
	subRecords := make([][]byte, len(records))
	for i, rec := range records {
		if startOffset < len(rec) {
			end := endOffset
			if end > len(rec) {
				end = len(rec)
			}
			subRecords[i] = rec[startOffset:end]
		} else {
			subRecords[i] = make([]byte, regionSize)
		}
	}

	fields := detectFieldsSmart(subRecords, regionSize, nil)
	// Adjust offsets back to absolute
	for i := range fields {
		fields[i].Offset += startOffset
		fields[i].Name = inferFieldName(fields[i].Offset, fields[i].Length, fields[i].Type)
	}
	return fields
}

func buildFieldInfo(samples [][]byte, offset, length int, classify []posClass, profile []posProfile, n float64) FieldInfo {
	// Determine dominant class in this range
	classCounts := map[posClass]int{}
	for i := offset; i < offset+length && i < len(classify); i++ {
		classCounts[classify[i]]++
	}

	dominant := pcEmpty
	maxCount := 0
	for cls, cnt := range classCounts {
		if cnt > maxCount {
			dominant = cls
			maxCount = cnt
		}
	}

	fType := "text"
	switch dominant {
	case pcEmpty:
		fType = "empty"
	case pcDigit:
		fType = "numeric"
	case pcAlpha:
		fType = "text"
	case pcMixed:
		fType = "text"
	case pcBinary:
		fType = "binary"
	case pcPackedDec:
		fType = "packed_decimal"
	}

	fi := FieldInfo{
		Offset: offset,
		Length: length,
		Type:   fType,
	}

	// Collect distinct samples
	fi.Samples, fi.Distinct = collectSamples(samples, offset, length, 5)

	// Refine type based on sample values
	fi.Type = refineType(fi.Type, fi.Samples, length)
	fi.Name = inferFieldName(offset, length, fi.Type)

	return fi
}

// ---------------------------------------------------------------------------
// Type refinement and naming
// ---------------------------------------------------------------------------

func refineType(baseType string, samples []string, length int) string {
	if baseType == "empty" || baseType == "binary" || baseType == "packed_decimal" || len(samples) == 0 {
		return baseType
	}

	// Check if all samples look like dates
	if length >= 8 && length <= 10 {
		allDates := true
		for _, s := range samples {
			clean := strings.TrimSpace(s)
			if len(clean) >= 8 && isAllDigits(clean[:8]) && looksLikeDate(clean[:8]) {
				continue
			}
			if clean == "" {
				continue
			}
			allDates = false
			break
		}
		if allDates && samples[0] != "" {
			return "date"
		}
	}

	// Check if it's a single-char flag (D/C, A/I, G/L/N, S/N, etc.)
	if length == 1 {
		allSingleChar := true
		for _, s := range samples {
			if len(s) > 1 {
				allSingleChar = false
				break
			}
		}
		if allSingleChar {
			return "flag"
		}
	}

	// Check for short codes (2-5 chars, mostly same length)
	if length <= 6 && baseType == "text" {
		return "code"
	}

	// Check for amounts (digits with possible decimal)
	if baseType == "numeric" && length >= 10 {
		return "amount"
	}

	return baseType
}

func inferFieldName(offset, length int, fType string) string {
	switch fType {
	case "date":
		return fmt.Sprintf("date_%d", offset)
	case "flag":
		return fmt.Sprintf("flag_%d", offset)
	case "code":
		return fmt.Sprintf("code_%d", offset)
	case "amount":
		return fmt.Sprintf("amount_%d", offset)
	case "numeric":
		return fmt.Sprintf("num_%d", offset)
	case "text":
		return fmt.Sprintf("text_%d", offset)
	case "empty":
		return fmt.Sprintf("pad_%d", offset)
	case "binary":
		return fmt.Sprintf("bin_%d", offset)
	case "packed_decimal":
		return fmt.Sprintf("bcd_%d", offset)
	default:
		return fmt.Sprintf("field_%d", offset)
	}
}

// ---------------------------------------------------------------------------
// Post-processing
// ---------------------------------------------------------------------------

func postProcessFields(fields []FieldInfo, samples [][]byte) []FieldInfo {
	if len(fields) <= 1 {
		return fields
	}

	var result []FieldInfo
	i := 0
	for i < len(fields) {
		f := fields[i]

		// Merge consecutive empty fields
		if f.Type == "empty" && i+1 < len(fields) && fields[i+1].Type == "empty" {
			merged := f
			for i+1 < len(fields) && fields[i+1].Type == "empty" {
				i++
				merged.Length = fields[i].Offset + fields[i].Length - merged.Offset
			}
			merged.Name = inferFieldName(merged.Offset, merged.Length, "empty")
			result = append(result, merged)
			i++
			continue
		}

		// Merge tiny non-empty fields (1-2 bytes) with adjacent field of same type
		if f.Length <= 2 && f.Type != "empty" && f.Type != "flag" && i+1 < len(fields) {
			next := fields[i+1]
			if next.Type != "empty" {
				merged := FieldInfo{
					Offset: f.Offset,
					Length: next.Offset + next.Length - f.Offset,
					Type:   next.Type,
				}
				merged.Samples, merged.Distinct = collectSamples(samples, merged.Offset, merged.Length, 5)
				merged.Type = refineType(merged.Type, merged.Samples, merged.Length)
				merged.Name = inferFieldName(merged.Offset, merged.Length, merged.Type)
				result = append(result, merged)
				i += 2
				continue
			}
		}

		result = append(result, f)
		i++
	}

	return result
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func collectSamples(records [][]byte, offset, length, maxSamples int) ([]string, int) {
	seen := make(map[string]int) // value -> count
	var ordered []string         // preserve first-seen order

	// Sample from distributed positions across all records (not just first N)
	step := 1
	if len(records) > 500 {
		step = len(records) / 500
	}

	for i := 0; i < len(records); i += step {
		val := extractValue(records[i], offset, length)
		if val == "" {
			continue
		}
		if _, exists := seen[val]; !exists {
			ordered = append(ordered, val)
		}
		seen[val]++
	}

	// Return the most frequent values as samples (more representative)
	if len(ordered) <= maxSamples {
		return ordered, len(seen)
	}

	// Sort by frequency and take top N
	type valCount struct {
		val   string
		count int
	}
	var sorted []valCount
	for v, c := range seen {
		sorted = append(sorted, valCount{v, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	samples := make([]string, 0, maxSamples)
	for i := 0; i < maxSamples && i < len(sorted); i++ {
		samples = append(samples, sorted[i].val)
	}
	return samples, len(seen)
}

func extractValue(rec []byte, offset, length int) string {
	if offset >= len(rec) {
		return ""
	}
	end := offset + length
	if end > len(rec) {
		end = len(rec)
	}
	field := rec[offset:end]

	trimEnd := len(field)
	for trimEnd > 0 && (field[trimEnd-1] == ' ' || field[trimEnd-1] == 0) {
		trimEnd--
	}
	if trimEnd == 0 {
		return ""
	}
	return isam.DecodeText(field[:trimEnd])
}

func looksLikeDate(s string) bool {
	if len(s) < 8 {
		return false
	}
	if !isAllDigits(s[:8]) {
		return false
	}
	if s[0] != '1' && s[0] != '2' {
		return false
	}
	m := (s[4]-'0')*10 + (s[5] - '0')
	if m < 1 || m > 12 {
		return false
	}
	d := (s[6]-'0')*10 + (s[7] - '0')
	return d >= 1 && d <= 31
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func lookupDescription(name string) string {
	// Try exact match
	if desc, ok := tableDescriptions[name]; ok {
		return desc
	}
	// Try without year suffix (Z032013 -> Z03)
	base := stripYearSuffix(name)
	if desc, ok := tableDescriptions[base]; ok {
		return desc
	}
	// Try without trailing 'A' suffix (Z082013A -> Z08)
	if strings.HasSuffix(base, "A") {
		base2 := base[:len(base)-1]
		if desc, ok := tableDescriptions[base2]; ok {
			return desc + " (Auxiliar)"
		}
	}
	return ""
}

func stripYearSuffix(name string) string {
	// Remove year patterns: Z032013 -> Z03, Z492013 -> Z49, N032014 -> N03
	// Also: Z082013A -> Z08A, Z0620171114 -> Z06
	upper := strings.ToUpper(name)

	// Try to find where the base name ends and year begins
	// Pattern: letters/digits that form the table ID, then 4-digit year (20xx or 19xx)
	for i := 2; i < len(upper); i++ {
		remaining := upper[i:]
		if len(remaining) >= 4 && (strings.HasPrefix(remaining, "20") || strings.HasPrefix(remaining, "19")) {
			if isAllDigits(remaining[:4]) {
				base := name[:i]
				// Check if there's a suffix after the year
				after := name[i+4:]
				for len(after) > 0 && isAllDigits(string(after[0])) {
					after = after[1:]
				}
				if after != "" {
					return base + after
				}
				return base
			}
		}
	}
	return name
}
