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
	Empresa       string `json:"empresa"`         // company code (3 chars)
	CodigoCuenta  string `json:"codigo_cuenta"`   // PUC code (up to 9 digits)
	Nombre        string `json:"nombre"`          // account name (70 chars)
	Activa        bool   `json:"activa"`          // S=active, N=inactive
	Auxiliar      bool   `json:"auxiliar"`         // S=has sub-accounts
	Naturaleza    string `json:"naturaleza"`      // account nature flags
	Hash          string `json:"hash"`
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
	var cuentas []CuentaContable
	for _, rec := range records {
		c := parseCuentaRecord(rec, extfh)
		if c.CodigoCuenta == "" || c.Nombre == "" {
			continue
		}
		cuentas = append(cuentas, c)
	}

	return cuentas, year, nil
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
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	codigoRaw := strings.TrimSpace(isam.ExtractField(rec, 3, 9))

	// Remove trailing zeros to get clean PUC code
	codigo := strings.TrimRight(codigoRaw, "0")
	if codigo == "" {
		return CuentaContable{}
	}
	// Keep full code for uniqueness
	codigo = codigoRaw

	activa := isam.ExtractField(rec, 12, 1) == "S"
	auxiliar := isam.ExtractField(rec, 13, 1) == "S"

	// Extract nature flags
	naturaleza := ""
	for j := 17; j < 25 && j < len(rec); j++ {
		if rec[j] == 'S' || rec[j] == 'N' {
			naturaleza += string(rec[j])
		}
	}

	nombre := strings.TrimSpace(isam.ExtractField(rec, 25, 70))
	if nombre == "" {
		return CuentaContable{}
	}

	return CuentaContable{
		Empresa:      empresa,
		CodigoCuenta: codigo,
		Nombre:       nombre,
		Activa:       activa,
		Auxiliar:     auxiliar,
		Naturaleza:   naturaleza,
		Hash:         fmt.Sprintf("%x", hash[:8]),
	}
}

func parseCuentaHeuristic(rec []byte, hash [32]byte) CuentaContable {
	nombre := findDescripcion(rec, 25)
	if nombre == "" {
		return CuentaContable{}
	}
	return CuentaContable{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}
