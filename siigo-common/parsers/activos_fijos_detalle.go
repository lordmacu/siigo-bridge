package parsers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"
)

// ActivoFijoDetalle represents a fixed asset detail record from Siigo Z27YYYY.
// Z27YYYY files are yearly snapshots of fixed assets with purchase values.
// Records are 2048 bytes each.
type ActivoFijoDetalle struct {
	Grupo          string  `json:"grupo"`           // asset group (5 chars)
	Secuencia      string  `json:"secuencia"`       // sequence within group (5 chars)
	Nombre         string  `json:"nombre"`          // asset name (~49 chars)
	NitResponsable string  `json:"nit_responsable"` // responsible NIT (13 chars, zero-padded)
	Codigo         string  `json:"codigo"`          // additional code (10 chars)
	Fecha          string  `json:"fecha"`           // date YYYYMMDD (8 chars)
	ValorCompra    float64 `json:"valor_compra"`    // purchase value (BCD at ~@118, 7 bytes, 2 decimals)
	Hash           string  `json:"hash"`
}

// ParseActivosFijosDetalle reads the latest Z27YYYY file and returns fixed asset detail records.
// Uses FindLatestZ27 from activos_fijos.go.
func ParseActivosFijosDetalle(dataPath string) ([]ActivoFijoDetalle, string, error) {
	path, year := FindLatestZ27(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z27YYYY fixed assets file found in %s", dataPath)
	}
	return ParseActivosFijosDetalleFile(path, year)
}

// ParseActivosFijosDetalleFile reads a specific Z27 file and returns fixed asset detail records.
func ParseActivosFijosDetalleFile(path, year string) ([]ActivoFijoDetalle, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("fixed assets file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var items []ActivoFijoDetalle
	for _, rec := range records {
		item := parseActivoFijoDetalleRecord(rec, extfh)
		if item.Nombre == "" {
			continue // skip empty/corrupt records
		}
		items = append(items, item)
	}

	return items, year, nil
}

func parseActivoFijoDetalleRecord(rec []byte, extfh bool) ActivoFijoDetalle {
	if len(rec) < 60 {
		return ActivoFijoDetalle{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseActivoFijoDetalleEXTFH(rec, hash)
	}
	return parseActivoFijoDetalleBinary(rec, hash)
}

// parseActivoFijoDetalleEXTFH extracts Z27YYYY records using EXTFH offsets.
// Z27YYYY structure (2048 bytes) verified via hex dump:
//
//	[0:6]     grupo - 6-char group code ("000001", "000002")
//	[6:11]    secuencia - 5-char sequence within group ("00001", "00002")
//	[11:61]   nombre - asset name (50 chars, space-padded)
//	[61:74]   nit_responsable - NIT (13 chars, zero-padded)
//	[74:84]   codigo - additional code (10 chars, e.g. "0000001000")
//	[84:122]  padding + additional codes
//	[122:130] fecha - date YYYYMMDD
//	[130:137] BCD valor_compra (7 bytes packed decimal, 2 decimals)
func parseActivoFijoDetalleEXTFH(rec []byte, hash [32]byte) ActivoFijoDetalle {
	if len(rec) < 130 {
		return ActivoFijoDetalle{}
	}

	grupo := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 0, 6)), "0")
	secuencia := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 6, 5)), "0")
	nombre := strings.TrimSpace(isam.ExtractField(rec, 11, 50))
	nitResponsable := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 61, 13)), "0")
	codigo := strings.TrimSpace(isam.ExtractField(rec, 74, 10))
	fecha := strings.TrimSpace(isam.ExtractField(rec, 122, 8))

	if nombre == "" {
		return ActivoFijoDetalle{}
	}

	if fecha != "" && !looksLikeDate(fecha) {
		fecha = ""
	}

	// BCD valor_compra at @130 (7 bytes, 2 decimal places) — may be zero in demo data
	var valorCompra float64
	if len(rec) >= 137 {
		valorCompra = DecodePacked(rec[130:137], 2)
	}

	return ActivoFijoDetalle{
		Grupo:          grupo,
		Secuencia:      secuencia,
		Nombre:         nombre,
		NitResponsable: nitResponsable,
		Codigo:         codigo,
		Fecha:          fecha,
		ValorCompra:    valorCompra,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

// parseActivoFijoDetalleBinary extracts Z27YYYY records using binary fallback (+2 offset).
func parseActivoFijoDetalleBinary(rec []byte, hash [32]byte) ActivoFijoDetalle {
	off := 2

	if len(rec) < off+130 {
		return ActivoFijoDetalle{}
	}

	grupo := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, off+0, 6)), "0")
	secuencia := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, off+6, 5)), "0")
	nombre := strings.TrimSpace(isam.ExtractField(rec, off+11, 50))
	nitResponsable := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, off+61, 13)), "0")
	codigo := strings.TrimSpace(isam.ExtractField(rec, off+74, 10))
	fecha := strings.TrimSpace(isam.ExtractField(rec, off+122, 8))

	if nombre == "" {
		return ActivoFijoDetalle{}
	}

	if fecha != "" && !looksLikeDate(fecha) {
		fecha = ""
	}

	var valorCompra float64
	if len(rec) >= off+137 {
		valorCompra = DecodePacked(rec[off+130:off+137], 2)
	}

	return ActivoFijoDetalle{
		Grupo:          grupo,
		Secuencia:      secuencia,
		Nombre:         nombre,
		NitResponsable: nitResponsable,
		Codigo:         codigo,
		Fecha:          fecha,
		ValorCompra:    valorCompra,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}
