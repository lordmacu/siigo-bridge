package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// CondicionPago represents a payment terms / credit conditions entry from Siigo Z05YYYY files.
// Z05 files contain credit conditions with mixed ASCII and BCD data (1023 bytes per record).
//
// Known EXTFH offsets (simplified — many binary/BCD fields remain undecoded):
//
//	[0:1]     tipo - 'C' (credit condition)
//	[1:4]     empresa - "001"
//	[4:9]     binary header (nulls + control bytes)
//	[9:10]    separator/flag byte (0x1F=header, 0x2F=detail, etc.)
//	[10:13]   secuencia - "000", "001", etc.
//	[13:14]   tipo_doc - 'N' (NIT), etc.
//	[14:22]   fecha - "YYYYMMDD"
//	[22:25]   binary control bytes
//	[25:38]   nit - 13-char zero-padded NIT
//	[131:135] secondary type+code (e.g. "E001")
//	[211:218] BCD monetary value (7 bytes, 2 decimals)
//	[224:232] fecha_registro or zeros - "YYYYMMDD" or "00000000"
type CondicionPago struct {
	Tipo          string  `json:"tipo"`           // 'C' = credit condition
	Empresa       string  `json:"empresa"`        // 001
	FlagByte      string  `json:"flag_byte"`      // hex of byte @9 (0x1F, 0x2F, etc.)
	Secuencia     string  `json:"secuencia"`      // sequential number
	TipoDoc       string  `json:"tipo_doc"`       // N=NIT, etc.
	Fecha         string  `json:"fecha"`          // YYYYMMDD
	NIT           string  `json:"nit"`            // NIT tercero
	TipoSecundario string `json:"tipo_secundario"` // secondary type+code at @131
	Valor         float64 `json:"valor"`          // BCD value at @211 (7 bytes)
	FechaRegistro string  `json:"fecha_registro"` // date at @224 or empty
	Hash          string  `json:"hash"`
}

// ParseCondicionesPago reads the latest Z05YYYY file and returns payment condition entries.
func ParseCondicionesPago(dataPath string) ([]CondicionPago, string, error) {
	path, year := findLatestZ05(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z05YYYY file found in %s", dataPath)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var result []CondicionPago
	for _, rec := range records {
		r := parseCondicionPago(rec, extfh)
		if r.Secuencia == "" && r.NIT == "" {
			continue
		}
		result = append(result, r)
	}

	return result, year, nil
}

func parseCondicionPago(rec []byte, extfh bool) CondicionPago {
	if len(rec) < 50 {
		return CondicionPago{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseCondicionPagoEXTFH(rec, hash)
	}
	return parseCondicionPagoBinary(rec, hash)
}

func parseCondicionPagoEXTFH(rec []byte, hash [32]byte) CondicionPago {
	tipo := string(rec[0])
	empresa := strings.TrimSpace(isam.ExtractField(rec, 1, 3))

	// Flag byte at @9
	flagByte := ""
	if len(rec) > 9 {
		flagByte = fmt.Sprintf("0x%02X", rec[9])
	}

	secuencia := strings.TrimSpace(isam.ExtractField(rec, 10, 3))
	tipoDoc := strings.TrimSpace(isam.ExtractField(rec, 13, 1))
	fecha := strings.TrimSpace(isam.ExtractField(rec, 14, 8))

	// NIT at @27, 13 chars zero-padded (bytes @22-26 are binary control, not NIT)
	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 27, 13)), "0")
	// Filter non-printable chars from NIT (binary control bytes may leak)
	cleanNit := ""
	for _, c := range nit {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			cleanNit += string(c)
		}
	}
	nit = cleanNit

	// Secondary type+code at @131 (4 chars, e.g. "E001")
	tipoSecundario := ""
	if len(rec) > 135 {
		tipoSecundario = strings.TrimSpace(isam.ExtractField(rec, 131, 4))
	}

	// BCD monetary value at @208 (7 bytes, 2 decimals).
	// Previously used @211 which always returned 0. Hex dump confirmed
	// valid BCD data starts at byte 208.
	var valor float64
	if len(rec) >= 215 {
		valor = DecodePacked(rec[208:215], 2)
	}

	// Date at @224 (8 chars)
	fechaRegistro := ""
	if len(rec) >= 232 {
		fechaRegistro = strings.TrimSpace(isam.ExtractField(rec, 224, 8))
		if !looksLikeDate(fechaRegistro) {
			fechaRegistro = ""
		}
	}

	// Validate main fecha
	if !looksLikeDate(fecha) {
		fecha = ""
	}

	// Skip records with no meaningful data
	if secuencia == "" && nit == "" && fecha == "" {
		return CondicionPago{}
	}

	return CondicionPago{
		Tipo:           tipo,
		Empresa:        empresa,
		FlagByte:       flagByte,
		Secuencia:      secuencia,
		TipoDoc:        tipoDoc,
		Fecha:          fecha,
		NIT:            nit,
		TipoSecundario: tipoSecundario,
		Valor:          valor,
		FechaRegistro:  fechaRegistro,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func parseCondicionPagoBinary(rec []byte, hash [32]byte) CondicionPago {
	// Binary fallback: offsets +2 for record markers
	if len(rec) < 52 {
		return CondicionPago{}
	}

	tipo := string(rec[2])
	empresa := strings.TrimSpace(isam.ExtractField(rec, 3, 3))

	flagByte := ""
	if len(rec) > 11 {
		flagByte = fmt.Sprintf("0x%02X", rec[11])
	}

	secuencia := strings.TrimSpace(isam.ExtractField(rec, 12, 3))
	tipoDoc := strings.TrimSpace(isam.ExtractField(rec, 15, 1))
	fecha := strings.TrimSpace(isam.ExtractField(rec, 16, 8))

	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 29, 13)), "0")
	cleanNit2 := ""
	for _, c := range nit {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			cleanNit2 += string(c)
		}
	}
	nit = cleanNit2

	tipoSecundario := ""
	if len(rec) > 137 {
		tipoSecundario = strings.TrimSpace(isam.ExtractField(rec, 133, 4))
	}

	var valor float64
	if len(rec) >= 220 {
		valor = DecodePacked(rec[213:220], 2)
	}

	fechaRegistro := ""
	if len(rec) >= 234 {
		fechaRegistro = strings.TrimSpace(isam.ExtractField(rec, 226, 8))
		if !looksLikeDate(fechaRegistro) {
			fechaRegistro = ""
		}
	}

	if !looksLikeDate(fecha) {
		fecha = ""
	}

	if secuencia == "" && nit == "" && fecha == "" {
		return CondicionPago{}
	}

	return CondicionPago{
		Tipo:           tipo,
		Empresa:        empresa,
		FlagByte:       flagByte,
		Secuencia:      secuencia,
		TipoDoc:        tipoDoc,
		Fecha:          fecha,
		NIT:            nit,
		TipoSecundario: tipoSecundario,
		Valor:          valor,
		FechaRegistro:  fechaRegistro,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}

func findLatestZ05(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z05[0-9][0-9][0-9][0-9]")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", ""
	}
	// Filter out .idx files and special codes
	var valid []string
	for _, m := range matches {
		if strings.HasSuffix(m, ".idx") {
			continue
		}
		year := m[len(m)-4:]
		if year >= "1990" && year <= "2099" {
			valid = append(valid, m)
		}
	}
	if len(valid) == 0 {
		return "", ""
	}
	sort.Strings(valid)
	best := valid[len(valid)-1]
	year := best[len(best)-4:]
	return best, year
}
