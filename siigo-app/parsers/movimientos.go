package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-app/isam"
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
	info, err := isam.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var movimientos []Movimiento
	for _, rec := range info.Records {
		m := parseMovimientoRecord(rec.Data)
		if m.Descripcion == "" && m.NitTercero == "" {
			continue
		}
		movimientos = append(movimientos, m)
	}

	return movimientos, nil
}

// ParseMovimientosAnio reads movements for a specific year (Z49YYYY)
func ParseMovimientosAnio(dataPath string, anio string) ([]Movimiento, error) {
	path := dataPath + "Z49" + anio
	info, err := isam.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var movimientos []Movimiento
	for _, rec := range info.Records {
		m := parseMovimientoRecord(rec.Data)
		if m.Descripcion == "" && m.NitTercero == "" {
			continue
		}
		movimientos = append(movimientos, m)
	}

	return movimientos, nil
}

func parseMovimientoRecord(rec []byte) Movimiento {
	if len(rec) < 100 {
		return Movimiento{}
	}

	hash := sha256.Sum256(rec)
	rawPreview := isam.ExtractField(rec, 0, 120)

	// Z49 record structure (2,295 bytes)
	// Siigo transaction records follow a pattern similar to Z17:
	// The first bytes contain type codes and keys
	//
	// Observed structure from hex analysis:
	// 0:   TipoComprobante (2-4) - RC, FV, NC, ND, CC, etc.
	// 4:   Empresa (3)           - 001
	// 7:   NumeroDoc (8-10)      - document number
	// 17:  Fecha (8)             - YYYYMMDD
	// 25:  NitTercero (13)       - NIT with leading zeros
	// 38:  CuentaContable (13)   - PUC account code
	// 51:  CentroCosto (6)       - cost center
	// 57:  TipoMov (1)           - D/C
	// 58:  Valor (15)            - amount as text with decimals
	// 73+: Descripcion (80)      - description text

	tipoComp := isam.ExtractField(rec, 0, 4)
	empresa := isam.ExtractField(rec, 4, 3)
	numeroDoc := isam.ExtractField(rec, 7, 10)
	fecha := isam.ExtractField(rec, 17, 8)
	nitTercero := isam.ExtractField(rec, 25, 13)
	cuentaContable := isam.ExtractField(rec, 38, 13)
	tipoMov := isam.ExtractField(rec, 57, 1)
	valor := isam.ExtractField(rec, 58, 15)

	// Find description - look for the longest readable text block
	// Description is typically after the numeric fields
	descripcion := findDescripcion(rec, 73)

	// If structured extraction didn't work, try heuristic approach
	if !looksLikeDate(fecha) {
		// Try alternative offsets - Siigo versions may vary
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

				// NIT is usually before or near the date
				// Look backwards for a numeric block (NIT)
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

	// Scan for account code pattern (4-digit starts with 1-9, like 1105, 2408)
	for i := 0; i < len(rec)-4 && i < 200; i++ {
		if rec[i] >= '1' && rec[i] <= '9' && isDigitRange(rec, i, 4) {
			code := isam.ExtractField(rec, i, 4)
			if looksLikeAccount(code) {
				m.CuentaContable = strings.TrimSpace(isam.ExtractField(rec, i, 13))
				break
			}
		}
	}

	// Find description text
	m.Descripcion = findDescripcion(rec, 0)

	// Extract tipo comprobante from first few bytes
	if len(rec) > 4 {
		tc := isam.ExtractField(rec, 0, 4)
		if len(tc) >= 2 {
			m.TipoComprobante = strings.TrimSpace(tc)
		}
	}

	// Look for D or C for debit/credit
	for i := 50; i < len(rec) && i < 100; i++ {
		if rec[i] == 'D' || rec[i] == 'C' {
			if (i == 0 || rec[i-1] == ' ' || rec[i-1] == 0) &&
				(i+1 >= len(rec) || rec[i+1] == ' ' || rec[i+1] == 0 || (rec[i+1] >= '0' && rec[i+1] <= '9')) {
				m.TipoMov = string(rec[i])
				break
			}
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
