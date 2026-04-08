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
	Company   string `json:"empresa"`       // company code (5 chars)
	Group     string `json:"grupo"`         // inventory group (3 chars)
	Code      string `json:"codigo"`        // product code (6 chars)
	Name      string `json:"nombre"`        // product name (50 chars)
	ShortName string `json:"nombre_corto"`  // short name (40 chars)
	Reference string `json:"referencia"`    // reference (30 chars)
	Hash      string `json:"hash"`
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
		if item.Code == "" || item.Name == "" {
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

	company := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
	group := strings.TrimSpace(isam.ExtractField(rec, 5, 3))
	code := strings.TrimSpace(isam.ExtractField(rec, 8, 6))
	nameRaw := strings.TrimSpace(isam.ExtractField(rec, 14, 50))

	// First char of name may be a sub-index digit (0,1,2,3...)
	// for products with the same code but different variants
	name := nameRaw
	subIndex := ""
	if len(nameRaw) > 1 && nameRaw[0] >= '0' && nameRaw[0] <= '9' {
		subIndex = string(nameRaw[0])
		name = strings.TrimSpace(nameRaw[1:])
	}

	// If still empty after trimming sub-index, skip
	if name == "" {
		return Inventario{}
	}

	// Use code + subIndex as unique key to handle variants
	fullCode := code
	if subIndex != "" {
		fullCode = code + "-" + subIndex
	}

	shortName := ""
	if len(rec) >= 104 {
		shortName = strings.TrimSpace(isam.ExtractField(rec, 64, 40))
	}

	reference := ""
	if len(rec) >= 134 {
		reference = strings.TrimSpace(isam.ExtractField(rec, 104, 30))
	}

	return Inventario{
		Company:   company,
		Group:     group,
		Code:      fullCode,
		Name:      name,
		ShortName: shortName,
		Reference: reference,
		Hash:      fmt.Sprintf("%x", hash[:8]),
	}
}

func parseInventarioHeuristic(rec []byte, hash [32]byte) Inventario {
	// Binary fallback: try to find readable product name
	name := findDescripcion(rec, 14)
	if name == "" {
		return Inventario{}
	}

	return Inventario{
		Name: name,
		Hash: fmt.Sprintf("%x", hash[:8]),
	}
}

// ToFinearomProduct converts an Inventario to a map for the Finearom API.
func (inv *Inventario) ToFinearomProduct() map[string]interface{} {
	return map[string]interface{}{
		"code":            inv.Code,
		"product_name":    inv.Name,
		"grupo":           inv.Group,
		"referencia":      inv.Reference,
		"siigo_sync_hash": inv.Hash,
	}
}
