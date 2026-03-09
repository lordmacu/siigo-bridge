package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// LibroAuxiliar represents an entry in the auxiliary ledger (Z07YYYY).
// Each record is a transaction line in the auxiliary books.
type LibroAuxiliar struct {
	Company        string  `json:"empresa"`
	LedgerAccount  string  `json:"cuenta_contable"`     // PUC account (9 digits)
	VoucherType    string  `json:"tipo_comprobante"`    // F=invoice, P=payment, G=expense, R=receipt, L=adjustment
	VoucherCode    string  `json:"codigo_comprobante"`
	DocDate        string  `json:"fecha_documento"`     // YYYYMMDD
	ThirdPartyNit  string  `json:"nit_tercero"`
	RefNumber      string  `json:"numero_referencia"`
	SecVoucherType string  `json:"tipo_comp_sec"`       // secondary/counter type
	SecVoucherCode string  `json:"codigo_comp_sec"`
	Balance        float64 `json:"saldo"`
	Debit          float64 `json:"debito"`
	Credit         float64 `json:"credito"`
	RegDate        string  `json:"fecha_registro"`      // YYYYMMDD
	Hash           string  `json:"hash"`
}

// ParseLibrosAuxiliares reads the latest Z07YYYY file and returns auxiliary ledger entries.
func ParseLibrosAuxiliares(dataPath string) ([]LibroAuxiliar, string, error) {
	path, year := findLatestZ07(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z07YYYY file found in %s", dataPath)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var result []LibroAuxiliar
	for _, rec := range records {
		r := parseLibroAuxiliar(rec, extfh)
		if r.LedgerAccount == "" {
			continue
		}
		result = append(result, r)
	}

	return result, year, nil
}

func parseLibroAuxiliar(rec []byte, extfh bool) LibroAuxiliar {
	if len(rec) < 140 {
		return LibroAuxiliar{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseLibroAuxiliarEXTFH(rec, hash)
	}
	return parseLibroAuxiliarBinary(rec, hash)
}

func parseLibroAuxiliarEXTFH(rec []byte, hash [32]byte) LibroAuxiliar {
	company := strings.TrimSpace(isam.ExtractField(rec, 7, 3))
	account := strings.TrimSpace(isam.ExtractField(rec, 10, 9))
	voucherType := strings.TrimSpace(isam.ExtractField(rec, 20, 1))
	voucherCode := strings.TrimSpace(isam.ExtractField(rec, 21, 3))
	docDate := strings.TrimSpace(isam.ExtractField(rec, 33, 8))
	nit := strings.TrimSpace(isam.ExtractField(rec, 41, 13))
	regDate := strings.TrimSpace(isam.ExtractField(rec, 133, 8))
	refNum := strings.TrimSpace(isam.ExtractField(rec, 144, 7))
	secVoucherType := strings.TrimSpace(isam.ExtractField(rec, 155, 1))
	secVoucherCode := strings.TrimSpace(isam.ExtractField(rec, 156, 3))

	// Strip leading zeros from NIT and ref
	nit = strings.TrimLeft(nit, "0")
	refNum = strings.TrimLeft(refNum, "0")

	// BCD values
	var balance, debit, credit float64
	if len(rec) >= 131 {
		balance = DecodePacked(rec[112:118], 2)
		debit = DecodePacked(rec[118:124], 2)
		credit = DecodePacked(rec[124:131], 2)
	}

	return LibroAuxiliar{
		Company:        company,
		LedgerAccount:  account,
		VoucherType:    voucherType,
		VoucherCode:    voucherCode,
		DocDate:        docDate,
		ThirdPartyNit:  nit,
		RefNumber:      refNum,
		SecVoucherType: secVoucherType,
		SecVoucherCode: secVoucherCode,
		Balance:        balance,
		Debit:          debit,
		Credit:         credit,
		RegDate:        regDate,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseLibroAuxiliarBinary(rec []byte, hash [32]byte) LibroAuxiliar {
	// Binary fallback: offsets +2 for record markers
	company := strings.TrimSpace(isam.ExtractField(rec, 9, 3))
	account := strings.TrimSpace(isam.ExtractField(rec, 12, 9))
	voucherType := strings.TrimSpace(isam.ExtractField(rec, 22, 1))
	voucherCode := strings.TrimSpace(isam.ExtractField(rec, 23, 3))
	docDate := strings.TrimSpace(isam.ExtractField(rec, 35, 8))
	nit := strings.TrimSpace(isam.ExtractField(rec, 43, 13))
	regDate := strings.TrimSpace(isam.ExtractField(rec, 135, 8))
	refNum := strings.TrimSpace(isam.ExtractField(rec, 146, 7))
	secVoucherType := strings.TrimSpace(isam.ExtractField(rec, 157, 1))
	secVoucherCode := strings.TrimSpace(isam.ExtractField(rec, 158, 3))

	nit = strings.TrimLeft(nit, "0")
	refNum = strings.TrimLeft(refNum, "0")

	var balance, debit, credit float64
	if len(rec) >= 133 {
		balance = DecodePacked(rec[114:120], 2)
		debit = DecodePacked(rec[120:126], 2)
		credit = DecodePacked(rec[126:133], 2)
	}

	return LibroAuxiliar{
		Company:        company,
		LedgerAccount:  account,
		VoucherType:    voucherType,
		VoucherCode:    voucherCode,
		DocDate:        docDate,
		ThirdPartyNit:  nit,
		RefNumber:      refNum,
		SecVoucherType: secVoucherType,
		SecVoucherCode: secVoucherCode,
		Balance:        balance,
		Debit:          debit,
		Credit:         credit,
		RegDate:        regDate,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func findLatestZ07(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z07[0-9][0-9][0-9][0-9]")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", ""
	}
	// Filter out .idx files and special codes (7777, 9999, etc)
	var valid []string
	for _, m := range matches {
		if strings.HasSuffix(m, ".idx") {
			continue
		}
		year := m[len(m)-4:]
		if year >= "1990" && year <= "2099" {
			valid = append(valid, m)
		}
	}
	if len(valid) == 0 {
		return "", ""
	}
	sort.Strings(valid)
	best := valid[len(valid)-1]
	year := best[len(best)-4:]
	return best, year
}
