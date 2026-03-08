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

// SaldoConsolidado represents a consolidated balance per account from Z28YYYY.
// Z28YYYY files contain per-account totals without NIT breakdown.
type SaldoConsolidado struct {
	Empresa        string  `json:"empresa"`          // company code (3 chars)
	CuentaContable string  `json:"cuenta_contable"`  // PUC account code (up to 9 digits)
	SaldoAnterior  float64 `json:"saldo_anterior"`   // previous balance (BCD)
	Debito         float64 `json:"debito"`           // debit total (BCD)
	Credito        float64 `json:"credito"`          // credit total (BCD)
	SaldoFinal     float64 `json:"saldo_final"`      // calculated: anterior + debito - credito
	Hash           string  `json:"hash"`
}

// FindLatestZ28 finds the most recent Z28YYYY file.
func FindLatestZ28(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z28[0-9][0-9][0-9][0-9]")
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

// ParseSaldosConsolidados reads the latest Z28YYYY file and returns consolidated balances.
func ParseSaldosConsolidados(dataPath string) ([]SaldoConsolidado, string, error) {
	path, year := FindLatestZ28(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z28YYYY file found in %s", dataPath)
	}
	return ParseSaldosConsolidadosFile(path, year)
}

// ParseSaldosConsolidadosFile reads a specific Z28 file.
func ParseSaldosConsolidadosFile(path, year string) ([]SaldoConsolidado, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var saldos []SaldoConsolidado
	for _, rec := range records {
		s := parseSaldoConsolidadoRecord(rec, extfh)
		if s.CuentaContable == "" {
			continue
		}
		saldos = append(saldos, s)
	}

	return saldos, year, nil
}

func parseSaldoConsolidadoRecord(rec []byte, extfh bool) SaldoConsolidado {
	if len(rec) < 40 {
		return SaldoConsolidado{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseSaldoConsolidadoEXTFH(rec, hash)
	}
	return parseSaldoConsolidadoHeuristic(rec, hash)
}

// parseSaldoConsolidadoEXTFH extracts Z28YYYY records using EXTFH offsets.
// Z28YYYY structure (512 bytes) verified via hex dump 2026-03-08:
//
//	[0:3]     empresa (3 chars)
//	[3:12]    cuenta_contable (9 digits, PUC code)
//	[12:38]   repeated key data (ASCII, skip)
//	[38:46]   BCD saldo_anterior (8 bytes packed decimal, 2 decimals)
//	[46:54]   BCD debito (8 bytes packed decimal, 2 decimals)
//	[54:62]   BCD credito (8 bytes packed decimal, 2 decimals)
//	[62+]     monthly BCD values (12 months × debito/credito pairs)
func parseSaldoConsolidadoEXTFH(rec []byte, hash [32]byte) SaldoConsolidado {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	cuentaRaw := strings.TrimSpace(isam.ExtractField(rec, 3, 9))

	if cuentaRaw == "" {
		return SaldoConsolidado{}
	}

	// Validate cuenta is numeric
	allZeros := true
	for _, c := range cuentaRaw {
		if c < '0' || c > '9' {
			return SaldoConsolidado{}
		}
		if c != '0' {
			allZeros = false
		}
	}
	if allZeros {
		return SaldoConsolidado{}
	}

	var saldoAnt, debito, credito float64
	if len(rec) >= 62 {
		saldoAnt = DecodePacked(rec[38:46], 2)
		debito = DecodePacked(rec[46:54], 2)
		credito = DecodePacked(rec[54:62], 2)
	}

	saldoFinal := saldoAnt + debito - credito

	return SaldoConsolidado{
		Empresa:        empresa,
		CuentaContable: cuentaRaw,
		SaldoAnterior:  saldoAnt,
		Debito:         debito,
		Credito:        credito,
		SaldoFinal:     saldoFinal,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseSaldoConsolidadoHeuristic(rec []byte, hash [32]byte) SaldoConsolidado {
	cuenta := findDigitSequence(rec, 3, 9)
	if cuenta == "" {
		return SaldoConsolidado{}
	}
	return SaldoConsolidado{
		CuentaContable: cuenta,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}
