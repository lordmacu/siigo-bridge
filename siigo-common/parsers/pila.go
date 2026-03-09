package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// ConceptoPILA represents a social security (PILA) configuration concept.
// PILA = Planilla Integrada de Liquidación de Aportes (Colombian social security).
type ConceptoPILA struct {
	RecType  string `json:"tipo"`          // action type (e.g. "Afecta")
	Fund     string `json:"fondo"`         // fund type: AFP, ARP, CAJA, EPS, ICBF, SENA
	Concept  string `json:"concepto"`      // concept code: COS, IGE, ING, IRP, LMA, RET, SLN, VAC
	Flags    string `json:"flags"`         // NN, SS, NS flags
	BaseType string `json:"tipo_base"`     // base type (IBC)
	CalcBase string `json:"base_calculo"`  // calculation basis: SUEL, IBC, NOAP, IBA
	Hash     string `json:"hash"`
}

// ParsePILA reads the ZPILA file and returns PILA social security concepts.
// EXTFH offsets: tipo@0(8) fondo@8(4) concepto@12(3) flags@30(2) tipoBase@32(4) baseCalculo@36(4)
func ParsePILA(dataPath string) ([]ConceptoPILA, error) {
	path := dataPath + "ZPILA"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	var concepts []ConceptoPILA
	for _, rec := range records {
		if len(rec) < 40 {
			continue
		}

		recType := strings.TrimSpace(isam.ExtractField(rec, 0, 8))
		fund := strings.TrimSpace(isam.ExtractField(rec, 8, 4))
		concept := strings.TrimSpace(isam.ExtractField(rec, 12, 3))
		flags := strings.TrimSpace(isam.ExtractField(rec, 30, 2))
		baseType := strings.TrimSpace(isam.ExtractField(rec, 32, 4))
		calcBase := strings.TrimSpace(isam.ExtractField(rec, 36, 4))

		if recType == "" || fund == "" || concept == "" {
			continue
		}

		hash := sha256.Sum256(rec)
		concepts = append(concepts, ConceptoPILA{
			RecType:  recType,
			Fund:     fund,
			Concept:  concept,
			Flags:    flags,
			BaseType: baseType,
			CalcBase: calcBase,
			Hash:     fmt.Sprintf("%x", hash[:8]),
		})
	}

	return concepts, nil
}
