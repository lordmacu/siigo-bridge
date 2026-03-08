package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Movimiento represents a document header from Siigo Z49 file.
// Z49 is a document INDEX — it stores headers only (type, number, name, description).
// It does NOT contain: dates, cuenta contable, valores, or tipo D/C.
// For detailed accounting lines with those fields, use Z09 (cartera).
type Movimiento struct {
	TipoComprobante string `json:"tipo_comprobante"` // RC=recibo caja, FV=factura venta, etc.
	Empresa         string `json:"empresa"`          // 001
	NumeroDoc       string `json:"numero_doc"`       // document number
	NombreTercero   string `json:"nombre_tercero"`   // third party name (only E, T types)
	Descripcion     string `json:"descripcion"`      // transaction description
	Descripcion2    string `json:"descripcion2"`     // secondary description / user initials
	RawPreview      string `json:"raw_preview"`      // first 120 chars for debugging
	Hash            string `json:"hash"`
}

// ParseMovimientos reads the Z49 file and returns all movements
func ParseMovimientos(dataPath string) ([]Movimiento, error) {
	path := dataPath + "Z49"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var movimientos []Movimiento
	for _, rec := range records {
		m := parseMovimientoRecord(rec, extfh)
		if m.Descripcion == "" && m.NombreTercero == "" && m.NumeroDoc == "" {
			continue
		}
		movimientos = append(movimientos, m)
	}

	return movimientos, nil
}

// ParseMovimientosAnio reads movements for a specific year (Z49YYYY)
func ParseMovimientosAnio(dataPath string, anio string) ([]Movimiento, error) {
	path := dataPath + "Z49" + anio
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var movimientos []Movimiento
	for _, rec := range records {
		m := parseMovimientoRecord(rec, extfh)
		if m.Descripcion == "" && m.NombreTercero == "" && m.NumeroDoc == "" {
			continue
		}
		movimientos = append(movimientos, m)
	}

	return movimientos, nil
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
//	[0]      tipo letter: R=recibo, T=traslado, F=factura, E=egreso,
//	         N=nota, P=pago, L=libro, H=hoja, J=journal, C=comprobante
//	         Space(0x20) = NIT-keyed record (no comprobante type)
//	[1:4]    codigo_comprobante - 3 digit code (001, 010, 100, etc.)
//	[4:15]   numero_doc - 11 chars zero-padded document number (also NIT for space-type)
//	[15:50]  nombre_tercero - 35 chars (only for types: T, E)
//	[72:128] descripcion - primary description text
//	[129:192] descripcion2 - secondary description / user initials
//
// Z49 does NOT store: dates, cuenta_contable, valores, or tipo D/C.
// Actual data extent: max ~192 bytes used, rest is spaces.
// For detailed accounting lines, use Z09 (cartera).
func parseMovimientoEXTFH(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	tipo := rec[0]

	// Space-prefixed records: keyed by NIT, no comprobante type
	if tipo == ' ' || tipo == '0' {
		numDoc := strings.TrimLeft(isam.ExtractField(rec, 4, 11), "0")
		desc := findDescripcion(rec, 50, 128)
		desc2 := findDescripcion(rec, 129, 192)
		if numDoc != "" || desc != "" {
			return Movimiento{
				NumeroDoc:    numDoc,
				NombreTercero: numDoc, // for space-type records, the doc number IS the NIT
				Descripcion:  desc,
				Descripcion2: desc2,
				RawPreview:   rawPreview,
				Hash:         fmt.Sprintf("%x", hash[:8]),
			}
		}
		return Movimiento{}
	}

	// Skip records without a letter type at position 0
	if tipo < 'A' || tipo > 'Z' {
		return Movimiento{}
	}

	// Map single-letter tipo to Siigo comprobante code
	tipoMap := map[byte]string{
		'R': "RC", // Recibo de Caja
		'T': "TR", // Traslado
		'F': "FV", // Factura de Venta
		'E': "CE", // Comprobante de Egreso
		'N': "ND", // Nota Debito
		'C': "CC", // Comprobante de Contabilidad
		'P': "PG", // Pago
		'L': "LB", // Libro
		'H': "HJ", // Hoja de trabajo
		'J': "JR", // Journal
	}

	tipoComp := tipoMap[tipo]
	if tipoComp == "" {
		tipoComp = string(tipo)
	}

	codigo := strings.TrimSpace(isam.ExtractField(rec, 1, 3))
	if codigo != "" {
		tipoComp = tipoComp + codigo
	}

	numeroDoc := strings.TrimLeft(isam.ExtractField(rec, 4, 11), "0")
	nombreTercero := strings.TrimSpace(isam.ExtractField(rec, 15, 35))

	// Two description areas
	descripcion := findDescripcion(rec, 72, 128)
	descripcion2 := findDescripcion(rec, 129, 192)

	return Movimiento{
		TipoComprobante: tipoComp,
		NumeroDoc:       numeroDoc,
		NombreTercero:   nombreTercero,
		Descripcion:     descripcion,
		Descripcion2:    descripcion2,
		RawPreview:      rawPreview,
		Hash:            fmt.Sprintf("%x", hash[:8]),
	}
}

// parseMovimientoBinary extracts data from binary-reader Z49 records.
// Binary reader includes 2-byte record markers, so all offsets shift +2.
func parseMovimientoBinary(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	if len(rec) < 50 {
		return Movimiento{}
	}

	tipo := rec[2] // +2 for binary marker

	if tipo == ' ' || tipo == '0' {
		numDoc := strings.TrimLeft(isam.ExtractField(rec, 6, 11), "0")
		desc := findDescripcion(rec, 52, 130)
		desc2 := findDescripcion(rec, 131, 194)
		if numDoc != "" || desc != "" {
			return Movimiento{
				NumeroDoc:     numDoc,
				NombreTercero: numDoc,
				Descripcion:   desc,
				Descripcion2:  desc2,
				RawPreview:    rawPreview,
				Hash:          fmt.Sprintf("%x", hash[:8]),
			}
		}
		return Movimiento{}
	}

	if tipo < 'A' || tipo > 'Z' {
		return Movimiento{}
	}

	tipoMap := map[byte]string{
		'R': "RC", 'T': "TR", 'F': "FV", 'E': "CE", 'N': "ND",
		'C': "CC", 'P': "PG", 'L': "LB", 'H': "HJ", 'J': "JR",
	}

	tipoComp := tipoMap[tipo]
	if tipoComp == "" {
		tipoComp = string(tipo)
	}

	codigo := strings.TrimSpace(isam.ExtractField(rec, 3, 3))
	if codigo != "" {
		tipoComp = tipoComp + codigo
	}

	numeroDoc := strings.TrimLeft(isam.ExtractField(rec, 6, 11), "0")
	nombreTercero := strings.TrimSpace(isam.ExtractField(rec, 17, 35))
	descripcion := findDescripcion(rec, 74, 130)
	descripcion2 := findDescripcion(rec, 131, 194)

	return Movimiento{
		TipoComprobante: tipoComp,
		NumeroDoc:       numeroDoc,
		NombreTercero:   nombreTercero,
		Descripcion:     descripcion,
		Descripcion2:    descripcion2,
		RawPreview:      rawPreview,
		Hash:            fmt.Sprintf("%x", hash[:8]),
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
		if bestLen > 80 {
			bestLen = 80
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
	// PUC accounts: 1xxx=activos, 2xxx=pasivos, etc. up to 9xxx
	first := s[0]
	return first >= '1' && first <= '9'
}

// ToFinearomRecaudo converts a movement to a recaudo map for Finearom API
func (m *Movimiento) ToFinearomRecaudo() map[string]interface{} {
	desc := m.Descripcion
	if m.Descripcion2 != "" {
		desc = desc + " " + m.Descripcion2
	}
	return map[string]interface{}{
		"nit":              m.NombreTercero,
		"numero_factura":   m.NumeroDoc,
		"tipo_comprobante": m.TipoComprobante,
		"descripcion":      desc,
		"siigo_sync_hash":  m.Hash,
	}
}

// IsReciboCaja returns true if this movement is a cash receipt (recaudo)
func (m *Movimiento) IsReciboCaja() bool {
	return len(m.TipoComprobante) >= 2 && m.TipoComprobante[:2] == "RC"
}

// IsFacturaVenta returns true if this movement is a sales invoice
func (m *Movimiento) IsFacturaVenta() bool {
	return len(m.TipoComprobante) >= 2 && m.TipoComprobante[:2] == "FV"
}
