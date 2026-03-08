package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// CodigoDane represents a Colombian municipality from the ZDANE file.
type CodigoDane struct {
	Codigo string `json:"codigo"` // 5-digit DANE code
	Nombre string `json:"nombre"` // municipality name
	Hash   string `json:"hash"`
}

// ParseDane reads the ZDANE file and returns DANE municipality codes.
func ParseDane(dataPath string) ([]CodigoDane, error) {
	path := dataPath + "ZDANE"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	var codigos []CodigoDane
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}

		codigo := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
		nombre := strings.TrimSpace(isam.ExtractField(rec, 5, 40))

		if codigo == "" || nombre == "" {
			continue
		}

		// Validate codigo is numeric
		allDigits := true
		for _, c := range codigo {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}

		hash := sha256.Sum256(rec)
		codigos = append(codigos, CodigoDane{
			Codigo: codigo,
			Nombre: nombre,
			Hash:   fmt.Sprintf("%x", hash[:8]),
		})
	}

	return codigos, nil
}
