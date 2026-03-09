package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-common/isam"
	"strings"
)

// Tercero represents a client/supplier from Siigo Z17 file
type Tercero struct {
	KeyType          string `json:"tipo_clave"`    // G=general, L=linea, N=NIT
	Company          string `json:"empresa"`       // 001
	Code             string `json:"codigo"`        // internal code
	DocType          string `json:"tipo_doc"`      // 13=NIT, 11=CC, etc.
	DocNumber        string `json:"numero_doc"`     // NIT or CC number
	CreationDate     string `json:"fecha_creacion"` // YYYYMMDD
	Name             string `json:"nombre"`        // name/business name
	PreferredAcctType string `json:"tipo_cta_pref"` // D=debit, C=credit
	Hash             string `json:"hash"`          // SHA256 of raw record
}

// ParseTerceros reads the Z17 file and returns all terceros
func ParseTerceros(dataPath string) ([]Tercero, error) {
	path := dataPath + "Z17"
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var thirdParties []Tercero
	for _, rec := range records {
		t := parseTerceroRecord(rec, extfh)
		if t.Name == "" || t.KeyType == "" {
			continue
		}
		thirdParties = append(thirdParties, t)
	}

	return thirdParties, nil
}

// ParseTercerosClientes returns only client/supplier master records (type G)
func ParseTercerosClientes(dataPath string) ([]Tercero, error) {
	all, err := ParseTerceros(dataPath)
	if err != nil {
		return nil, err
	}

	var clients []Tercero
	for _, t := range all {
		if t.KeyType == "G" {
			clients = append(clients, t)
		}
	}
	return clients, nil
}

func parseTerceroRecord(rec []byte, extfh bool) Tercero {
	if len(rec) < 80 {
		return Tercero{}
	}

	hash := sha256.Sum256(rec)

	var t Tercero
	if extfh {
		// EXTFH delivers clean data without 2-byte record markers.
		// Verified offsets from hex analysis of EXTFH output:
		// [0:1]   TipoClave   G/L/N/R
		// [1:4]   Empresa     001
		// [4:18]  Codigo      00000000002001
		// [18:20] TipoDoc     13=NIT, 11=CC, 00=none
		// [20:22] SubTipo     account subcode
		// [22:28] NumeroDoc   050000 (6 chars)
		// [28:36] Fecha       20121030
		// [36:76] Nombre      SUPERMERCADOS LA GRAN ESTRELLA (40 chars)
		// [86:87] CtaPref     D or C
		t = Tercero{
			KeyType:          isam.ExtractField(rec, 0, 1),
			Company:          isam.ExtractField(rec, 1, 3),
			Code:             isam.ExtractField(rec, 4, 14),
			DocType:          isam.ExtractField(rec, 18, 2),
			DocNumber:        isam.ExtractField(rec, 22, 6),
			CreationDate:     isam.ExtractField(rec, 28, 8),
			Name:             isam.ExtractField(rec, 36, 40),
			PreferredAcctType: isam.ExtractField(rec, 86, 1),
			Hash:             fmt.Sprintf("%x", hash[:8]),
		}
	} else {
		// Binary reader includes 2-byte record markers, different offsets
		t = Tercero{
			KeyType:          isam.ExtractField(rec, 0, 1),
			Company:          isam.ExtractField(rec, 1, 3),
			Code:             isam.ExtractField(rec, 4, 14),
			DocType:          isam.ExtractField(rec, 22, 2),
			DocNumber:        isam.ExtractField(rec, 24, 10),
			CreationDate:     isam.ExtractField(rec, 34, 8),
			Name:             isam.ExtractField(rec, 43, 40),
			PreferredAcctType: isam.ExtractField(rec, 83, 1),
			Hash:             fmt.Sprintf("%x", hash[:8]),
		}
	}

	return t
}

// ToFinearomClient converts a Tercero to a map suitable for the Finearom API
func (t *Tercero) ToFinearomClient() map[string]interface{} {
	// Map Siigo doc type to description
	docTypeMap := map[string]string{
		"11": "CC",
		"12": "CE",
		"13": "NIT",
		"22": "TI",
		"31": "NIT",
		"41": "PAS",
	}

	docType := docTypeMap[t.DocType]
	if docType == "" {
		docType = "NIT"
	}

	nit := strings.TrimLeft(t.DocNumber, "0")

	return map[string]interface{}{
		"nit":             nit,
		"client_name":     t.Name,
		"business_name":   t.Name,
		"taxpayer_type":   docType,
		"siigo_codigo":    t.Code,
		"siigo_empresa":   t.Company,
		"siigo_sync_hash": t.Hash,
	}
}
