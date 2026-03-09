package parsers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"strings"
)

// MovimientoInventario represents an inventory movement from Z16YYYY.
// These are physical inventory transactions: entries, exits, transfers.
type MovimientoInventario struct {
	RecordKey       string `json:"record_key"`
	Empresa         string `json:"empresa"`
	Grupo           string `json:"grupo"`
	CodigoProducto  string `json:"codigo_producto"`
	TipoComprobante string `json:"tipo_comprobante"` // E=entrada, L=liquidacion, P=pedido
	CodigoComp      string `json:"codigo_comp"`
	Secuencia       string `json:"secuencia"`
	TipoDoc         string `json:"tipo_doc"`
	Fecha           string `json:"fecha"`
	Cantidad        string `json:"cantidad"`
	Valor           string `json:"valor"`
	TipoMov         string `json:"tipo_mov"` // D=debito, C=credito
	Hash            string `json:"hash"`
}

// FindLatestZ16 finds the Z16YYYY file with most data (largest file size).
func FindLatestZ16(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z16[0-9][0-9][0-9][0-9]")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	// Pick the largest file (most data), not just the latest year
	var bestPath string
	var bestYear string
	var bestSize int64
	for _, m := range matches {
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestPath = m
			bestYear = filepath.Base(m)[3:]
		}
	}
	return bestPath, bestYear
}

// ParseMovimientosInventario reads the latest Z16YYYY and returns records.
func ParseMovimientosInventario(dataPath string) ([]MovimientoInventario, string, error) {
	path, year := FindLatestZ16(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z16YYYY file found in %s", dataPath)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("Z16 file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var items []MovimientoInventario
	for _, rec := range records {
		item := parseMovInvRecord(rec, extfh)
		if item.CodigoProducto == "" {
			continue
		}
		items = append(items, item)
	}
	return items, year, nil
}

// Z16YYYY structure (320 bytes) — verified from hex dump with v2 reader:
//
//	[0:3]     empresa ("001")
//	[3:6]     grupo inventario ("000")
//	[6:7]     variante
//	[7:13]    codigo producto ("100000")
//	[13:14]   tipo comprobante (E=entrada, L=liquidacion, P=pedido)
//	[14:17]   codigo comprobante ("001", "999")
//	[17:18]   null separator
//	[23:26]   secuencia de linea
//	[26:28]   tipo documento ("06"=factura, "07"=nota)
//	[28:31]   empresa ref
//	[31:37]   producto ref
//	[37:44]   padding/flags
//	[44:52]   fecha YYYYMMDD
//	[52:68]   cantidad (ASCII numerico, 16 chars)
//	[68:84]   cantidad2 (auxiliar)
//	[78:94]   cantidad3
//	[94:95]   separador
//	[95:96]   espacio
//	[96:103]  padding
//	[103:104] D/C (D=debito/salida, C=credito/entrada)
//	[104:119] valor (ASCII numerico, 15 chars)
func parseMovInvRecord(rec []byte, extfh bool) MovimientoInventario {
	if len(rec) < 120 {
		return MovimientoInventario{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseMovInvEXTFH(rec, hash)
	}
	return parseMovInvEXTFH(rec, hash) // same offsets for v2 reader
}

func parseMovInvEXTFH(rec []byte, hash [32]byte) MovimientoInventario {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 3))
	grupo := strings.TrimSpace(isam.ExtractField(rec, 3, 3))
	codigo := strings.TrimSpace(isam.ExtractField(rec, 7, 6))

	if codigo == "" || codigo == "000000" {
		return MovimientoInventario{}
	}

	tipoComp := strings.TrimSpace(isam.ExtractField(rec, 13, 1))
	codigoComp := strings.TrimSpace(isam.ExtractField(rec, 14, 3))
	secuencia := strings.TrimSpace(isam.ExtractField(rec, 23, 3))
	tipoDoc := strings.TrimSpace(isam.ExtractField(rec, 26, 2))
	fecha := strings.TrimSpace(isam.ExtractField(rec, 44, 8))
	cantidad := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 52, 16)), "0")
	tipoMov := strings.TrimSpace(isam.ExtractField(rec, 103, 1))
	valor := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 104, 15)), "0")

	if cantidad == "" {
		cantidad = "0"
	}
	if valor == "" {
		valor = "0"
	}

	// Build unique key (hash suffix for records with same logical key)
	hashStr := fmt.Sprintf("%x", hash[:8])
	key := fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s", empresa, grupo, codigo, tipoComp+codigoComp, secuencia, fecha, hashStr[:6])

	return MovimientoInventario{
		RecordKey:       key,
		Empresa:         empresa,
		Grupo:           grupo,
		CodigoProducto:  codigo,
		TipoComprobante: tipoComp,
		CodigoComp:      codigoComp,
		Secuencia:       secuencia,
		TipoDoc:         tipoDoc,
		Fecha:           fecha,
		Cantidad:        cantidad,
		Valor:           valor,
		TipoMov:         tipoMov,
		Hash:            fmt.Sprintf("%x", hash[:8]),
	}
}
