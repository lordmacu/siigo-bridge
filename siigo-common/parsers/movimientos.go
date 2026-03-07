package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Movimiento represents a transaction from Siigo Z49 file
type Movimiento struct {
	TipoComprobante string `json:"tipo_comprobante"` // RC=recibo caja, FV=factura venta, etc.
	Empresa         string `json:"empresa"`          // 001
	NumeroDoc       string `json:"numero_doc"`       // document number
	Fecha           string `json:"fecha"`            // YYYYMMDD
	NitTercero      string `json:"nit_tercero"`      // NIT or CC of the third party
	CuentaContable  string `json:"cuenta_contable"`  // PUC account code
	Descripcion     string `json:"descripcion"`      // transaction description
	Valor           string `json:"valor"`            // amount as text
	TipoMov         string `json:"tipo_mov"`         // D=debit, C=credit
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
		if m.Descripcion == "" && m.NitTercero == "" && m.NumeroDoc == "" {
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
		if m.Descripcion == "" && m.NitTercero == "" && m.NumeroDoc == "" {
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
// Z49 EXTFH structure (2295 bytes) - verified via hex dump:
//
//	[0]      tipo letter: R=recibo, T=traslado, F=factura, E=egreso,
//	         N=nota, P=pago, L=libro, H=hoja, J=journal, C=comprobante
//	         Space(0x20) = NIT-keyed record (no comprobante type)
//	[1:4]    codigo_comprobante - 3 digit code (001, 010, 100, etc.)
//	[4:15]   numero_doc - 11 chars zero-padded document number (also NIT for space-type)
//	[15:50]  nombre_tercero - 35 chars (only present in some types: T, E)
//	[72+]    descripcion - text description (variable position within ~72-200)
//	[129+]   additional description / user initials
//
// Z49 does NOT store dates, cuenta_contable, valores, or tipo_mov as text.
// Financial details must come from Z03 (movimientos contables) or Z09 (cartera).
func parseMovimientoEXTFH(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	tipo := rec[0]

	// Space-prefixed records: keyed by NIT, no comprobante type
	if tipo == ' ' || tipo == '0' {
		numDoc := strings.TrimLeft(isam.ExtractField(rec, 4, 11), "0")
		desc := findDescripcion(rec, 50)
		if numDoc != "" || desc != "" {
			return Movimiento{
				NitTercero:  numDoc, // for space-type records, the doc number IS the NIT
				Descripcion: desc,
				RawPreview:  rawPreview,
				Hash:        fmt.Sprintf("%x", hash[:8]),
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

	// Description at offset 72+
	descripcion := findDescripcion(rec, 72)

	return Movimiento{
		TipoComprobante: tipoComp,
		NumeroDoc:       numeroDoc,
		NitTercero:      nombreTercero,
		Descripcion:     descripcion,
		RawPreview:      rawPreview,
		Hash:            fmt.Sprintf("%x", hash[:8]),
	}
}

// parseMovimientoBinary extracts data from binary-reader Z49 records.
// Binary reader includes 2-byte record markers, so offsets differ from EXTFH.
func parseMovimientoBinary(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	if len(rec) < 100 {
		return Movimiento{}
	}

	tipoComp := isam.ExtractField(rec, 0, 4)
	empresa := isam.ExtractField(rec, 4, 3)
	numeroDoc := isam.ExtractField(rec, 7, 10)
	fecha := isam.ExtractField(rec, 17, 8)
	nitTercero := isam.ExtractField(rec, 25, 13)
	cuentaContable := isam.ExtractField(rec, 38, 13)
	tipoMov := isam.ExtractField(rec, 57, 1)
	valor := isam.ExtractField(rec, 58, 15)
	descripcion := findDescripcion(rec, 73)

	if !looksLikeDate(fecha) {
		return parseMovimientoHeuristic(rec, hash, rawPreview)
	}

	return Movimiento{
		TipoComprobante: strings.TrimSpace(tipoComp),
		Empresa:         strings.TrimSpace(empresa),
		NumeroDoc:       strings.TrimLeft(strings.TrimSpace(numeroDoc), "0"),
		Fecha:           fecha,
		NitTercero:      strings.TrimLeft(strings.TrimSpace(nitTercero), "0"),
		CuentaContable:  strings.TrimSpace(cuentaContable),
		Descripcion:     descripcion,
		Valor:           strings.TrimSpace(valor),
		TipoMov:         tipoMov,
		RawPreview:      rawPreview,
		Hash:            fmt.Sprintf("%x", hash[:8]),
	}
}

// parseMovimientoHeuristic tries to extract data when exact offsets don't match
func parseMovimientoHeuristic(rec []byte, hash [32]byte, rawPreview string) Movimiento {
	m := Movimiento{
		RawPreview: rawPreview,
		Hash:       fmt.Sprintf("%x", hash[:8]),
	}

	// Scan for date pattern (YYYYMMDD - starts with 20xx)
	for i := 0; i < len(rec)-8 && i < 200; i++ {
		if rec[i] == '2' && rec[i+1] == '0' && isDigitRange(rec, i, 8) {
			candidate := isam.ExtractField(rec, i, 8)
			if looksLikeDate(candidate) {
				m.Fecha = candidate
				for j := i - 1; j >= 0 && j > i-20; j-- {
					if rec[j] >= '0' && rec[j] <= '9' {
						start := j
						for start > 0 && rec[start-1] >= '0' && rec[start-1] <= '9' {
							start--
						}
						if j-start >= 4 {
							m.NitTercero = strings.TrimLeft(isam.ExtractField(rec, start, j-start+1), "0")
						}
						break
					}
				}
				break
			}
		}
	}

	m.Descripcion = findDescripcion(rec, 0)

	if len(rec) > 4 {
		tc := isam.ExtractField(rec, 0, 4)
		if len(tc) >= 2 {
			m.TipoComprobante = strings.TrimSpace(tc)
		}
	}

	return m
}

func findDescripcion(rec []byte, startFrom int) string {
	if startFrom >= len(rec) {
		startFrom = 0
	}

	bestStart := -1
	bestLen := 0
	inText := false
	textStart := 0

	limit := len(rec)
	if limit > 500 {
		limit = 500
	}

	for i := startFrom; i < limit; i++ {
		isReadable := (rec[i] >= 'A' && rec[i] <= 'Z') || rec[i] == ' ' || rec[i] == '.' ||
			rec[i] == ',' || rec[i] == '/' || rec[i] == '-' ||
			(rec[i] >= '0' && rec[i] <= '9') ||
			(rec[i] >= 0xC0 && rec[i] <= 0xFF) // accented chars

		if !inText && rec[i] >= 'A' && rec[i] <= 'Z' {
			inText = true
			textStart = i
		} else if inText && !isReadable {
			textLen := i - textStart
			if textLen > bestLen && textLen > 5 {
				bestStart = textStart
				bestLen = textLen
			}
			inText = false
		}
	}
	if inText {
		textLen := limit - textStart
		if textLen > bestLen && textLen > 5 {
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
	return map[string]interface{}{
		"nit":             m.NitTercero,
		"numero_factura":  m.NumeroDoc,
		"fecha_recaudo":   formatFecha(m.Fecha),
		"valor_cancelado": m.Valor,
		"descripcion":     m.Descripcion,
		"siigo_sync_hash": m.Hash,
	}
}

// IsReciboCaja returns true if this movement is a cash receipt (recaudo)
func (m *Movimiento) IsReciboCaja() bool {
	return m.TipoComprobante == "RC" || m.TipoComprobante == "RC01"
}

// IsFacturaVenta returns true if this movement is a sales invoice
func (m *Movimiento) IsFacturaVenta() bool {
	return m.TipoComprobante == "FV" || m.TipoComprobante == "FV01"
}

// formatFecha converts YYYYMMDD to YYYY-MM-DD
func formatFecha(fecha string) string {
	if len(fecha) != 8 {
		return fecha
	}
	return fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
}
