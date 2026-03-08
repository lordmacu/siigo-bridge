package parsers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// SaldoTercero represents a balance per account and third-party from Z25YYYY.
// Z25YYYY files contain account-NIT pairs with BCD-encoded monetary values.
type SaldoTercero struct {
	Empresa        string  `json:"empresa"`          // company code (3 chars)
	CuentaContable string  `json:"cuenta_contable"`  // PUC account code (up to 9 digits)
	NitTercero     string  `json:"nit_tercero"`      // third-party NIT (13 chars)
	SaldoAnterior  float64 `json:"saldo_anterior"`   // previous balance (BCD)
	Debito         float64 `json:"debito"`           // debit amount (BCD)
	Credito        float64 `json:"credito"`          // credit amount (BCD)
	SaldoFinal     float64 `json:"saldo_final"`      // final balance (calculated)
	Hash           string  `json:"hash"`
}

// FindLatestZ25 finds the most recent Z25YYYY file.
func FindLatestZ25(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z25[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, m := range matches {
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		base := filepath.Base(m)
		year := base[3:]
		return m, year
	}
	return "", ""
}

// ParseSaldosTerceros reads the latest Z25YYYY file and returns account-NIT balances.
func ParseSaldosTerceros(dataPath string) ([]SaldoTercero, string, error) {
	path, year := FindLatestZ25(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z25YYYY file found in %s", dataPath)
	}
	return ParseSaldosTercerosFile(path, year)
}

// ParseSaldosTercerosFile reads a specific Z25 file.
func ParseSaldosTercerosFile(path, year string) ([]SaldoTercero, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var saldos []SaldoTercero
	for _, rec := range records {
		s := parseSaldoTerceroRecord(rec, extfh)
		if s.CuentaContable == "" || s.NitTercero == "" {
			continue
		}
		saldos = append(saldos, s)
	}

	return saldos, year, nil
}

func parseSaldoTerceroRecord(rec []byte, extfh bool) SaldoTercero {
	if len(rec) < 50 {
		return SaldoTercero{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseSaldoTerceroEXTFH(rec, hash)
	}
	return parseSaldoTerceroHeuristic(rec, hash)
}

// parseSaldoTerceroEXTFH extracts Z25YYYY records using EXTFH offsets.
// Z25YYYY structure (512 bytes) verified via hex dump 2026-03-08:
//
//	[0:3]     empresa (3 chars)
//	[3:12]    cuenta_contable (9 digits, PUC code)
//	[12:25]   nit_tercero (13 chars, zero-padded left)
//	[25:140]  repeated key data (ASCII, skip)
//	[140:148] BCD saldo_anterior (8 bytes packed decimal, 2 decimals)
//	[148:156] BCD debito (8 bytes packed decimal, 2 decimals)
//	[156:164] BCD credito (8 bytes packed decimal, 2 decimals)
func parseSaldoTerceroEXTFH(rec []byte, hash [32]byte) SaldoTercero {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	cuentaRaw := strings.TrimSpace(isam.ExtractField(rec, 3, 9))
	nit := strings.TrimSpace(isam.ExtractField(rec, 12, 13))
	nit = strings.TrimLeft(nit, "0")

	if cuentaRaw == "" || nit == "" {
		return SaldoTercero{}
	}

	// Try to decode BCD monetary values
	var saldoAnt, debito, credito float64
	if len(rec) >= 164 {
		saldoAnt = DecodePacked(rec[140:148], 2)
		debito = DecodePacked(rec[148:156], 2)
		credito = DecodePacked(rec[156:164], 2)
	}

	saldoFinal := saldoAnt + debito - credito

	return SaldoTercero{
		Empresa:        empresa,
		CuentaContable: cuentaRaw,
		NitTercero:     nit,
		SaldoAnterior:  saldoAnt,
		Debito:         debito,
		Credito:        credito,
		SaldoFinal:     saldoFinal,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseSaldoTerceroHeuristic(rec []byte, hash [32]byte) SaldoTercero {
	// Heuristic: look for digit sequences that could be cuenta and NIT
	cuenta := findDigitSequence(rec, 3, 9)
	nit := findDigitSequence(rec, 12, 13)
	if cuenta == "" || nit == "" {
		return SaldoTercero{}
	}
	return SaldoTercero{
		CuentaContable: cuenta,
		NitTercero:     strings.TrimLeft(nit, "0"),
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

// findDigitSequence extracts a sequence of digits starting at offset.
func findDigitSequence(rec []byte, offset, length int) string {
	if offset+length > len(rec) {
		return ""
	}
	s := string(rec[offset : offset+length])
	s = strings.TrimSpace(s)
	// Verify it's mostly digits
	digits := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
		}
	}
	if digits == 0 {
		return ""
	}
	return s
}
