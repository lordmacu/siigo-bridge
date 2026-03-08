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

// ActivoFijo represents a fixed asset from Siigo Z27YYYY.
type ActivoFijo struct {
	Empresa          string `json:"empresa"`           // company code (5 chars)
	Codigo           string `json:"codigo"`            // asset code (6 chars)
	Nombre           string `json:"nombre"`            // asset name (50 chars)
	NitResponsable   string `json:"nit_responsable"`   // NIT of responsible person (13 chars)
	FechaAdquisicion string `json:"fecha_adquisicion"` // YYYYMMDD
	Hash             string `json:"hash"`
}

// FindLatestZ27 finds the most recent Z27YYYY file.
func FindLatestZ27(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z27[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	// Filter out Z27YYYYA (companion files)
	var filtered []string
	for _, m := range matches {
		base := filepath.Base(m)
		if len(base) == 7 && !strings.HasSuffix(strings.ToLower(m), ".idx") {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return "", ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(filtered)))
	base := filepath.Base(filtered[0])
	year := base[3:]
	return filtered[0], year
}

// ParseActivosFijos reads the latest Z27YYYY file and returns fixed assets.
func ParseActivosFijos(dataPath string) ([]ActivoFijo, string, error) {
	path, year := FindLatestZ27(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z27YYYY file found in %s", dataPath)
	}
	return ParseActivosFijosFile(path, year)
}

// ParseActivosFijosFile reads a specific Z27 file.
func ParseActivosFijosFile(path, year string) ([]ActivoFijo, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var activos []ActivoFijo
	for _, rec := range records {
		a := parseActivoRecord(rec, extfh)
		if a.Codigo == "" || a.Nombre == "" {
			continue
		}
		activos = append(activos, a)
	}

	return activos, year, nil
}

func parseActivoRecord(rec []byte, extfh bool) ActivoFijo {
	if len(rec) < 61 {
		return ActivoFijo{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseActivoEXTFH(rec, hash)
	}
	return parseActivoHeuristic(rec, hash)
}

// parseActivoEXTFH extracts Z27YYYY records using EXTFH offsets.
// Z27YYYY structure (2048 bytes) verified via hex dump:
//
//	[0:5]     empresa (5 chars)
//	[5:11]    codigo (6 chars)
//	[11:61]   nombre (50 chars)
//	[61:74]   nit_responsable (13 chars)
//	[74:84]   campo numerico
//	[84:104]  espacios
//	[104:118] campo numerico
//	[118:122] espacios
//	[122:130] fecha_adquisicion (YYYYMMDD)
func parseActivoEXTFH(rec []byte, hash [32]byte) ActivoFijo {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
	codigo := strings.TrimSpace(isam.ExtractField(rec, 5, 6))
	nombre := strings.TrimSpace(isam.ExtractField(rec, 11, 50))

	if nombre == "" || codigo == "" {
		return ActivoFijo{}
	}

	nit := strings.TrimSpace(isam.ExtractField(rec, 61, 13))
	nit = strings.TrimLeft(nit, "0")

	fecha := ""
	if len(rec) >= 130 {
		fechaRaw := isam.ExtractField(rec, 122, 8)
		if looksLikeDate(fechaRaw) {
			fecha = fechaRaw
		}
	}

	return ActivoFijo{
		Empresa:          empresa,
		Codigo:           codigo,
		Nombre:           nombre,
		NitResponsable:   nit,
		FechaAdquisicion: fecha,
		Hash:             fmt.Sprintf("%x", hash[:8]),
	}
}

func parseActivoHeuristic(rec []byte, hash [32]byte) ActivoFijo {
	nombre := findDescripcion(rec, 11)
	if nombre == "" {
		return ActivoFijo{}
	}
	return ActivoFijo{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}
