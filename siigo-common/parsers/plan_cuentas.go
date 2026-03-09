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

// CuentaContable represents an account from the PUC (Plan Unico de Cuentas) in Z03YYYY.
// Z03YYYY files are yearly snapshots of the chart of accounts.
type CuentaContable struct {
	Company     string `json:"empresa"`         // company code (3 chars)
	AccountCode string `json:"codigo_cuenta"`   // PUC code (up to 9 digits)
	Name        string `json:"nombre"`          // account name (70 chars)
	Active      bool   `json:"activa"`          // S=active, N=inactive
	Auxiliary   bool   `json:"auxiliar"`         // S=has sub-accounts
	Nature      string `json:"naturaleza"`      // account nature flags
	Hash        string `json:"hash"`
}

// FindLatestZ03 finds the most recent Z03YYYY file in the data directory.
func FindLatestZ03(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z03[0-9][0-9][0-9][0-9]")
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

// ParsePlanCuentas reads the latest Z03YYYY file and returns chart of accounts.
func ParsePlanCuentas(dataPath string) ([]CuentaContable, string, error) {
	path, year := FindLatestZ03(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z03YYYY file found in %s", dataPath)
	}
	return ParsePlanCuentasFile(path, year)
}

// ParsePlanCuentasFile reads a specific Z03 file.
func ParsePlanCuentasFile(path, year string) ([]CuentaContable, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var accounts []CuentaContable
	for _, rec := range records {
		c := parseCuentaRecord(rec, extfh)
		if c.AccountCode == "" || c.Name == "" {
			continue
		}
		accounts = append(accounts, c)
	}

	return accounts, year, nil
}

func parseCuentaRecord(rec []byte, extfh bool) CuentaContable {
	if len(rec) < 95 {
		return CuentaContable{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseCuentaEXTFH(rec, hash)
	}
	return parseCuentaHeuristic(rec, hash)
}

// parseCuentaEXTFH extracts Z03YYYY records using EXTFH offsets.
// Z03YYYY structure (1152 bytes) verified via hex dump:
//
//	[0:3]    empresa (3 chars)
//	[3:12]   codigo_cuenta (9 digits, PUC code)
//	[12:13]  activa (S/N)
//	[13:14]  auxiliar (S/N)
//	[17:25]  flags naturaleza (8 x S/N)
//	[25:95]  nombre (70 chars)
func parseCuentaEXTFH(rec []byte, hash [32]byte) CuentaContable {
	company := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	codeRaw := strings.TrimSpace(isam.ExtractField(rec, 3, 9))

	// Remove trailing zeros to get clean PUC code
	code := strings.TrimRight(codeRaw, "0")
	if code == "" {
		return CuentaContable{}
	}
	// Keep full code for uniqueness
	code = codeRaw

	active := isam.ExtractField(rec, 12, 1) == "S"
	auxiliary := isam.ExtractField(rec, 13, 1) == "S"

	// Extract nature flags
	nature := ""
	for j := 17; j < 25 && j < len(rec); j++ {
		if rec[j] == 'S' || rec[j] == 'N' {
			nature += string(rec[j])
		}
	}

	name := strings.TrimSpace(isam.ExtractField(rec, 25, 70))
	if name == "" {
		return CuentaContable{}
	}

	return CuentaContable{
		Company:     company,
		AccountCode: code,
		Name:        name,
		Active:      active,
		Auxiliary:   auxiliary,
		Nature:      nature,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}

func parseCuentaHeuristic(rec []byte, hash [32]byte) CuentaContable {
	name := findDescripcion(rec, 25)
	if name == "" {
		return CuentaContable{}
	}
	return CuentaContable{
		Name: name,
		Hash: fmt.Sprintf("%x", hash[:8]),
	}
}
