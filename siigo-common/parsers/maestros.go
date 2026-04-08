package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Maestro represents a configuration/master record from Siigo Z06 file.
// Z06 is Siigo's master configuration file containing multiple record types:
// A=branches, B=warehouses, C=payroll concepts, I=inventory groups,
// V=salespeople, X=economic activities, Z=zones, L=fixed asset lines,
// g=cities, k=countries, d=ratings, p=payment methods, etc.
type Maestro struct {
	RecType     string `json:"tipo"`                  // Record type letter
	Code        string `json:"codigo"`
	Name        string `json:"nombre"`
	Responsible string `json:"responsable,omitempty"` // A/B types
	Address     string `json:"direccion,omitempty"`   // A/B types
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
	var masters []Maestro
	for _, rec := range records {
		m := parseMaestroRecord(rec, extfh)
		if m.Name == "" {
			continue
		}
		masters = append(masters, m)
	}

	return masters, nil
}

// ParseMaestrosPorTipo returns only Z06 records of a specific type
func ParseMaestrosPorTipo(dataPath string, recType byte) ([]Maestro, error) {
	all, err := ParseMaestros(dataPath)
	if err != nil {
		return nil, err
	}
	var filtered []Maestro
	for _, m := range all {
		if len(m.RecType) > 0 && m.RecType[0] == recType {
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

	recType := rec[0]

	validTypes := map[byte]bool{
		'A': true, // Branches / cost centers
		'B': true, // Warehouses
		'C': true, // Payroll concepts
		'I': true, // Inventory groups
		'V': true, // Salespeople
		'X': true, // Economic activities
		'Z': true, // Zones
		'L': true, // Fixed asset lines
		'g': true, // Cities
		'k': true, // Countries
		'd': true, // Ratings
		'p': true, // Payment methods
		'M': true, // Billing reasons
		'O': true, // Currencies
		'T': true, // Company types
		'Y': true, // Insurance
	}

	if !validTypes[recType] {
		return Maestro{}
	}

	code := strings.TrimSpace(isam.ExtractField(rec, 2, 7))
	name := strings.TrimSpace(isam.ExtractField(rec, 31, 20))

	if name == "" {
		name = findDescripcion(rec, 30)
	}

	m := Maestro{
		RecType: string(recType),
		Code:    code,
		Name:    name,
		Hash:    fmt.Sprintf("%x", hash[:8]),
	}

	if recType == 'A' || recType == 'B' {
		m.Responsible = strings.TrimSpace(isam.ExtractField(rec, 70, 20))
		m.Address = strings.TrimSpace(isam.ExtractField(rec, 90, 30))
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
	code := ""
	name := ""

	for i := 0; i < len(rec) && i < 50; i++ {
		if rec[i] >= '0' && rec[i] <= 'Z' && rec[i] != ' ' {
			end := i
			for end < len(rec) && end < i+20 && rec[end] != ' ' && rec[end] >= 0x20 {
				end++
			}
			if end-i >= 2 {
				code = isam.ExtractField(rec, i, end-i)
				break
			}
		}
	}

	name = findDescripcion(rec, len(code))

	if name == "" {
		return Maestro{}
	}

	return Maestro{
		Code: code,
		Name: name,
		Hash: fmt.Sprintf("%x", hash[:8]),
	}
}
