package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// Cartera represents a portfolio/accounting entry from Siigo Z09YYYY files.
// Z09 files contain the full accounting detail: NIT, ledger account, date,
// description, movement type (D/C), and amounts (in packed decimal).
type Cartera struct {
	RecordType     string `json:"tipo_registro"`   // F=factura, G=general, L=libro, P=pago, R=recibo
	Company        string `json:"empresa"`          // 001
	Sequence       string `json:"secuencia"`        // sequential record number
	DocType        string `json:"tipo_doc"`         // N=NIT, etc.
	ThirdPartyNit  string `json:"nit_tercero"`      // 13-digit NIT
	LedgerAccount  string `json:"cuenta_contable"`  // 13-digit PUC account
	Date           string `json:"fecha"`            // YYYYMMDD
	Description    string `json:"descripcion"`      // transaction description
	MovType        string `json:"tipo_mov"`         // D=debit, C=credit
	Hash           string `json:"hash"`
}

// FindLatestZ09 finds the most recent Z09YYYY file (excluding variants like Z09CL, Z09D, etc.)
func FindLatestZ09(dataPath string) (string, string) {
	matches, _ := filepath.Glob(dataPath + "Z09[0-9][0-9][0-9][0-9]")
	if len(matches) == 0 {
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

// ParseCarteraLatest reads the latest Z09YYYY file and returns cartera entries.
func ParseCarteraLatest(dataPath string) ([]Cartera, string, error) {
	_, year := FindLatestZ09(dataPath)
	if year == "" {
		return nil, "", fmt.Errorf("no Z09YYYY file found in %s", dataPath)
	}
	items, err := ParseCartera(dataPath, year)
	return items, year, err
}

// ParseCartera reads a Z09YYYY file and returns all cartera entries
func ParseCartera(dataPath string, year string) ([]Cartera, error) {
	path := dataPath + "Z09" + year
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var results []Cartera
	for _, rec := range records {
		c := parseCarteraRecord(rec, extfh)
		if c.ThirdPartyNit == "" && c.Description == "" {
			continue
		}
		results = append(results, c)
	}

	return results, nil
}

// ParseCarteraByTipo returns cartera entries filtered by record type
func ParseCarteraByTipo(dataPath string, year string, recType byte) ([]Cartera, error) {
	all, err := ParseCartera(dataPath, year)
	if err != nil {
		return nil, err
	}
	var filtered []Cartera
	for _, c := range all {
		if len(c.RecordType) > 0 && c.RecordType[0] == recType {
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
//	[0:1]   record type - F/G/L/P/R
//	[1:4]   company - 001
//	[4:9]   padding (nulls)
//	[9:10]  separator (0x1F)
//	[10:15] sequence - 00001
//	[15:16] doc type - N=NIT, etc.
//	[16:29] third-party NIT - 13 chars, zero-padded
//	[29:42] ledger account - 13 chars, PUC account code
//	[42:50] date - YYYYMMDD
//	[50:93] packed decimal data (amounts, counts - binary, not decoded yet)
//	[93:143] description - 50 chars text (extra 10 bytes before D/C@143 confirmed as text)
//	[143:144] movement type - D=debit, C=credit
func parseCarteraEXTFH(rec []byte, hash [32]byte) Cartera {
	recType := rec[0]
	// Valid cartera record types
	if recType != 'F' && recType != 'G' && recType != 'L' && recType != 'P' && recType != 'R' {
		return Cartera{}
	}

	company := isam.ExtractField(rec, 1, 3)
	seq := isam.ExtractField(rec, 10, 5)
	docType := isam.ExtractField(rec, 15, 1)
	nit := strings.TrimLeft(isam.ExtractField(rec, 16, 13), "0")
	account := isam.ExtractField(rec, 29, 13)
	date := isam.ExtractField(rec, 42, 8)
	// Description field: bytes 93-142 (50 chars). Previously extracted only 40 chars,
	// truncating text like "PALACIO" -> "PALACI". Extra 10 bytes before D/C@143 confirmed as text.
	description := strings.TrimSpace(isam.ExtractField(rec, 93, 50))
	movType := ""

	// Movement type is at offset 143 (D or C)
	if len(rec) > 143 {
		mv := rec[143]
		if mv == 'D' || mv == 'C' {
			movType = string(mv)
		}
	}

	// Validate date
	if !looksLikeDate(date) {
		date = ""
	}

	return Cartera{
		RecordType:    string(recType),
		Company:       company,
		Sequence:      seq,
		DocType:       docType,
		ThirdPartyNit: nit,
		LedgerAccount: account,
		Date:          date,
		Description:   description,
		MovType:       movType,
		Hash:          fmt.Sprintf("%x", hash[:8]),
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
				c.Date = candidate
				break
			}
		}
	}

	c.Description = findDescripcion(rec, 0)

	if len(rec) > 1 {
		c.RecordType = isam.ExtractField(rec, 0, 1)
	}

	return c
}

// ToFinearomCartera converts a Cartera entry to a map for the Finearom API
func (c *Cartera) ToFinearomCartera() map[string]interface{} {
	date := c.Date
	if len(date) == 8 {
		date = date[:4] + "-" + date[4:6] + "-" + date[6:8]
	}

	return map[string]interface{}{
		"nit":              c.ThirdPartyNit,
		"cuenta_contable":  c.LedgerAccount,
		"fecha":            date,
		"tipo_movimiento":  c.MovType,
		"descripcion":      c.Description,
		"tipo_registro":    c.RecordType,
		"siigo_sync_hash":  c.Hash,
	}
}
