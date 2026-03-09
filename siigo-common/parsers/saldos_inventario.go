package parsers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"strings"
)

// SaldoInventario represents inventory balance per product from Z15YYYY.
// Each record has the product key and BCD-encoded period balances.
type SaldoInventario struct {
	RecordKey      string  `json:"record_key"`
	Empresa        string  `json:"empresa"`
	Grupo          string  `json:"grupo"`
	CodigoProducto string  `json:"codigo_producto"`
	SaldoInicial   float64 `json:"saldo_inicial"`
	Entradas       float64 `json:"entradas"`
	Salidas        float64 `json:"salidas"`
	SaldoFinal     float64 `json:"saldo_final"`
	Hash           string  `json:"hash"`
}

// FindLatestZ15 finds the Z15YYYY file with most data (largest file size).
func FindLatestZ15(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z15[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
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

// ParseSaldosInventario reads the latest Z15YYYY and returns balance records.
func ParseSaldosInventario(dataPath string) ([]SaldoInventario, string, error) {
	path, year := FindLatestZ15(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z15YYYY file found in %s", dataPath)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("Z15 file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var items []SaldoInventario
	for _, rec := range records {
		item := parseSaldoInvRecord(rec, extfh)
		if item.CodigoProducto == "" {
			continue
		}
		items = append(items, item)
	}
	return items, year, nil
}

// Z15YYYY structure (320 bytes) — verified from hex dump with v2 reader:
//
//	[0:2]     tipo ("06"=periodo)
//	[2:5]     empresa ("001")
//	[5:8]     grupo inventario ("000")
//	[8:14]    codigo producto ("100000")
//	[14:15]   variante ("1"=base)
//	[15:16]   null separator
//	[16:22]   padding/flags
//	[22:23]   BCD sign (0x0C = positive)
//	[23:30]   BCD saldo inicial (7 bytes packed decimal)
//	[30:31]   BCD sign
//	[31:38]   BCD entradas
//	[38:39]   BCD sign
//	[39:46]   BCD saldo campo 2
//	[46:47]   BCD sign
//	... repeating BCD fields for each period (12 periods)
//
// Note: BCD fields use packed decimal with sign nibble C=positive, D=negative, F=unsigned.
// Each BCD group is 7 bytes (sign byte + 6 data bytes) = up to 12 digits.
func parseSaldoInvRecord(rec []byte, extfh bool) SaldoInventario {
	if len(rec) < 50 {
		return SaldoInventario{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseSaldoInvEXTFH(rec, hash)
	}
	return parseSaldoInvEXTFH(rec, hash) // same offsets for v2
}

func parseSaldoInvEXTFH(rec []byte, hash [32]byte) SaldoInventario {
	// Key fields are ASCII at the beginning
	tipo := strings.TrimSpace(isam.ExtractField(rec, 0, 2))
	empresa := strings.TrimSpace(isam.ExtractField(rec, 2, 3))
	grupo := strings.TrimSpace(isam.ExtractField(rec, 5, 3))
	codigo := strings.TrimSpace(isam.ExtractField(rec, 8, 6))

	if codigo == "" || codigo == "000000" {
		return SaldoInventario{}
	}

	// BCD fields: separate sign byte (0x0C=positive, 0x0D=negative) + 7 bytes packed data
	// Sign bytes at 24, 32, 42; data at 25, 33, 43
	saldoInicial := extractBCDValueSigned(rec, 24, 25, 7)
	entradas := extractBCDValueSigned(rec, 32, 33, 7)
	salidas := extractBCDValueSigned(rec, 42, 43, 7)
	saldoFinal := saldoInicial + entradas - salidas

	hashStr := fmt.Sprintf("%x", hash[:8])
	key := fmt.Sprintf("%s-%s-%s-%s-%s", tipo, empresa, grupo, codigo, hashStr[:6])

	return SaldoInventario{
		RecordKey:      key,
		Empresa:        empresa,
		Grupo:          grupo,
		CodigoProducto: codigo,
		SaldoInicial:   saldoInicial,
		Entradas:       entradas,
		Salidas:        salidas,
		SaldoFinal:     saldoFinal,
		Hash:           hashStr,
	}
}

// extractBCDValueSigned extracts a packed decimal with separate sign byte.
// Z15 uses sign byte (0x0C=positive, 0x0D=negative) before the BCD data.
func extractBCDValueSigned(rec []byte, signOffset, dataOffset, length int) float64 {
	if dataOffset+length > len(rec) || signOffset >= len(rec) {
		return 0
	}
	value := DecodePacked(rec[dataOffset:dataOffset+length], 2)
	if rec[signOffset] == 0x0D {
		value = -value
	}
	return value
}
