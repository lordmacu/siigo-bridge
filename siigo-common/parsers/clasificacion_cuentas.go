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

// ClasificacionCuenta represents an account classification/reporting category from Z279CPYY.
// Z279CPYY files map account codes to descriptions for reporting purposes.
type ClasificacionCuenta struct {
	AccountCode string `json:"codigo_cuenta"`  // primary account code (4 chars)
	GroupCode   string `json:"codigo_grupo"`   // secondary group code (4 chars)
	DetailCode  string `json:"codigo_detalle"` // tertiary detail code (4 chars)
	Description string `json:"descripcion"`    // description text (up to 114 chars)
	Hash        string `json:"hash"`
}

// FindLatestZ279CP finds the most recent Z279CPYY file in the data directory.
func FindLatestZ279CP(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z279CP[0-9][0-9]")
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
		year := base[6:] // "Z279CP" is 6 chars, then YY
		return m, year
	}
	return "", ""
}

// ParseClasificacionCuentas reads the latest Z279CPYY file and returns account classifications.
func ParseClasificacionCuentas(dataPath string) ([]ClasificacionCuenta, string, error) {
	path, year := FindLatestZ279CP(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z279CPYY file found in %s", dataPath)
	}
	return ParseClasificacionCuentasFile(path, year)
}

// ParseClasificacionCuentasFile reads a specific Z279CP file.
func ParseClasificacionCuentasFile(path, year string) ([]ClasificacionCuenta, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var accounts []ClasificacionCuenta
	for _, rec := range records {
		c := parseClasificacionRecord(rec, extfh)
		if c.AccountCode == "" || c.Description == "" {
			continue
		}
		accounts = append(accounts, c)
	}

	return accounts, year, nil
}

func parseClasificacionRecord(rec []byte, extfh bool) ClasificacionCuenta {
	if len(rec) < 15 {
		return ClasificacionCuenta{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseClasificacionEXTFH(rec, hash)
	}
	return parseClasificacionBinary(rec, hash)
}

// parseClasificacionEXTFH extracts Z279CPYY records using EXTFH offsets.
// Z279CPYY structure (128 bytes) verified via sample data:
//
//	[0:4]     account code (4 chars, primary account code e.g. "1110")
//	[4:6]     unknown code (2 chars, skipped)
//	[6:10]    group code (4 chars, secondary group code e.g. "1211")
//	[10:14]   detail code (4 chars, tertiary detail code e.g. "1006")
//	[14:128]  description (114 chars, space-padded)
func parseClasificacionEXTFH(rec []byte, hash [32]byte) ClasificacionCuenta {
	accountCode := strings.TrimSpace(isam.ExtractField(rec, 0, 4))
	groupCode := strings.TrimSpace(isam.ExtractField(rec, 6, 4))
	detailCode := strings.TrimSpace(isam.ExtractField(rec, 10, 4))
	description := strings.TrimSpace(isam.ExtractField(rec, 14, 114))

	if accountCode == "" || description == "" {
		return ClasificacionCuenta{}
	}

	return ClasificacionCuenta{
		AccountCode: accountCode,
		GroupCode:   groupCode,
		DetailCode:  detailCode,
		Description: description,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}

// parseClasificacionBinary extracts Z279CPYY records with +2 byte offset for binary mode.
func parseClasificacionBinary(rec []byte, hash [32]byte) ClasificacionCuenta {
	off := 2 // binary mode: 2-byte record marker shifts all offsets

	if len(rec) < off+15 {
		return ClasificacionCuenta{}
	}

	accountCode := strings.TrimSpace(isam.ExtractField(rec, off+0, 4))
	groupCode := strings.TrimSpace(isam.ExtractField(rec, off+6, 4))
	detailCode := strings.TrimSpace(isam.ExtractField(rec, off+10, 4))
	description := strings.TrimSpace(isam.ExtractField(rec, off+14, 114))

	if accountCode == "" || description == "" {
		return ClasificacionCuenta{}
	}

	return ClasificacionCuenta{
		AccountCode: accountCode,
		GroupCode:   groupCode,
		DetailCode:  detailCode,
		Description: description,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}
