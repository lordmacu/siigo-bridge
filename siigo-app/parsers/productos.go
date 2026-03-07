package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-app/isam"
	"strings"
)

// Producto represents a product from Siigo Z06 file
type Producto struct {
	Codigo      string `json:"codigo"`
	Nombre      string `json:"nombre"`
	Grupo       string `json:"grupo"`
	UnidadMed   string `json:"unidad_medida"`
	Estado      string `json:"estado"`
	RawPreview  string `json:"raw_preview"` // first 120 chars for debugging
	Hash        string `json:"hash"`
}

// ParseProductos reads the Z06 file and returns products
// Z06 has a special format (recSize=4096 matches page size)
// We use the backup file Z0620171114 if available (recSize=2286)
// or fall back to scanning for product-like records
func ParseProductos(dataPath string) ([]Producto, error) {
	// Try backup first (cleaner format)
	path := dataPath + "Z0620171114"
	records, recSize, err := isam.ReadIsamFile(path)
	if err != nil || len(records) == 0 {
		// Fall back to main Z06
		path = dataPath + "Z06"
		records, recSize, err = isam.ReadIsamFile(path)
		if err != nil {
			return nil, err
		}
	}

	var productos []Producto
	for _, rec := range records {
		p := parseProductoRecord(rec, recSize)
		if p.Nombre == "" {
			continue
		}
		productos = append(productos, p)
	}

	return productos, nil
}

func parseProductoRecord(rec []byte, recSize int) Producto {
	if len(rec) < 50 {
		return Producto{}
	}

	hash := sha256.Sum256(rec)

	// Z06 record structure varies by version. Common patterns:
	// The first ~20 bytes contain codes/flags
	// Product name is typically a 40-50 char field early in the record
	// We search for the first block of uppercase readable text

	rawPreview := isam.ExtractField(rec, 0, 120)

	// Try to find product code and name by scanning
	codigo := ""
	nombre := ""

	// Strategy: Look for the first significant text block
	// Product records typically start with: code(20) + name(40-60) + ...
	// But the exact layout depends on the record type within Z06

	// Extract what looks like a code (first alphanumeric block)
	for i := 0; i < len(rec) && i < 50; i++ {
		if rec[i] >= '0' && rec[i] <= 'Z' && rec[i] != ' ' {
			end := i
			for end < len(rec) && end < i+20 && rec[end] != ' ' && rec[end] >= 0x20 {
				end++
			}
			if end-i >= 2 {
				codigo = isam.ExtractField(rec, i, end-i)
				break
			}
		}
	}

	// Find name (longest uppercase text block after code)
	bestStart := -1
	bestLen := 0
	inText := false
	textStart := 0

	for i := len(codigo); i < len(rec) && i < 200; i++ {
		isUpper := (rec[i] >= 'A' && rec[i] <= 'Z') || rec[i] == ' ' || rec[i] == '.' ||
			(rec[i] >= 0xC0 && rec[i] <= 0xFF) // accented chars
		if !inText && rec[i] >= 'A' && rec[i] <= 'Z' {
			inText = true
			textStart = i
		} else if inText && !isUpper && rec[i] != ' ' {
			textLen := i - textStart
			if textLen > bestLen && textLen > 3 {
				bestStart = textStart
				bestLen = textLen
			}
			inText = false
		}
	}
	if inText {
		textLen := min(200, len(rec)) - textStart
		if textLen > bestLen && textLen > 3 {
			bestStart = textStart
			bestLen = textLen
		}
	}

	if bestStart >= 0 {
		nombre = strings.TrimSpace(isam.ExtractField(rec, bestStart, bestLen))
	}

	if nombre == "" {
		return Producto{}
	}

	return Producto{
		Codigo:     codigo,
		Nombre:     nombre,
		RawPreview: rawPreview,
		Hash:       fmt.Sprintf("%x", hash[:8]),
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ToFinearomProduct converts a Producto to a map for the Finearom API
func (p *Producto) ToFinearomProduct() map[string]interface{} {
	return map[string]interface{}{
		"code":            p.Codigo,
		"product_name":    p.Nombre,
		"siigo_sync_hash": p.Hash,
	}
}
