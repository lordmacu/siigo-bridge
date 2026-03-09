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
	Company       string  `json:"empresa"`          // company code (3 chars)
	LedgerAccount string  `json:"cuenta_contable"`  // PUC account code (up to 9 digits)
	PrevBalance   float64 `json:"saldo_anterior"`   // previous balance (BCD)
	Debit         float64 `json:"debito"`           // debit total (BCD)
	Credit        float64 `json:"credito"`          // credit total (BCD)
	FinalBalance  float64 `json:"saldo_final"`      // calculated: anterior + debito - credito
	Hash          string  `json:"hash"`
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
	var balances []SaldoConsolidado
	for _, rec := range records {
		s := parseSaldoConsolidadoRecord(rec, extfh)
		if s.LedgerAccount == "" {
			continue
		}
		balances = append(balances, s)
	}

	return balances, year, nil
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
	company := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	accountRaw := strings.TrimSpace(isam.ExtractField(rec, 3, 9))

	if accountRaw == "" {
		return SaldoConsolidado{}
	}

	// Validate account is numeric
	allZeros := true
	for _, c := range accountRaw {
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

	var prevBalance, debit, credit float64
	if len(rec) >= 62 {
		prevBalance = DecodePacked(rec[38:46], 2)
		debit = DecodePacked(rec[46:54], 2)
		credit = DecodePacked(rec[54:62], 2)
	}

	finalBalance := prevBalance + debit - credit

	return SaldoConsolidado{
		Company:       company,
		LedgerAccount: accountRaw,
		PrevBalance:   prevBalance,
		Debit:         debit,
		Credit:        credit,
		FinalBalance:  finalBalance,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}

func parseSaldoConsolidadoHeuristic(rec []byte, hash [32]byte) SaldoConsolidado {
	account := findDigitSequence(rec, 3, 9)
	if account == "" {
		return SaldoConsolidado{}
	}
	return SaldoConsolidado{
		LedgerAccount: account,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}
