package parsers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"strings"
)

// MovimientoInventario represents an inventory movement from Z16YYYY.
// These are physical inventory transactions: entries, exits, transfers.
type MovimientoInventario struct {
	RecordKey   string `json:"record_key"`
	Company     string `json:"empresa"`
	Group       string `json:"grupo"`
	ProductCode string `json:"codigo_producto"`
	VoucherType string `json:"tipo_comprobante"` // E=entry, L=settlement, P=order
	VoucherCode string `json:"codigo_comp"`
	Sequence    string `json:"secuencia"`
	DocType     string `json:"tipo_doc"`
	Date        string `json:"fecha"`
	Quantity    string `json:"cantidad"`
	Amount      string `json:"valor"`
	MovType     string `json:"tipo_mov"` // D=debit, C=credit
	Hash        string `json:"hash"`
}

// FindLatestZ16 finds the Z16YYYY file with most data (largest file size).
func FindLatestZ16(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z16[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	// Pick the largest file (most data), not just the latest year
	var bestPath string
	var bestYear string
	var bestSize int64
	for _, m := range matches {
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestPath = m
			bestYear = filepath.Base(m)[3:]
		}
	}
	return bestPath, bestYear
}

// ParseMovimientosInventario reads the latest Z16YYYY and returns records.
func ParseMovimientosInventario(dataPath string) ([]MovimientoInventario, string, error) {
	path, year := FindLatestZ16(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z16YYYY file found in %s", dataPath)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("Z16 file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var items []MovimientoInventario
	for _, rec := range records {
		item := parseMovInvRecord(rec, extfh)
		if item.ProductCode == "" {
			continue
		}
		items = append(items, item)
	}
	return items, year, nil
}

// Z16YYYY structure (320 bytes) - verified from hex dump with v2 reader:
//
//	[0:3]     company ("001")
//	[3:6]     inventory group ("000")
//	[6:7]     variant
//	[7:13]    product code ("100000")
//	[13:14]   voucher type (E=entry, L=settlement, P=order)
//	[14:17]   voucher code ("001", "999")
//	[17:18]   null separator
//	[23:26]   line sequence
//	[26:28]   document type ("06"=invoice, "07"=note)
//	[28:31]   company ref
//	[31:37]   product ref
//	[37:44]   padding/flags
//	[44:52]   date YYYYMMDD
//	[52:68]   quantity (ASCII numeric, 16 chars)
//	[68:84]   quantity2 (auxiliary)
//	[78:94]   quantity3
//	[94:95]   separator
//	[95:96]   space
//	[96:103]  padding
//	[103:104] D/C (D=debit/exit, C=credit/entry)
//	[104:119] amount (ASCII numeric, 15 chars)
func parseMovInvRecord(rec []byte, extfh bool) MovimientoInventario {
	if len(rec) < 120 {
		return MovimientoInventario{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseMovInvEXTFH(rec, hash)
	}
	return parseMovInvEXTFH(rec, hash) // same offsets for v2 reader
}

func parseMovInvEXTFH(rec []byte, hash [32]byte) MovimientoInventario {
	company := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	group := strings.TrimSpace(isam.ExtractField(rec, 3, 3))
	code := strings.TrimSpace(isam.ExtractField(rec, 7, 6))

	if code == "" || code == "000000" {
		return MovimientoInventario{}
	}

	voucherType := strings.TrimSpace(isam.ExtractField(rec, 13, 1))
	voucherCode := strings.TrimSpace(isam.ExtractField(rec, 14, 3))
	seq := strings.TrimSpace(isam.ExtractField(rec, 23, 3))
	docType := strings.TrimSpace(isam.ExtractField(rec, 26, 2))
	date := strings.TrimSpace(isam.ExtractField(rec, 44, 8))
	quantity := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 52, 16)), "0")
	movType := strings.TrimSpace(isam.ExtractField(rec, 103, 1))
	amount := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 104, 15)), "0")

	if quantity == "" {
		quantity = "0"
	}
	if amount == "" {
		amount = "0"
	}

	// Build unique key (hash suffix for records with same logical key)
	hashStr := fmt.Sprintf("%x", hash[:8])
	key := fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s", company, group, code, voucherType+voucherCode, seq, date, hashStr[:6])

	return MovimientoInventario{
		RecordKey:   key,
		Company:     company,
		Group:       group,
		ProductCode: code,
		VoucherType: voucherType,
		VoucherCode: voucherCode,
		Sequence:    seq,
		DocType:     docType,
		Date:        date,
		Quantity:    quantity,
		Amount:      amount,
		MovType:     movType,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}
