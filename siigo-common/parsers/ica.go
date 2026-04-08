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
	Code string `json:"codigo"` // 5-digit activity code
	Name string `json:"nombre"` // activity description
	Rate string `json:"tarifa"` // tax rate (e.g. "009.66" = 9.66 per mil)
	Hash string `json:"hash"`
}

// ParseICA reads the ZICA file and returns ICA activity codes.
func ParseICA(dataPath string) ([]ActividadICA, error) {
	path := dataPath + "ZICA"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	var activities []ActividadICA
	for _, rec := range records {
		if len(rec) < 10 {
			continue
		}

		code := strings.TrimSpace(isam.ExtractField(rec, 0, 5))
		name := strings.TrimSpace(isam.ExtractField(rec, 5, 50))
		rate := strings.TrimSpace(isam.ExtractField(rec, 55, 6))

		if code == "" || name == "" {
			continue
		}

		// Validate code is numeric
		allDigits := true
		for _, c := range code {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}

		hash := sha256.Sum256(rec)
		activities = append(activities, ActividadICA{
			Code: code,
			Name: name,
			Rate: rate,
			Hash: fmt.Sprintf("%x", hash[:8]),
		})
	}

	return activities, nil
}
