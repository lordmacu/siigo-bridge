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
	Tipo         string `json:"tipo"`          // action type (e.g. "Afecta")
	Fondo        string `json:"fondo"`         // fund type: AFP, ARP, CAJA, EPS, ICBF, SENA
	Concepto     string `json:"concepto"`      // concept code: COS, IGE, ING, IRP, LMA, RET, SLN, VAC
	Flags        string `json:"flags"`         // NN, SS, NS flags
	TipoBase     string `json:"tipo_base"`     // base type (IBC)
	BaseCalculo  string `json:"base_calculo"`  // calculation basis: SUEL, IBC, NOAP, IBA
	Hash         string `json:"hash"`
}

// ParsePILA reads the ZPILA file and returns PILA social security concepts.
// EXTFH offsets: tipo@0(8) fondo@8(4) concepto@12(3) flags@30(2) tipoBase@32(4) baseCalculo@36(4)
func ParsePILA(dataPath string) ([]ConceptoPILA, error) {
	path := dataPath + "ZPILA"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	var conceptos []ConceptoPILA
	for _, rec := range records {
		if len(rec) < 40 {
			continue
		}

		tipo := strings.TrimSpace(isam.ExtractField(rec, 0, 8))
		fondo := strings.TrimSpace(isam.ExtractField(rec, 8, 4))
		concepto := strings.TrimSpace(isam.ExtractField(rec, 12, 3))
		flags := strings.TrimSpace(isam.ExtractField(rec, 30, 2))
		tipoBase := strings.TrimSpace(isam.ExtractField(rec, 32, 4))
		baseCalculo := strings.TrimSpace(isam.ExtractField(rec, 36, 4))

		if tipo == "" || fondo == "" || concepto == "" {
			continue
		}

		hash := sha256.Sum256(rec)
		conceptos = append(conceptos, ConceptoPILA{
			Tipo:        tipo,
			Fondo:       fondo,
			Concepto:    concepto,
			Flags:       flags,
			TipoBase:    tipoBase,
			BaseCalculo: baseCalculo,
			Hash:        fmt.Sprintf("%x", hash[:8]),
		})
	}

	return conceptos, nil
}
