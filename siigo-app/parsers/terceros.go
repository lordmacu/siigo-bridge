package parsers

import (
	"crypto/sha256"
	"fmt"
	"siigo-app/isam"
	"strings"
)

// Tercero represents a client/supplier from Siigo Z17 file
type Tercero struct {
	TipoClave    string `json:"tipo_clave"`    // G=general, L=linea, N=NIT
	Empresa      string `json:"empresa"`       // 001
	Codigo       string `json:"codigo"`        // internal code
	TipoDoc      string `json:"tipo_doc"`      // 13=NIT, 11=CC, etc.
	NumeroDoc    string `json:"numero_doc"`     // NIT or CC number
	FechaCreacion string `json:"fecha_creacion"` // YYYYMMDD
	TipoTercero  string `json:"tipo_tercero"`  // A=active?
	Nombre       string `json:"nombre"`        // name/business name
	TipoCtaPref  string `json:"tipo_cta_pref"` // D=debit, C=credit
	Hash         string `json:"hash"`          // SHA256 of raw record
}

// ParseTerceros reads the Z17 file and returns all terceros
func ParseTerceros(dataPath string) ([]Tercero, error) {
	path := dataPath + "Z17"
	info, err := isam.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var terceros []Tercero
	for _, rec := range info.Records {
		t := parseTerceroRecord(rec.Data)
		if t.Nombre == "" || t.TipoClave == "" {
			continue
		}
		// Only include main records (G type = general/master)
		terceros = append(terceros, t)
	}

	return terceros, nil
}

// ParseTercerosClientes returns only client/supplier master records (type G)
func ParseTercerosClientes(dataPath string) ([]Tercero, error) {
	all, err := ParseTerceros(dataPath)
	if err != nil {
		return nil, err
	}

	var clientes []Tercero
	for _, t := range all {
		if t.TipoClave == "G" {
			clientes = append(clientes, t)
		}
	}
	return clientes, nil
}

func parseTerceroRecord(rec []byte) Tercero {
	if len(rec) < 84 {
		return Tercero{}
	}

	// Z17 record structure (after 2-byte marker):
	// 0:  TipoClave (1)   - G, L, N
	// 1:  Empresa (3)     - 001
	// 4:  Codigo (14)     - 00000000000020
	// 18: Secuencial (4)  - 01
	// 22: TipoDoc (2)     - 13=NIT, 11=CC
	// 24: NumeroDoc (10)  - 3005000020
	// 34: FechaCreacion (8) - 20121030
	// 42: TipoTercero (1) - A
	// 43: Nombre (40)     - PROVEEDORES
	// 83: TipoCtaPref (1) - D, C

	hash := sha256.Sum256(rec)

	t := Tercero{
		TipoClave:     isam.ExtractField(rec, 0, 1),
		Empresa:       isam.ExtractField(rec, 1, 3),
		Codigo:        isam.ExtractField(rec, 4, 14),
		TipoDoc:       isam.ExtractField(rec, 22, 2),
		NumeroDoc:     isam.ExtractField(rec, 24, 10),
		FechaCreacion: isam.ExtractField(rec, 34, 8),
		TipoTercero:   isam.ExtractField(rec, 42, 1),
		Nombre:        isam.ExtractField(rec, 43, 40),
		TipoCtaPref:   isam.ExtractField(rec, 83, 1),
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}

	return t
}

// ToFinearomClient converts a Tercero to a map suitable for the Finearom API
func (t *Tercero) ToFinearomClient() map[string]interface{} {
	// Map Siigo tipo_doc to description
	tipoDocMap := map[string]string{
		"11": "CC",
		"12": "CE",
		"13": "NIT",
		"22": "TI",
		"31": "NIT",
		"41": "PAS",
	}

	tipoDoc := tipoDocMap[t.TipoDoc]
	if tipoDoc == "" {
		tipoDoc = "NIT"
	}

	nit := strings.TrimLeft(t.NumeroDoc, "0")

	return map[string]interface{}{
		"nit":             nit,
		"client_name":     t.Nombre,
		"business_name":   t.Nombre,
		"taxpayer_type":   tipoDoc,
		"siigo_codigo":    t.Codigo,
		"siigo_empresa":   t.Empresa,
		"siigo_sync_hash": t.Hash,
	}
}
