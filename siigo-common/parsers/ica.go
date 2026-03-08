package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// ActividadICA represents an ICA tax activity code from the ZICA file.
// ICA = Impuesto de Industria y Comercio (Colombian municipal tax).
type ActividadICA struct {
	Codigo string `json:"codigo"` // 5-digit activity code
	Nombre string `json:"nombre"` // activity description
	Tarifa string `json:"tarifa"` // tax rate (e.g. "009.66" = 9.66 per mil)
	Hash   string `json:"hash"`
}

// ParseICA reads the ZICA file and returns ICA activity codes.
func ParseICA(dataPath string) ([]ActividadICA, error) {
	path := dataPath + "ZICA"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	var actividades []ActividadICA
	for _, rec := range records {
		if len(rec) < 10 {
			continue
		}

		codigo := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
		nombre := strings.TrimSpace(isam.ExtractField(rec, 5, 50))
		tarifa := strings.TrimSpace(isam.ExtractField(rec, 55, 6))

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
		actividades = append(actividades, ActividadICA{
			Codigo: codigo,
			Nombre: nombre,
			Tarifa: tarifa,
			Hash:   fmt.Sprintf("%x", hash[:8]),
		})
	}

	return actividades, nil
}
