package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// PeriodoContable represents a fiscal period from Siigo Z26YYYY files.
// Each record defines a fiscal period with start/end dates and BCD balance totals.
// Z26YYYY files use year suffix: Z262016, Z262014, etc.
type PeriodoContable struct {
	Empresa       string  `json:"empresa"`        // company code from config flags
	NumeroPeriodo string  `json:"numero_periodo"` // period number (0001, 0002, ...)
	FechaInicio   string  `json:"fecha_inicio"`   // YYYYMMDD start date
	FechaFin      string  `json:"fecha_fin"`      // YYYYMMDD end date (00000000 = open)
	Estado        string  `json:"estado"`          // "abierto" or "cerrado"
	ConfigFlags   string  `json:"config_flags"`   // raw config bytes @0-31 (ASCII 0/1 flags)
	Saldo1        float64 `json:"saldo1"`         // BCD value at @60 (7 bytes, 2 decimals)
	Saldo2        float64 `json:"saldo2"`         // BCD value at @67 (7 bytes, 2 decimals)
	Saldo3        float64 `json:"saldo3"`         // BCD value at @74 (7 bytes, 2 decimals)
	Hash          string  `json:"hash"`
}

// ParsePeriodos reads the latest Z26YYYY file and returns fiscal period entries.
func ParsePeriodos(dataPath string) ([]PeriodoContable, string, error) {
	path, year := findLatestZ26(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z26YYYY file found in %s", dataPath)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var result []PeriodoContable
	for _, rec := range records {
		p := parsePeriodoRecord(rec, extfh)
		if p.NumeroPeriodo == "" {
			continue
		}
		result = append(result, p)
	}

	return result, year, nil
}

func parsePeriodoRecord(rec []byte, extfh bool) PeriodoContable {
	if len(rec) < 60 {
		return PeriodoContable{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parsePeriodoEXTFH(rec, hash)
	}
	return parsePeriodoBinary(rec, hash)
}

// parsePeriodoEXTFH extracts fiscal period records using EXTFH offsets.
// Z26YYYY structure (1544 bytes) verified via hex dump:
//
//	[0:32]    config_flags - ASCII 0/1 flags (period metadata)
//	[32:36]   padding / additional flags
//	[36:40]   ignored
//	[40:44]   numero_periodo - 4-char ASCII period number ("0001", "0002", ...)
//	[44:52]   fecha_inicio - YYYYMMDD start date
//	[52:60]   fecha_fin - YYYYMMDD end date (00000000 = open period)
//	[60:67]   BCD saldo1 (7 bytes packed decimal, 2 decimals)
//	[67:74]   BCD saldo2 (7 bytes packed decimal, 2 decimals)
//	[74:81]   BCD saldo3 (7 bytes packed decimal, 2 decimals)
func parsePeriodoEXTFH(rec []byte, hash [32]byte) PeriodoContable {
	configFlags := isam.ExtractField(rec, 0, 32)

	numPeriodo := strings.TrimSpace(isam.ExtractField(rec, 40, 4))
	if numPeriodo == "" || numPeriodo == "0000" {
		return PeriodoContable{}
	}

	// Validate numPeriodo is numeric
	for _, c := range numPeriodo {
		if c < '0' || c > '9' {
			return PeriodoContable{}
		}
	}

	fechaInicio := strings.TrimSpace(isam.ExtractField(rec, 44, 8))
	fechaFin := strings.TrimSpace(isam.ExtractField(rec, 52, 8))

	// Empresa is encoded in the config flags pattern - extract from position 4 (after "0000")
	// Pattern shows: "00001" = empresa 1, "00002" = empresa 2, etc.
	empresa := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 0, 5)), "0")
	if empresa == "" {
		empresa = "1"
	}

	// Determine estado based on fecha_fin
	estado := "cerrado"
	if fechaFin == "00000000" || fechaFin == "" {
		estado = "abierto"
		fechaFin = ""
	}

	// Validate dates
	if fechaInicio != "" && !looksLikeDate(fechaInicio) {
		fechaInicio = ""
	}
	if fechaFin != "" && !looksLikeDate(fechaFin) {
		fechaFin = ""
	}

	// BCD saldos at @60
	var saldo1, saldo2, saldo3 float64
	if len(rec) >= 81 {
		saldo1 = DecodePacked(rec[60:67], 2)
		saldo2 = DecodePacked(rec[67:74], 2)
		saldo3 = DecodePacked(rec[74:81], 2)
	}

	return PeriodoContable{
		Empresa:       empresa,
		NumeroPeriodo: numPeriodo,
		FechaInicio:   fechaInicio,
		FechaFin:      fechaFin,
		Estado:        estado,
		ConfigFlags:   configFlags,
		Saldo1:        saldo1,
		Saldo2:        saldo2,
		Saldo3:        saldo3,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}

// parsePeriodoBinary extracts fiscal period records using binary fallback offsets (+2).
func parsePeriodoBinary(rec []byte, hash [32]byte) PeriodoContable {
	if len(rec) < 62 {
		return PeriodoContable{}
	}

	configFlags := isam.ExtractField(rec, 2, 32)

	numPeriodo := strings.TrimSpace(isam.ExtractField(rec, 42, 4))
	if numPeriodo == "" || numPeriodo == "0000" {
		return PeriodoContable{}
	}

	for _, c := range numPeriodo {
		if c < '0' || c > '9' {
			return PeriodoContable{}
		}
	}

	fechaInicio := strings.TrimSpace(isam.ExtractField(rec, 46, 8))
	fechaFin := strings.TrimSpace(isam.ExtractField(rec, 54, 8))

	empresa := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 2, 5)), "0")
	if empresa == "" {
		empresa = "1"
	}

	estado := "cerrado"
	if fechaFin == "00000000" || fechaFin == "" {
		estado = "abierto"
		fechaFin = ""
	}

	if fechaInicio != "" && !looksLikeDate(fechaInicio) {
		fechaInicio = ""
	}
	if fechaFin != "" && !looksLikeDate(fechaFin) {
		fechaFin = ""
	}

	var saldo1, saldo2, saldo3 float64
	if len(rec) >= 83 {
		saldo1 = DecodePacked(rec[62:69], 2)
		saldo2 = DecodePacked(rec[69:76], 2)
		saldo3 = DecodePacked(rec[76:83], 2)
	}

	return PeriodoContable{
		Empresa:       empresa,
		NumeroPeriodo: numPeriodo,
		FechaInicio:   fechaInicio,
		FechaFin:      fechaFin,
		Estado:        estado,
		ConfigFlags:   configFlags,
		Saldo1:        saldo1,
		Saldo2:        saldo2,
		Saldo3:        saldo3,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}

// findLatestZ26 finds the most recent Z26YYYY file in the data directory.
func findLatestZ26(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z26[0-9][0-9][0-9][0-9]")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", ""
	}
	// Filter out .idx files and invalid years
	var valid []string
	for _, m := range matches {
		if strings.HasSuffix(strings.ToLower(m), ".idx") {
			continue
		}
		base := filepath.Base(m)
		year := base[3:]
		if year >= "1990" && year <= "2099" {
			valid = append(valid, m)
		}
	}
	if len(valid) == 0 {
		return "", ""
	}
	sort.Strings(valid)
	best := valid[len(valid)-1]
	year := filepath.Base(best)[3:]
	return best, year
}
