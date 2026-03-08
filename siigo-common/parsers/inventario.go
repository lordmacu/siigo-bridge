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

// Inventario represents a real product/item from Siigo Z04YYYY (inventory master).
// Z04YYYY files are yearly snapshots of the product catalog.
// The latest year file is a superset of all previous years.
type Inventario struct {
	Empresa      string `json:"empresa"`       // company code (5 chars)
	Grupo        string `json:"grupo"`         // inventory group (3 chars)
	Codigo       string `json:"codigo"`        // product code (6 chars)
	Nombre       string `json:"nombre"`        // product name (50 chars)
	NombreCorto  string `json:"nombre_corto"`  // short name (40 chars)
	Referencia   string `json:"referencia"`    // reference (30 chars)
	Hash         string `json:"hash"`
}

// FindLatestZ04 finds the most recent Z04YYYY file in the data directory.
// Returns the full path and the year string, or empty if none found.
func FindLatestZ04(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z04[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}

	// Sort descending to get latest year first
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))

	for _, m := range matches {
		// Skip .idx files
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		base := filepath.Base(m)
		year := base[3:] // "Z042016" -> "2016"
		return m, year
	}
	return "", ""
}

// ParseInventario reads the latest Z04YYYY file and returns inventory/product records.
func ParseInventario(dataPath string) ([]Inventario, string, error) {
	path, year := FindLatestZ04(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z04YYYY inventory file found in %s", dataPath)
	}

	return ParseInventarioFile(path, year)
}

// ParseInventarioFile reads a specific Z04 file and returns inventory records.
func ParseInventarioFile(path, year string) ([]Inventario, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("inventory file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var items []Inventario
	for _, rec := range records {
		item := parseInventarioRecord(rec, extfh)
		if item.Codigo == "" || item.Nombre == "" {
			continue // skip corrupt/empty records
		}
		items = append(items, item)
	}

	return items, year, nil
}

func parseInventarioRecord(rec []byte, extfh bool) Inventario {
	if len(rec) < 64 {
		return Inventario{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseInventarioEXTFH(rec, hash)
	}
	return parseInventarioHeuristic(rec, hash)
}

// parseInventarioEXTFH extracts Z04YYYY records using EXTFH offsets.
// Z04YYYY structure (3520 bytes) verified via peek_files diagnostic:
//
//	[0:5]     empresa - company code ("00001")
//	[5:8]     grupo - inventory group ("000")
//	[8:14]    codigo - product code ("100000")
//	[14:64]   nombre - product name (50 chars, first char may be sub-index)
//	[64:104]  nombre_corto - short name (40 chars)
//	[104:134] referencia - reference code (30 chars)
func parseInventarioEXTFH(rec []byte, hash [32]byte) Inventario {
	if len(rec) < 64 {
		return Inventario{}
	}

	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
	grupo := strings.TrimSpace(isam.ExtractField(rec, 5, 3))
	codigo := strings.TrimSpace(isam.ExtractField(rec, 8, 6))
	nombreRaw := strings.TrimSpace(isam.ExtractField(rec, 14, 50))

	// First char of nombre may be a sub-index digit (0,1,2,3...)
	// for products with the same code but different variants
	nombre := nombreRaw
	subIndex := ""
	if len(nombreRaw) > 1 && nombreRaw[0] >= '0' && nombreRaw[0] <= '9' {
		subIndex = string(nombreRaw[0])
		nombre = strings.TrimSpace(nombreRaw[1:])
	}

	// If still empty after trimming sub-index, skip
	if nombre == "" {
		return Inventario{}
	}

	// Use codigo + subIndex as unique key to handle variants
	fullCodigo := codigo
	if subIndex != "" {
		fullCodigo = codigo + "-" + subIndex
	}

	nombreCorto := ""
	if len(rec) >= 104 {
		nombreCorto = strings.TrimSpace(isam.ExtractField(rec, 64, 40))
	}

	referencia := ""
	if len(rec) >= 134 {
		referencia = strings.TrimSpace(isam.ExtractField(rec, 104, 30))
	}

	return Inventario{
		Empresa:     empresa,
		Grupo:       grupo,
		Codigo:      fullCodigo,
		Nombre:      nombre,
		NombreCorto: nombreCorto,
		Referencia:  referencia,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}

func parseInventarioHeuristic(rec []byte, hash [32]byte) Inventario {
	// Binary fallback: try to find readable product name
	nombre := findDescripcion(rec, 14)
	if nombre == "" {
		return Inventario{}
	}

	return Inventario{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}

// ToFinearomProduct converts an Inventario to a map for the Finearom API.
func (inv *Inventario) ToFinearomProduct() map[string]interface{} {
	return map[string]interface{}{
		"code":            inv.Codigo,
		"product_name":    inv.Nombre,
		"grupo":           inv.Grupo,
		"referencia":      inv.Referencia,
		"siigo_sync_hash": inv.Hash,
	}
}
