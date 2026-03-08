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

// TerceroAmpliado represents an extended third-party record from Z08YYYYA.
// Complements Z17 with additional data: tipo persona, representante legal,
// dirección, email.
type TerceroAmpliado struct {
	Empresa          string `json:"empresa"`           // company code (3 chars)
	Nit              string `json:"nit"`               // NIT (8+ chars at offset 5)
	TipoPersona      string `json:"tipo_persona"`      // NO=natural, NP=juridica
	Nombre           string `json:"nombre"`            // full name (60 chars)
	RepresentanteLegal string `json:"representante_legal"` // legal representative name (60 chars)
	Direccion        string `json:"direccion"`         // address (56 chars)
	Email            string `json:"email"`             // email (~70 chars at offset ~323)
	Hash             string `json:"hash"`
}

// FindLatestZ08A finds the most recent Z08YYYYA file.
func FindLatestZ08A(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z08[0-9][0-9][0-9][0-9]A")
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
		year := base[3:7]
		return m, year
	}
	return "", ""
}

// ParseTercerosAmpliados reads the latest Z08YYYYA file and returns extended third-party data.
func ParseTercerosAmpliados(dataPath string) ([]TerceroAmpliado, string, error) {
	path, year := FindLatestZ08A(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z08YYYYA file found in %s", dataPath)
	}
	return ParseTercerosAmpliadosFile(path, year)
}

// ParseTercerosAmpliadosFile reads a specific Z08A file.
func ParseTercerosAmpliadosFile(path, year string) ([]TerceroAmpliado, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var terceros []TerceroAmpliado
	for _, rec := range records {
		t := parseTerceroAmpliadoRecord(rec, extfh)
		if t.Nit == "" || t.Nombre == "" {
			continue
		}
		terceros = append(terceros, t)
	}

	return terceros, year, nil
}

func parseTerceroAmpliadoRecord(rec []byte, extfh bool) TerceroAmpliado {
	if len(rec) < 78 {
		return TerceroAmpliado{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseTerceroAmpliadoEXTFH(rec, hash)
	}
	return parseTerceroAmpliadoHeuristic(rec, hash)
}

// parseTerceroAmpliadoEXTFH extracts Z08YYYYA records using EXTFH offsets.
// Z08YYYYA structure (1152 bytes) verified via hex dump:
//
//	[0:3]     empresa (3 chars)
//	[5:13]    nit (8 digits, zero-padded left)
//	[16:18]   tipo_persona (NO=natural, NP=juridica)
//	[18:78]   nombre (60 chars)
//	[96:156]  representante_legal (60 chars)
//	[194:250] direccion (56 chars)
//	[323:393] email (~70 chars)
func parseTerceroAmpliadoEXTFH(rec []byte, hash [32]byte) TerceroAmpliado {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 0, 3))

	nitRaw := strings.TrimSpace(isam.ExtractField(rec, 5, 8))
	nit := strings.TrimLeft(nitRaw, "0")
	if nit == "" {
		return TerceroAmpliado{}
	}

	tipoPersona := strings.TrimSpace(isam.ExtractField(rec, 16, 2))
	nombre := strings.TrimSpace(isam.ExtractField(rec, 18, 60))

	if nombre == "" {
		return TerceroAmpliado{}
	}

	repLegal := ""
	if len(rec) >= 156 {
		repLegal = strings.TrimSpace(isam.ExtractField(rec, 96, 60))
		// If same as nombre, it's not a separate representative
		if repLegal == nombre {
			repLegal = ""
		}
	}

	direccion := ""
	if len(rec) >= 250 {
		direccion = strings.TrimSpace(isam.ExtractField(rec, 194, 56))
	}

	email := ""
	if len(rec) >= 393 {
		emailRaw := strings.TrimSpace(isam.ExtractField(rec, 323, 70))
		// Validate it looks like an email
		if strings.Contains(emailRaw, "@") && strings.Contains(emailRaw, ".") {
			email = emailRaw
		}
	}

	return TerceroAmpliado{
		Empresa:            empresa,
		Nit:                nit,
		TipoPersona:        tipoPersona,
		Nombre:             nombre,
		RepresentanteLegal: repLegal,
		Direccion:          direccion,
		Email:              email,
		Hash:               fmt.Sprintf("%x", hash[:8]),
	}
}

func parseTerceroAmpliadoHeuristic(rec []byte, hash [32]byte) TerceroAmpliado {
	nombre := findDescripcion(rec, 18)
	if nombre == "" {
		return TerceroAmpliado{}
	}
	return TerceroAmpliado{
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}
