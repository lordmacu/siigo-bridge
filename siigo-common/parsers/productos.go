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
	Voucher        string `json:"comprobante"`               // comprobante number (4 digits)
	Sequence       string `json:"secuencia"`                 // sequence within comprobante
	ThirdPartyType string `json:"tipo_tercero"`              // R=recibo, G=general, L=linea
	Group          string `json:"grupo"`                     // inventory group code
	LedgerAccount  string `json:"cuenta_contable,omitempty"` // PUC account
	Date           string `json:"fecha,omitempty"`           // YYYYMMDD
	Name           string `json:"nombre"`                    // product name
	MovType        string `json:"tipo_mov,omitempty"`        // D=debit, C=credit
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
	var products []Producto
	for _, rec := range records {
		p := parseProductoRecord(rec, extfh)
		if p.Name == "" {
			continue
		}
		products = append(products, p)
	}

	return products, nil
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

	voucher := strings.TrimLeft(isam.ExtractField(rec, 0, 5), "0")
	seq := strings.TrimLeft(isam.ExtractField(rec, 5, 5), "0")
	thirdPartyType := isam.ExtractField(rec, 10, 1)
	group := strings.TrimSpace(isam.ExtractField(rec, 11, 3))

	account := strings.TrimSpace(isam.ExtractField(rec, 14, 13))

	dateRaw := isam.ExtractField(rec, 38, 8)
	date := ""
	if looksLikeDate(dateRaw) {
		date = dateRaw
	}

	name := strings.TrimSpace(isam.ExtractField(rec, 46, 40))

	if name == "" {
		name = findDescripcion(rec, 46)
	}

	movType := ""
	if len(rec) > 92 {
		tm := rec[92]
		if tm == 'D' || tm == 'C' {
			movType = string(tm)
		}
	}

	return Producto{
		Voucher:        voucher,
		Sequence:       seq,
		ThirdPartyType: thirdPartyType,
		Group:          group,
		LedgerAccount:  account,
		Date:           date,
		Name:           name,
		MovType:        movType,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseProductoHeuristic(rec []byte, hash [32]byte) Producto {
	name := findDescripcion(rec, 30)
	if name == "" {
		return Producto{}
	}

	return Producto{
		Name: name,
		Hash: fmt.Sprintf("%x", hash[:8]),
	}
}

// ToFinearomProduct converts a Producto to a map for the Finearom API
func (p *Producto) ToFinearomProduct() map[string]interface{} {
	return map[string]interface{}{
		"code":            p.Voucher + "-" + p.Sequence,
		"product_name":    p.Name,
		"grupo":           p.Group,
		"cuenta_contable": p.LedgerAccount,
		"siigo_sync_hash": p.Hash,
	}
}
