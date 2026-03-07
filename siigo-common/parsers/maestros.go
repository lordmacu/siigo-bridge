package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Maestro represents a configuration/master record from Siigo Z06 file.
// Z06 is Siigo's master configuration file containing multiple record types:
// A=sucursales, B=bodegas, C=conceptos nomina, I=grupos inventario,
// V=vendedores, X=actividades economicas, Z=zonas, L=lineas activos,
// g=ciudades, k=paises, d=calificaciones, p=formas de pago, etc.
type Maestro struct {
	Tipo        string `json:"tipo"`                  // Record type letter
	Codigo      string `json:"codigo"`
	Nombre      string `json:"nombre"`
	Responsable string `json:"responsable,omitempty"` // A/B types
	Direccion   string `json:"direccion,omitempty"`   // A/B types
	Email       string `json:"email,omitempty"`       // A/B types
	Hash        string `json:"hash"`
}

// ParseMaestros reads the Z06 file and returns master configuration records.
func ParseMaestros(dataPath string) ([]Maestro, error) {
	path := dataPath + "Z06"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var maestros []Maestro
	for _, rec := range records {
		m := parseMaestroRecord(rec, extfh)
		if m.Nombre == "" {
			continue
		}
		maestros = append(maestros, m)
	}

	return maestros, nil
}

// ParseMaestrosPorTipo returns only Z06 records of a specific type
func ParseMaestrosPorTipo(dataPath string, tipo byte) ([]Maestro, error) {
	all, err := ParseMaestros(dataPath)
	if err != nil {
		return nil, err
	}
	var filtered []Maestro
	for _, m := range all {
		if len(m.Tipo) > 0 && m.Tipo[0] == tipo {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

func parseMaestroRecord(rec []byte, extfh bool) Maestro {
	if len(rec) < 50 {
		return Maestro{}
	}

	hash := sha256.Sum256(rec)

	if extfh {
		return parseMaestroEXTFH(rec, hash)
	}
	return parseMaestroHeuristic(rec, hash)
}

// parseMaestroEXTFH extracts Z06 records using verified EXTFH offsets.
// Z06 structure via EXTFH (4096 bytes, multi-type):
//
//	[0:1]   tipo - record type letter (A/B/I/V/X/Z/etc.)
//	[2:9]   codigo - 7 chars (e.g., "0001000")
//	[30:31] tipo_repeat - same letter as [0]
//	[31:51] nombre - 20 chars primary name
//	[70:90] responsable - 20 chars (A/B types)
//	[90:120] direccion - 30 chars (A/B types)
func parseMaestroEXTFH(rec []byte, hash [32]byte) Maestro {
	if len(rec) < 50 {
		return Maestro{}
	}

	tipo := rec[0]

	validTypes := map[byte]bool{
		'A': true, // Sucursales/centros de costo
		'B': true, // Bodegas
		'C': true, // Conceptos nomina
		'I': true, // Grupos de inventario
		'V': true, // Vendedores
		'X': true, // Actividades economicas
		'Z': true, // Zonas
		'L': true, // Lineas de activos fijos
		'g': true, // Ciudades
		'k': true, // Paises
		'd': true, // Calificaciones
		'p': true, // Formas de pago
		'M': true, // Motivos de cobro
		'O': true, // Monedas
		'T': true, // Tipos de empresa
		'Y': true, // Seguros
	}

	if !validTypes[tipo] {
		return Maestro{}
	}

	codigo := strings.TrimSpace(isam.ExtractField(rec, 2, 7))
	nombre := strings.TrimSpace(isam.ExtractField(rec, 31, 20))

	if nombre == "" {
		nombre = findDescripcion(rec, 30)
	}

	m := Maestro{
		Tipo:   string(tipo),
		Codigo: codigo,
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}

	if tipo == 'A' || tipo == 'B' {
		m.Responsable = strings.TrimSpace(isam.ExtractField(rec, 70, 20))
		m.Direccion = strings.TrimSpace(isam.ExtractField(rec, 90, 30))
		emailField := isam.ExtractField(rec, 200, 50)
		if idx := strings.Index(emailField, "@"); idx > 0 {
			start := idx
			for start > 0 && emailField[start-1] != ' ' && emailField[start-1] != 0 {
				start--
			}
			end := idx
			for end < len(emailField) && emailField[end] != ' ' && emailField[end] != 0 {
				end++
			}
			m.Email = emailField[start:end]
		}
	}

	return m
}

func parseMaestroHeuristic(rec []byte, hash [32]byte) Maestro {
	codigo := ""
	nombre := ""

	for i := 0; i < len(rec) && i < 50; i++ {
		if rec[i] >= '0' && rec[i] <= 'Z' && rec[i] != ' ' {
			end := i
			for end < len(rec) && end < i+20 && rec[end] != ' ' && rec[end] >= 0x20 {
				end++
			}
			if end-i >= 2 {
				codigo = isam.ExtractField(rec, i, end-i)
				break
			}
		}
	}

	nombre = findDescripcion(rec, len(codigo))

	if nombre == "" {
		return Maestro{}
	}

	return Maestro{
		Codigo: codigo,
		Nombre: nombre,
		Hash:   fmt.Sprintf("%x", hash[:8]),
	}
}
