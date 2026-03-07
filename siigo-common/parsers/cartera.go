package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Cartera represents a portfolio/accounting entry from Siigo Z09YYYY files.
// Z09 files contain the full accounting detail: NIT, cuenta contable, fecha,
// descripcion, tipo movimiento (D/C), and amounts (in packed decimal).
type Cartera struct {
	TipoRegistro   string `json:"tipo_registro"`   // F=factura, G=general, L=libro, P=pago, R=recibo
	Empresa        string `json:"empresa"`          // 001
	Secuencia      string `json:"secuencia"`        // sequential record number
	TipoDoc        string `json:"tipo_doc"`         // N=NIT, etc.
	NitTercero     string `json:"nit_tercero"`      // 13-digit NIT
	CuentaContable string `json:"cuenta_contable"`  // 13-digit PUC account
	Fecha          string `json:"fecha"`            // YYYYMMDD
	Descripcion    string `json:"descripcion"`      // transaction description
	TipoMov        string `json:"tipo_mov"`         // D=debit, C=credit
	Hash           string `json:"hash"`
}

// ParseCartera reads a Z09YYYY file and returns all cartera entries
func ParseCartera(dataPath string, anio string) ([]Cartera, error) {
	path := dataPath + "Z09" + anio
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var cartera []Cartera
	for _, rec := range records {
		c := parseCarteraRecord(rec, extfh)
		if c.NitTercero == "" && c.Descripcion == "" {
			continue
		}
		cartera = append(cartera, c)
	}

	return cartera, nil
}

// ParseCarteraByTipo returns cartera entries filtered by tipo_registro
func ParseCarteraByTipo(dataPath string, anio string, tipo byte) ([]Cartera, error) {
	all, err := ParseCartera(dataPath, anio)
	if err != nil {
		return nil, err
	}
	var filtered []Cartera
	for _, c := range all {
		if len(c.TipoRegistro) > 0 && c.TipoRegistro[0] == tipo {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

func parseCarteraRecord(rec []byte, extfh bool) Cartera {
	if len(rec) < 50 {
		return Cartera{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseCarteraEXTFH(rec, hash)
	}
	return parseCarteraHeuristic(rec, hash)
}

// parseCarteraEXTFH extracts cartera records using verified EXTFH offsets.
// Z09YYYY structure via EXTFH (1152 bytes):
//
//	[0:1]   tipo_registro - F/G/L/P/R
//	[1:4]   empresa - 001
//	[4:9]   padding (nulls)
//	[9:10]  separator (0x1F)
//	[10:15] secuencia - 00001
//	[15:16] tipo_doc - N=NIT, etc.
//	[16:29] nit_tercero - 13 chars, zero-padded NIT
//	[29:42] cuenta_contable - 13 chars, PUC account code
//	[42:50] fecha - YYYYMMDD
//	[50:93] packed decimal data (amounts, counts - binary, not decoded yet)
//	[93:133] descripcion - 40 chars text
//	[143:144] tipo_mov - D=debit, C=credit
func parseCarteraEXTFH(rec []byte, hash [32]byte) Cartera {
	tipo := rec[0]
	// Valid cartera record types
	if tipo != 'F' && tipo != 'G' && tipo != 'L' && tipo != 'P' && tipo != 'R' {
		return Cartera{}
	}

	empresa := isam.ExtractField(rec, 1, 3)
	secuencia := isam.ExtractField(rec, 10, 5)
	tipoDoc := isam.ExtractField(rec, 15, 1)
	nit := strings.TrimLeft(isam.ExtractField(rec, 16, 13), "0")
	cuenta := isam.ExtractField(rec, 29, 13)
	fecha := isam.ExtractField(rec, 42, 8)
	descripcion := strings.TrimSpace(isam.ExtractField(rec, 93, 40))
	tipoMov := ""

	// tipo_mov is at offset 143 (D or C)
	if len(rec) > 143 {
		mv := rec[143]
		if mv == 'D' || mv == 'C' {
			tipoMov = string(mv)
		}
	}

	// Validate fecha
	if !looksLikeDate(fecha) {
		fecha = ""
	}

	return Cartera{
		TipoRegistro:   string(tipo),
		Empresa:        empresa,
		Secuencia:      secuencia,
		TipoDoc:        tipoDoc,
		NitTercero:     nit,
		CuentaContable: cuenta,
		Fecha:          fecha,
		Descripcion:    descripcion,
		TipoMov:        tipoMov,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

// parseCarteraHeuristic extracts cartera records using scanning (binary reader fallback)
func parseCarteraHeuristic(rec []byte, hash [32]byte) Cartera {
	c := Cartera{
		Hash: fmt.Sprintf("%x", hash[:8]),
	}

	// Try to find date, NIT, description using heuristics
	for i := 0; i < len(rec)-8 && i < 200; i++ {
		if rec[i] == '2' && rec[i+1] == '0' && isDigitRange(rec, i, 8) {
			candidate := isam.ExtractField(rec, i, 8)
			if looksLikeDate(candidate) {
				c.Fecha = candidate
				break
			}
		}
	}

	c.Descripcion = findDescripcion(rec, 0)

	if len(rec) > 1 {
		c.TipoRegistro = isam.ExtractField(rec, 0, 1)
	}

	return c
}

// ToFinearomCartera converts a Cartera entry to a map for the Finearom API
func (c *Cartera) ToFinearomCartera() map[string]interface{} {
	fecha := c.Fecha
	if len(fecha) == 8 {
		fecha = fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
	}

	return map[string]interface{}{
		"nit":              c.NitTercero,
		"cuenta_contable":  c.CuentaContable,
		"fecha":            fecha,
		"tipo_movimiento":  c.TipoMov,
		"descripcion":      c.Descripcion,
		"tipo_registro":    c.TipoRegistro,
		"siigo_sync_hash":  c.Hash,
	}
}
