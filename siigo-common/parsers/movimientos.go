package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Movimiento represents a document header from Siigo Z49 file.
// Z49 is a document INDEX — it stores headers only (type, number, name, description).
// It does NOT contain: dates, accounting account, amounts, or D/C type.
// For detailed accounting lines with those fields, use Z09 (cartera).
type Movimiento struct {
	VoucherType    string `json:"tipo_comprobante"` // RC=cash receipt, FV=sales invoice, etc.
	Company        string `json:"empresa"`          // 001
	DocNumber      string `json:"numero_doc"`       // document number
	ThirdPartyName string `json:"nombre_tercero"`   // third party name (only E, T types)
	Description    string `json:"descripcion"`      // transaction description
	Description2   string `json:"descripcion2"`     // secondary description / user initials
	RawPreview     string `json:"raw_preview"`      // first 120 chars for debugging
	Hash           string `json:"hash"`
}

// ParseMovimientos reads the Z49 file and returns all movements
func ParseMovimientos(dataPath string) ([]Movimiento, error) {
	path := dataPath + "Z49"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var movements []Movimiento
	for _, rec := range records {
		m := parseMovimientoRecord(rec, extfh)
		if m.Description == "" && m.ThirdPartyName == "" && m.DocNumber == "" {
			continue
		}
		movements = append(movements, m)
	}

	return movements, nil
}

// ParseMovimientosAnio reads movements for a specific year (Z49YYYY)
func ParseMovimientosAnio(dataPath string, year string) ([]Movimiento, error) {
	path := dataPath + "Z49" + year
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var movements []Movimiento
	for _, rec := range records {
		m := parseMovimientoRecord(rec, extfh)
		if m.Description == "" && m.ThirdPartyName == "" && m.DocNumber == "" {
			continue
		}
		movements = append(movements, m)
	}

	return movements, nil
}

func parseMovimientoRecord(rec []byte, extfh bool) Movimiento {
	if len(rec) < 50 {
		return Movimiento{}
	}

	hash := sha256.Sum256(rec)
	rawPreview := isam.ExtractField(rec, 0, 120)

	if extfh {
		return parseMovimientoEXTFH(rec, hash, rawPreview)
	}
	return parseMovimientoBinary(rec, hash, rawPreview)
}

// parseMovimientoEXTFH extracts data from EXTFH-delivered Z49 records.
// Z49 EXTFH structure (2295 bytes) - verified via hex dump 2026-03-08:
//
//	[0]      tipo letter: R=receipt, T=transfer, F=invoice, E=expense,
//	         N=note, P=payment, L=ledger, H=worksheet, J=journal, C=voucher
//	         Space(0x20) = NIT-keyed record (no voucher type)
//	[1:4]    codigo_comprobante - 3 digit code (001, 010, 100, etc.)
//	[4:15]   numero_doc - 11 chars zero-padded document number (also NIT for space-type)
//	[15:72]  nombre_tercero - 57 chars (only for types: T, E, F)
//	[72:128] descripcion - primary description text
//	[129:192] descripcion2 - secondary description / user initials
//
// Z49 does NOT store: dates, cuenta_contable, values, or D/C type.
// Actual data extent: max ~192 bytes used, rest is spaces.
// For detailed accounting lines, use Z09 (cartera).
func parseMovimientoEXTFH(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	recType := rec[0]

	// Space-prefixed records: keyed by NIT, no voucher type
	if recType == ' ' || recType == '0' {
		docNum := strings.TrimLeft(isam.ExtractField(rec, 4, 11), "0")
		desc := findDescripcion(rec, 50, 128)
		desc2 := findDescripcion(rec, 129, 192)
		if docNum != "" || desc != "" {
			return Movimiento{
				DocNumber:      docNum,
				ThirdPartyName: docNum, // for space-type records, the doc number IS the NIT
				Description:    desc,
				Description2:   desc2,
				RawPreview:     rawPreview,
				Hash:           fmt.Sprintf("%x", hash[:8]),
			}
		}
		return Movimiento{}
	}

	// Skip records without a letter type at position 0
	if recType < 'A' || recType > 'Z' {
		return Movimiento{}
	}

	// Map single-letter type to Siigo voucher code
	typeMap := map[byte]string{
		'R': "RC", // Cash receipt
		'T': "TR", // Transfer
		'F': "FV", // Sales invoice
		'E': "CE", // Expense voucher
		'N': "ND", // Debit note
		'C': "CC", // Accounting voucher
		'P': "PG", // Payment
		'L': "LB", // Ledger
		'H': "HJ", // Worksheet
		'J': "JR", // Journal
	}

	voucherType := typeMap[recType]
	if voucherType == "" {
		voucherType = string(recType)
	}

	code := strings.TrimSpace(isam.ExtractField(rec, 1, 3))
	if code != "" {
		voucherType = voucherType + code
	}

	docNum := strings.TrimLeft(isam.ExtractField(rec, 4, 11), "0")
	// thirdPartyName: bytes 15-71 (57 chars). Previously extracted only 35 chars,
	// truncating text like "GERENCIA Y CONTABILIDAD" -> "GERENCIA".
	thirdPartyName := strings.TrimSpace(isam.ExtractField(rec, 15, 57))

	// Two description areas - extended zone 2 to byte 250 (data confirmed past 192)
	description := findDescripcion(rec, 72, 128)
	description2 := findDescripcion(rec, 129, 250)

	return Movimiento{
		VoucherType:    voucherType,
		DocNumber:      docNum,
		ThirdPartyName: thirdPartyName,
		Description:    description,
		Description2:   description2,
		RawPreview:     rawPreview,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

// parseMovimientoBinary extracts data from binary-reader Z49 records.
// Binary reader includes 2-byte record markers, so all offsets shift +2.
func parseMovimientoBinary(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	if len(rec) < 50 {
		return Movimiento{}
	}

	recType := rec[2] // +2 for binary marker

	if recType == ' ' || recType == '0' {
		docNum := strings.TrimLeft(isam.ExtractField(rec, 6, 11), "0")
		desc := findDescripcion(rec, 52, 130)
		desc2 := findDescripcion(rec, 131, 194)
		if docNum != "" || desc != "" {
			return Movimiento{
				DocNumber:      docNum,
				ThirdPartyName: docNum,
				Description:    desc,
				Description2:   desc2,
				RawPreview:     rawPreview,
				Hash:           fmt.Sprintf("%x", hash[:8]),
			}
		}
		return Movimiento{}
	}

	if recType < 'A' || recType > 'Z' {
		return Movimiento{}
	}

	typeMap := map[byte]string{
		'R': "RC", 'T': "TR", 'F': "FV", 'E': "CE", 'N': "ND",
		'C': "CC", 'P': "PG", 'L': "LB", 'H': "HJ", 'J': "JR",
	}

	voucherType := typeMap[recType]
	if voucherType == "" {
		voucherType = string(recType)
	}

	code := strings.TrimSpace(isam.ExtractField(rec, 3, 3))
	if code != "" {
		voucherType = voucherType + code
	}

	docNum := strings.TrimLeft(isam.ExtractField(rec, 6, 11), "0")
	thirdPartyName := strings.TrimSpace(isam.ExtractField(rec, 17, 57))
	description := findDescripcion(rec, 74, 130)
	description2 := findDescripcion(rec, 131, 252)

	return Movimiento{
		VoucherType:    voucherType,
		DocNumber:      docNum,
		ThirdPartyName: thirdPartyName,
		Description:    description,
		Description2:   description2,
		RawPreview:     rawPreview,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func findDescripcion(rec []byte, startFrom int, endAtOpt ...int) string {
	if startFrom >= len(rec) {
		return ""
	}

	limit := 500
	if len(endAtOpt) > 0 {
		limit = endAtOpt[0]
	}
	if limit > len(rec) {
		limit = len(rec)
	}

	bestStart := -1
	bestLen := 0
	inText := false
	textStart := 0

	for i := startFrom; i < limit; i++ {
		isReadable := (rec[i] >= 'A' && rec[i] <= 'Z') || (rec[i] >= 'a' && rec[i] <= 'z') ||
			rec[i] == ' ' || rec[i] == '.' ||
			rec[i] == ',' || rec[i] == '/' || rec[i] == '-' ||
			(rec[i] >= '0' && rec[i] <= '9') ||
			(rec[i] >= 0xC0 && rec[i] <= 0xFF) // accented chars

		if !inText && ((rec[i] >= 'A' && rec[i] <= 'Z') || (rec[i] >= 'a' && rec[i] <= 'z')) {
			inText = true
			textStart = i
		} else if inText && !isReadable {
			textLen := i - textStart
			if textLen > bestLen && textLen > 2 {
				bestStart = textStart
				bestLen = textLen
			}
			inText = false
		}
	}
	if inText {
		textLen := limit - textStart
		if textLen > bestLen && textLen > 2 {
			bestStart = textStart
			bestLen = textLen
		}
	}

	if bestStart >= 0 {
		if bestLen > 120 {
			bestLen = 120
		}
		return strings.TrimSpace(isam.ExtractField(rec, bestStart, bestLen))
	}
	return ""
}

func isDigitRange(rec []byte, start, length int) bool {
	if start+length > len(rec) {
		return false
	}
	for i := start; i < start+length; i++ {
		if rec[i] < '0' || rec[i] > '9' {
			return false
		}
	}
	return true
}

func looksLikeDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	// Must start with 19 or 20 (year)
	if s[0] != '1' && s[0] != '2' {
		return false
	}
	// Month 01-12
	m := (s[4]-'0')*10 + (s[5] - '0')
	if m < 1 || m > 12 {
		return false
	}
	// Day 01-31
	d := (s[6]-'0')*10 + (s[7] - '0')
	if d < 1 || d > 31 {
		return false
	}
	return true
}

func looksLikeAccount(s string) bool {
	if len(s) < 4 {
		return false
	}
	// PUC accounts: 1xxx=assets, 2xxx=liabilities, etc. up to 9xxx
	first := s[0]
	return first >= '1' && first <= '9'
}

// cleanPrintable strips non-printable characters from a string, keeping only
// ASCII printable (0x20-0x7E) and extended Latin (0xC0-0xFF for accented chars).
func cleanPrintable(s string) string {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 0x20 && b <= 0x7E) || (b >= 0xC0) {
			clean = append(clean, b)
		}
	}
	return strings.TrimSpace(string(clean))
}

// ToFinearomRecaudo converts a movement to a collection receipt map for Finearom API
func (m *Movimiento) ToFinearomRecaudo() map[string]interface{} {
	desc := m.Description
	if m.Description2 != "" {
		desc = desc + " " + m.Description2
	}
	return map[string]interface{}{
		"nit":              m.ThirdPartyName,
		"numero_factura":   m.DocNumber,
		"tipo_comprobante": m.VoucherType,
		"descripcion":      desc,
		"siigo_sync_hash":  m.Hash,
	}
}

// IsReciboCaja returns true if this movement is a cash receipt
func (m *Movimiento) IsReciboCaja() bool {
	return len(m.VoucherType) >= 2 && m.VoucherType[:2] == "RC"
}

// IsFacturaVenta returns true if this movement is a sales invoice
func (m *Movimiento) IsFacturaVenta() bool {
	return len(m.VoucherType) >= 2 && m.VoucherType[:2] == "FV"
}
