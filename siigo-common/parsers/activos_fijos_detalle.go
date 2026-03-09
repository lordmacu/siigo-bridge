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
// Complements Z27 with financial data: valor compra (BCD), ubicación, referencia.
type ActivoFijoDetalle struct {
	Grupo          string  `json:"grupo"`
	Secuencia      string  `json:"secuencia"`
	Nombre         string  `json:"nombre"`
	NitResponsable string  `json:"nit_responsable"`
	Codigo         string  `json:"codigo"`
	Fecha          string  `json:"fecha"`
	ValorCompra    float64 `json:"valor_compra"`
	Ubicacion      string  `json:"ubicacion"`
	Referencia     string  `json:"referencia"`
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
		if a.Nombre == "" {
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
//   [0:5]     empresa (5 chars)
//   [5:11]    codigo (6 chars)
//   [11:61]   nombre (50 chars)
//   [61:74]   nit_responsable (13 chars)
//   [122:130] fecha_adquisicion (YYYYMMDD)
//   [130:138] BCD valor_compra (8 bytes, sign in last nibble: C=pos, D=neg)
//   [586:632] ubicacion (46 chars, e.g. "PRODUCCION")
//   [736:744] referencia (8 chars, e.g. "SIN14545")
func parseActivoDetalleEXTFH(rec []byte, hash [32]byte) ActivoFijoDetalle {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
	codigo := strings.TrimSpace(isam.ExtractField(rec, 5, 6))
	nombre := strings.TrimSpace(isam.ExtractField(rec, 11, 50))
	if nombre == "" {
		return ActivoFijoDetalle{}
	}

	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 61, 13)), "0")

	fecha := ""
	if len(rec) >= 130 {
		fechaRaw := isam.ExtractField(rec, 122, 8)
		if looksLikeDate(fechaRaw) {
			fecha = fechaRaw
		}
	}

	var valorCompra float64
	if len(rec) >= 138 {
		valorCompra = DecodePacked(rec[130:138], 2)
	}

	ubicacion := ""
	if len(rec) >= 632 {
		ubicacion = strings.TrimSpace(isam.ExtractField(rec, 586, 46))
	}
	referencia := ""
	if len(rec) >= 744 {
		referencia = strings.TrimSpace(isam.ExtractField(rec, 736, 8))
	}

	return ActivoFijoDetalle{
		Grupo:          empresa,
		Secuencia:      codigo,
		Nombre:         nombre,
		NitResponsable: nit,
		Codigo:         codigo,
		Fecha:          fecha,
		ValorCompra:    valorCompra,
		Ubicacion:      ubicacion,
		Referencia:     referencia,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseActivoDetalleHeuristic(rec []byte, hash [32]byte) ActivoFijoDetalle {
	nombre := findDescripcion(rec, 11)
	if nombre == "" {
		return ActivoFijoDetalle{}
	}
	return ActivoFijoDetalle{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}
