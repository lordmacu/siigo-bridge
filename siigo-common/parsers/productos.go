package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Producto represents a product/item from Siigo Z06CP file.
// Z06CP contains actual products with names, account codes, and pricing.
type Producto struct {
	Comprobante    string `json:"comprobante"`               // comprobante number (4 digits)
	Secuencia      string `json:"secuencia"`                 // sequence within comprobante
	TipoTercero    string `json:"tipo_tercero"`              // R=recibo, G=general, L=linea
	Grupo          string `json:"grupo"`                     // inventory group code
	CuentaContable string `json:"cuenta_contable,omitempty"` // PUC account
	Fecha          string `json:"fecha,omitempty"`           // YYYYMMDD
	Nombre         string `json:"nombre"`                    // product name
	TipoMov        string `json:"tipo_mov,omitempty"`        // D=debit, C=credit
	Hash           string `json:"hash"`
}

// ParseProductos reads the Z06CP file and returns product records.
func ParseProductos(dataPath string) ([]Producto, error) {
	path := dataPath + "Z06CP"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var productos []Producto
	for _, rec := range records {
		p := parseProductoRecord(rec, extfh)
		if p.Nombre == "" {
			continue
		}
		productos = append(productos, p)
	}

	return productos, nil
}

func parseProductoRecord(rec []byte, extfh bool) Producto {
	if len(rec) < 60 {
		return Producto{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseProductoEXTFH(rec, hash)
	}
	return parseProductoHeuristic(rec, hash)
}

// parseProductoEXTFH extracts Z06CP records using EXTFH offsets.
// Z06CP structure (2036 bytes) verified via hex dump:
//
//	[0:5]    comprobante - 5 digit number (00001, 00002, etc.)
//	[5:10]   secuencia - 5 digit sequence (00001, 00002, etc.)
//	[10:11]  tipo_tercero - R/G/L
//	[11:14]  grupo - inventory group (001, 002, 999)
//	[14:27]  cuenta_contable - PUC account (13 chars)
//	[27:38]  numeric data
//	[38:46]  fecha - YYYYMMDD (e.g., 20121031)
//	[46:86]  nombre - product description (40 chars)
//	[92:93]  tipo_mov - D/C
func parseProductoEXTFH(rec []byte, hash [32]byte) Producto {
	if len(rec) < 86 {
		return Producto{}
	}

	comprobante := strings.TrimLeft(isam.ExtractField(rec, 0, 5), "0")
	secuencia := strings.TrimLeft(isam.ExtractField(rec, 5, 5), "0")
	tipoTercero := isam.ExtractField(rec, 10, 1)
	grupo := strings.TrimSpace(isam.ExtractField(rec, 11, 3))

	cuenta := strings.TrimSpace(isam.ExtractField(rec, 14, 13))

	fechaRaw := isam.ExtractField(rec, 38, 8)
	fecha := ""
	if looksLikeDate(fechaRaw) {
		fecha = fechaRaw
	}

	nombre := strings.TrimSpace(isam.ExtractField(rec, 46, 40))

	if nombre == "" {
		nombre = findDescripcion(rec, 46)
	}

	tipoMov := ""
	if len(rec) > 92 {
		tm := rec[92]
		if tm == 'D' || tm == 'C' {
			tipoMov = string(tm)
		}
	}

	return Producto{
		Comprobante:    comprobante,
		Secuencia:      secuencia,
		TipoTercero:    tipoTercero,
		Grupo:          grupo,
		CuentaContable: cuenta,
		Fecha:          fecha,
		Nombre:         nombre,
		TipoMov:        tipoMov,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseProductoHeuristic(rec []byte, hash [32]byte) Producto {
	nombre := findDescripcion(rec, 30)
	if nombre == "" {
		return Producto{}
	}

	return Producto{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}

// ToFinearomProduct converts a Producto to a map for the Finearom API
func (p *Producto) ToFinearomProduct() map[string]interface{} {
	return map[string]interface{}{
		"code":            p.Comprobante + "-" + p.Secuencia,
		"product_name":    p.Nombre,
		"grupo":           p.Grupo,
		"cuenta_contable": p.CuentaContable,
		"siigo_sync_hash": p.Hash,
	}
}
