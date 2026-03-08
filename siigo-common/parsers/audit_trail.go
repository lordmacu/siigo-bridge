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

// AuditTrailTercero represents an audit trail record for third parties from Z11NYYYY.
// Tracks changes made to tercero records: who changed what and when.
type AuditTrailTercero struct {
	FechaCambio         string `json:"fecha_cambio"`          // YYYYMMDD when change was made
	NitTercero          string `json:"nit_tercero"`           // 13-digit NIT of the tercero
	Timestamp           string `json:"timestamp"`             // YYYYMMDDHHmmssCC full timestamp
	Usuario             string `json:"usuario"`               // 8-char user who made the change
	FechaPeriodo        string `json:"fecha_periodo"`         // YYYYMMDD period end date
	TipoDoc             string `json:"tipo_doc"`              // NO=NIT, NP=NIT persona, NC=NIT compañía
	Nombre              string `json:"nombre"`                // 48-char name of tercero
	NitRepresentante    string `json:"nit_representante"`     // 13-digit NIT of legal representative
	NombreRepresentante string `json:"nombre_representante"`  // 40-char name of representative
	Hash                string `json:"hash"`
}

// findLatestZ11N finds the most recent Z11NYYYY file with actual data.
// Skips files that are just empty ISAM headers (<=4096 bytes).
func findLatestZ11N(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z11N[0-9][0-9][0-9][0-9]")
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
		if len(base) < 8 {
			continue
		}
		year := base[4:8]
		if year < "1990" || year > "2099" {
			continue
		}
		// Skip empty files (ISAM header only = ~4096 bytes)
		info, err := os.Stat(m)
		if err != nil || info.Size() <= 4096 {
			continue
		}
		return m, year
	}
	return "", ""
}

// ParseAuditTrailTerceros reads the latest Z11NYYYY file and returns audit trail records.
func ParseAuditTrailTerceros(dataPath string) ([]AuditTrailTercero, string, error) {
	path, year := findLatestZ11N(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z11NYYYY file found in %s", dataPath)
	}
	return ParseAuditTrailTercerosFile(path, year)
}

// ParseAuditTrailTercerosFile reads a specific Z11N file.
func ParseAuditTrailTercerosFile(path, year string) ([]AuditTrailTercero, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var entries []AuditTrailTercero
	for _, rec := range records {
		t := parseAuditTrailRecord(rec, extfh)
		if t.NitTercero == "" || t.Nombre == "" {
			continue
		}
		entries = append(entries, t)
	}

	return entries, year, nil
}

func parseAuditTrailRecord(rec []byte, extfh bool) AuditTrailTercero {
	if len(rec) < 100 {
		return AuditTrailTercero{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseAuditTrailEXTFH(rec, hash)
	}
	return parseAuditTrailBinary(rec, hash)
}

// parseAuditTrailEXTFH extracts Z11NYYYY records using EXTFH offsets.
// Z11NYYYY structure (846 bytes) — audit trail for terceros:
//
//	[0:8]     fecha_cambio (YYYYMMDD)
//	[8:16]    padding/spaces
//	[16:29]   nit_tercero (13 digits, zero-padded)
//	[29:32]   unknown ("000")
//	[32:48]   timestamp (YYYYMMDDHHmmssCC)
//	[48:56]   usuario (8 chars)
//	[56:64]   fecha_periodo (YYYYMMDD)
//	[64:77]   nit_tercero2 (13 digits, duplicate)
//	[77:80]   unknown ("000")
//	[80:82]   tipo_doc (NO/NP/NC)
//	[82:130]  nombre (48 chars)
//	[130:143] padding
//	[143:156] nit_representante (13 digits)
//	[156:196] nombre_representante (40 chars)
func parseAuditTrailEXTFH(rec []byte, hash [32]byte) AuditTrailTercero {
	fechaCambio := strings.TrimSpace(isam.ExtractField(rec, 0, 8))
	if fechaCambio == "" || len(fechaCambio) < 8 {
		return AuditTrailTercero{}
	}

	nitRaw := strings.TrimSpace(isam.ExtractField(rec, 16, 13))
	nit := strings.TrimLeft(nitRaw, "0")
	if nit == "" {
		return AuditTrailTercero{}
	}

	timestamp := strings.TrimSpace(isam.ExtractField(rec, 32, 16))
	usuario := strings.TrimSpace(isam.ExtractField(rec, 48, 8))
	fechaPeriodo := strings.TrimSpace(isam.ExtractField(rec, 56, 8))

	tipoDoc := ""
	if len(rec) >= 82 {
		tipoDoc = strings.TrimSpace(isam.ExtractField(rec, 80, 2))
	}

	nombre := ""
	if len(rec) >= 130 {
		nombre = strings.TrimSpace(isam.ExtractField(rec, 82, 48))
	}
	if nombre == "" {
		return AuditTrailTercero{}
	}

	nitRepresentante := ""
	if len(rec) >= 156 {
		repRaw := strings.TrimSpace(isam.ExtractField(rec, 143, 13))
		nitRepresentante = strings.TrimLeft(repRaw, "0")
	}

	nombreRepresentante := ""
	if len(rec) >= 196 {
		nombreRepresentante = strings.TrimSpace(isam.ExtractField(rec, 156, 40))
	}

	return AuditTrailTercero{
		FechaCambio:         fechaCambio,
		NitTercero:          nit,
		Timestamp:           timestamp,
		Usuario:             usuario,
		FechaPeriodo:        fechaPeriodo,
		TipoDoc:             tipoDoc,
		Nombre:              nombre,
		NitRepresentante:    nitRepresentante,
		NombreRepresentante: nombreRepresentante,
		Hash:                fmt.Sprintf("%x", hash[:8]),
	}
}

// parseAuditTrailBinary extracts Z11NYYYY records in binary mode (+2 byte offset for record markers).
func parseAuditTrailBinary(rec []byte, hash [32]byte) AuditTrailTercero {
	const off = 2 // binary mode offset for record markers

	if len(rec) < 100+off {
		return AuditTrailTercero{}
	}

	fechaCambio := strings.TrimSpace(isam.ExtractField(rec, 0+off, 8))
	if fechaCambio == "" || len(fechaCambio) < 8 {
		return AuditTrailTercero{}
	}

	nitRaw := strings.TrimSpace(isam.ExtractField(rec, 16+off, 13))
	nit := strings.TrimLeft(nitRaw, "0")
	if nit == "" {
		return AuditTrailTercero{}
	}

	timestamp := strings.TrimSpace(isam.ExtractField(rec, 32+off, 16))
	usuario := strings.TrimSpace(isam.ExtractField(rec, 48+off, 8))
	fechaPeriodo := strings.TrimSpace(isam.ExtractField(rec, 56+off, 8))

	tipoDoc := ""
	if len(rec) >= 82+off {
		tipoDoc = strings.TrimSpace(isam.ExtractField(rec, 80+off, 2))
	}

	nombre := ""
	if len(rec) >= 130+off {
		nombre = strings.TrimSpace(isam.ExtractField(rec, 82+off, 48))
	}
	if nombre == "" {
		return AuditTrailTercero{}
	}

	nitRepresentante := ""
	if len(rec) >= 156+off {
		repRaw := strings.TrimSpace(isam.ExtractField(rec, 143+off, 13))
		nitRepresentante = strings.TrimLeft(repRaw, "0")
	}

	nombreRepresentante := ""
	if len(rec) >= 196+off {
		nombreRepresentante = strings.TrimSpace(isam.ExtractField(rec, 156+off, 40))
	}

	return AuditTrailTercero{
		FechaCambio:         fechaCambio,
		NitTercero:          nit,
		Timestamp:           timestamp,
		Usuario:             usuario,
		FechaPeriodo:        fechaPeriodo,
		TipoDoc:             tipoDoc,
		Nombre:              nombre,
		NitRepresentante:    nitRepresentante,
		NombreRepresentante: nombreRepresentante,
		Hash:                fmt.Sprintf("%x", hash[:8]),
	}
}
