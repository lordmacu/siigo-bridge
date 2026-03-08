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

// HistorialDoc represents a document history entry from Z18YYYY.
// Z18 contains document transaction history with company names and dates.
type HistorialDoc struct {
	TipoRegistro string `json:"tipo_registro"` // record type flag
	SubTipo      string `json:"sub_tipo"`       // SF, PRE, etc.
	Empresa      string `json:"empresa"`        // company code
	Fecha        string `json:"fecha"`          // YYYYMMDD
	NombreOrigen string `json:"nombre_origen"`  // originator company name
	NombreDestin string `json:"nombre_destin"`  // destination company name
	NitOrigen    string `json:"nit_origen"`      // originator NIT
	Hash         string `json:"hash"`
}

// FindLatestZ18 finds the most recent Z18YYYY file.
func FindLatestZ18(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z18[0-9][0-9][0-9][0-9]")
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
		if len(base) != 7 {
			continue
		}
		year := base[3:]
		return m, year
	}
	return "", ""
}

// ParseHistorial reads the latest Z18YYYY file.
func ParseHistorial(dataPath string) ([]HistorialDoc, string, error) {
	path, year := FindLatestZ18(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z18YYYY file found in %s", dataPath)
	}
	return ParseHistorialFile(path, year)
}

// ParseHistorialFile reads a specific Z18 file.
func ParseHistorialFile(path, year string) ([]HistorialDoc, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var docs []HistorialDoc
	for _, rec := range records {
		d := parseHistorialRecord(rec, extfh)
		if d.NombreOrigen == "" && d.NombreDestin == "" {
			continue
		}
		docs = append(docs, d)
	}

	return docs, year, nil
}

func parseHistorialRecord(rec []byte, extfh bool) HistorialDoc {
	if len(rec) < 100 {
		return HistorialDoc{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseHistorialEXTFH(rec, hash)
	}
	return parseHistorialHeuristic(rec, hash)
}

// parseHistorialEXTFH extracts Z18YYYY records using EXTFH offsets.
// Z18YYYY structure (524 bytes) verified via hex dump 2026-03-08:
//
//	[0:1]     tipo_registro (1=active, 2=inactive)
//	[1:3]     sub_tipo (SF=factura, PRE=presupuesto)
//	[3:6]     empresa (3 chars)
//	[6:53]    key data (zeros, sequence, flags)
//	[63:71]   fecha (YYYYMMDD)
//	[71:77]   hora (HHMMSS)
//	[77:117]  nombre_origen (40 chars, company name)
//	[137:150] nit area
//	[161:201] nombre_destino (40 chars)
func parseHistorialEXTFH(rec []byte, hash [32]byte) HistorialDoc {
	tipo := strings.TrimSpace(isam.ExtractField(rec, 0, 1))
	subTipo := strings.TrimSpace(isam.ExtractField(rec, 1, 2))
	empresa := strings.TrimSpace(isam.ExtractField(rec, 3, 3))

	// Date at offset 63
	fecha := ""
	if len(rec) >= 71 {
		f := isam.ExtractField(rec, 63, 8)
		if looksLikeDate(f) {
			fecha = f
		}
	}

	// Company names
	nombre1 := ""
	if len(rec) >= 117 {
		nombre1 = strings.TrimSpace(isam.ExtractField(rec, 77, 40))
	}

	nombre2 := ""
	if len(rec) >= 201 {
		nombre2 = strings.TrimSpace(isam.ExtractField(rec, 161, 40))
	}

	// NIT from area around offset 137
	nit := ""
	if len(rec) >= 150 {
		nitRaw := strings.TrimSpace(isam.ExtractField(rec, 137, 13))
		nit = strings.TrimLeft(nitRaw, "0")
		// Validate numeric
		clean := ""
		for _, c := range nit {
			if c >= '0' && c <= '9' {
				clean += string(c)
			}
		}
		nit = clean
	}

	if nombre1 == "" && nombre2 == "" {
		return HistorialDoc{}
	}

	return HistorialDoc{
		TipoRegistro: tipo,
		SubTipo:      subTipo,
		Empresa:      empresa,
		Fecha:        fecha,
		NombreOrigen: nombre1,
		NombreDestin: nombre2,
		NitOrigen:    nit,
		Hash:         fmt.Sprintf("%x", hash[:8]),
	}
}

func parseHistorialHeuristic(rec []byte, hash [32]byte) HistorialDoc {
	nombre := findDescripcion(rec, 40)
	if nombre == "" {
		return HistorialDoc{}
	}
	return HistorialDoc{
		NombreOrigen: nombre,
		Hash:         fmt.Sprintf("%x", hash[:8]),
	}
}
