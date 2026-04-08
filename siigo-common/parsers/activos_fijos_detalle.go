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

// ActivoFijoDetalle represents a detailed fixed asset record from Z27YYYYA.
// Complements Z27 with financial data: purchase value (BCD), location, reference.
type ActivoFijoDetalle struct {
	Group          string  `json:"grupo"`
	Sequence       string  `json:"secuencia"`
	Name           string  `json:"nombre"`
	ResponsibleNit string  `json:"nit_responsable"`
	Code           string  `json:"codigo"`
	Date           string  `json:"fecha"`
	PurchaseValue  float64 `json:"valor_compra"`
	Location       string  `json:"ubicacion"`
	Reference      string  `json:"referencia"`
	Hash           string  `json:"hash"`
}

// FindLatestZ27A finds the most recent Z27YYYYA file (by year descending).
func FindLatestZ27A(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z27[0-9][0-9][0-9][0-9]A")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, m := range matches {
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		info, err := os.Stat(m)
		if err != nil || info.Size() <= 4096 {
			continue
		}
		base := filepath.Base(m)
		year := base[3:7]
		return m, year
	}
	return "", ""
}

// ParseActivosFijosDetalle reads the latest Z27YYYYA file.
func ParseActivosFijosDetalle(dataPath string) ([]ActivoFijoDetalle, string, error) {
	path, year := FindLatestZ27A(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z27YYYYA file found in %s", dataPath)
	}
	return ParseActivosFijosDetalleFile(path, year)
}

func ParseActivosFijosDetalleFile(path, year string) ([]ActivoFijoDetalle, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}
	extfh := isam.ExtfhAvailable()
	var items []ActivoFijoDetalle
	for _, rec := range records {
		a := parseActivoDetalleRecord(rec, extfh)
		if a.Name == "" {
			continue
		}
		items = append(items, a)
	}
	return items, year, nil
}

func parseActivoDetalleRecord(rec []byte, extfh bool) ActivoFijoDetalle {
	if len(rec) < 130 {
		return ActivoFijoDetalle{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseActivoDetalleEXTFH(rec, hash)
	}
	return parseActivoDetalleHeuristic(rec, hash)
}

// parseActivoDetalleEXTFH extracts Z27YYYYA records.
// Z27YYYYA structure (1536 bytes) verified via hex dump:
//   [0:5]     company (5 chars)
//   [5:11]    code (6 chars)
//   [11:61]   name (50 chars)
//   [61:74]   nit_responsable (13 chars)
//   [122:130] acquisition date (YYYYMMDD)
//   [130:138] BCD purchase value (8 bytes, sign in last nibble: C=pos, D=neg)
//   [586:632] location (46 chars, e.g. "PRODUCCION")
//   [736:744] reference (8 chars, e.g. "SIN14545")
func parseActivoDetalleEXTFH(rec []byte, hash [32]byte) ActivoFijoDetalle {
	company := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
	code := strings.TrimSpace(isam.ExtractField(rec, 5, 6))
	name := strings.TrimSpace(isam.ExtractField(rec, 11, 50))
	if name == "" {
		return ActivoFijoDetalle{}
	}

	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 61, 13)), "0")

	date := ""
	if len(rec) >= 130 {
		dateRaw := isam.ExtractField(rec, 122, 8)
		if looksLikeDate(dateRaw) {
			date = dateRaw
		}
	}

	var purchaseValue float64
	if len(rec) >= 138 {
		purchaseValue = DecodePacked(rec[130:138], 2)
	}

	location := ""
	if len(rec) >= 632 {
		location = strings.TrimSpace(isam.ExtractField(rec, 586, 46))
	}
	reference := ""
	if len(rec) >= 744 {
		reference = strings.TrimSpace(isam.ExtractField(rec, 736, 8))
	}

	return ActivoFijoDetalle{
		Group:          company,
		Sequence:       code,
		Name:           name,
		ResponsibleNit: nit,
		Code:           code,
		Date:           date,
		PurchaseValue:  purchaseValue,
		Location:       location,
		Reference:      reference,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseActivoDetalleHeuristic(rec []byte, hash [32]byte) ActivoFijoDetalle {
	name := findDescripcion(rec, 11)
	if name == "" {
		return ActivoFijoDetalle{}
	}
	return ActivoFijoDetalle{
		Name: name,
		Hash: fmt.Sprintf("%x", hash[:8]),
	}
}
