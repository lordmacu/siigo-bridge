package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

// LibroAuxiliar represents an entry in the auxiliary ledger (Z07YYYY).
// Each record is a transaction line in the auxiliary books.
type LibroAuxiliar struct {
	Empresa           string  `json:"empresa"`
	CuentaContable    string  `json:"cuenta_contable"`     // PUC account (9 digits)
	TipoComprobante   string  `json:"tipo_comprobante"`    // F=factura, P=pago, G=egreso, R=recibo, L=ajuste
	CodigoComprobante string  `json:"codigo_comprobante"`
	FechaDocumento    string  `json:"fecha_documento"`     // YYYYMMDD
	NitTercero        string  `json:"nit_tercero"`
	NumeroReferencia  string  `json:"numero_referencia"`
	TipoCompSec      string  `json:"tipo_comp_sec"`       // secondary/counter type
	CodigoCompSec     string  `json:"codigo_comp_sec"`
	Saldo             float64 `json:"saldo"`
	Debito            float64 `json:"debito"`
	Credito           float64 `json:"credito"`
	FechaRegistro     string  `json:"fecha_registro"`      // YYYYMMDD
	Hash              string  `json:"hash"`
}

// ParseLibrosAuxiliares reads the latest Z07YYYY file and returns auxiliary ledger entries.
func ParseLibrosAuxiliares(dataPath string) ([]LibroAuxiliar, string, error) {
	path, year := findLatestZ07(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z07YYYY file found in %s", dataPath)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var result []LibroAuxiliar
	for _, rec := range records {
		r := parseLibroAuxiliar(rec, extfh)
		if r.CuentaContable == "" {
			continue
		}
		result = append(result, r)
	}

	return result, year, nil
}

func parseLibroAuxiliar(rec []byte, extfh bool) LibroAuxiliar {
	if len(rec) < 140 {
		return LibroAuxiliar{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseLibroAuxiliarEXTFH(rec, hash)
	}
	return parseLibroAuxiliarBinary(rec, hash)
}

func parseLibroAuxiliarEXTFH(rec []byte, hash [32]byte) LibroAuxiliar {
	empresa := strings.TrimSpace(isam.ExtractField(rec, 7, 3))
	cuenta := strings.TrimSpace(isam.ExtractField(rec, 10, 9))
	tipoComp := strings.TrimSpace(isam.ExtractField(rec, 20, 1))
	codComp := strings.TrimSpace(isam.ExtractField(rec, 21, 3))
	fechaDoc := strings.TrimSpace(isam.ExtractField(rec, 33, 8))
	nit := strings.TrimSpace(isam.ExtractField(rec, 41, 13))
	fechaReg := strings.TrimSpace(isam.ExtractField(rec, 133, 8))
	numRef := strings.TrimSpace(isam.ExtractField(rec, 144, 7))
	tipoCompSec := strings.TrimSpace(isam.ExtractField(rec, 155, 1))
	codCompSec := strings.TrimSpace(isam.ExtractField(rec, 156, 3))

	// Strip leading zeros from NIT and ref
	nit = strings.TrimLeft(nit, "0")
	numRef = strings.TrimLeft(numRef, "0")

	// BCD values
	var saldo, debito, credito float64
	if len(rec) >= 131 {
		saldo = DecodePacked(rec[112:118], 2)
		debito = DecodePacked(rec[118:124], 2)
		credito = DecodePacked(rec[124:131], 2)
	}

	return LibroAuxiliar{
		Empresa:           empresa,
		CuentaContable:    cuenta,
		TipoComprobante:   tipoComp,
		CodigoComprobante: codComp,
		FechaDocumento:    fechaDoc,
		NitTercero:        nit,
		NumeroReferencia:  numRef,
		TipoCompSec:      tipoCompSec,
		CodigoCompSec:     codCompSec,
		Saldo:             saldo,
		Debito:            debito,
		Credito:           credito,
		FechaRegistro:     fechaReg,
		Hash:              fmt.Sprintf("%x", hash[:8]),
	}
}

func parseLibroAuxiliarBinary(rec []byte, hash [32]byte) LibroAuxiliar {
	// Binary fallback: offsets +2 for record markers
	empresa := strings.TrimSpace(isam.ExtractField(rec, 9, 3))
	cuenta := strings.TrimSpace(isam.ExtractField(rec, 12, 9))
	tipoComp := strings.TrimSpace(isam.ExtractField(rec, 22, 1))
	codComp := strings.TrimSpace(isam.ExtractField(rec, 23, 3))
	fechaDoc := strings.TrimSpace(isam.ExtractField(rec, 35, 8))
	nit := strings.TrimSpace(isam.ExtractField(rec, 43, 13))
	fechaReg := strings.TrimSpace(isam.ExtractField(rec, 135, 8))
	numRef := strings.TrimSpace(isam.ExtractField(rec, 146, 7))
	tipoCompSec := strings.TrimSpace(isam.ExtractField(rec, 157, 1))
	codCompSec := strings.TrimSpace(isam.ExtractField(rec, 158, 3))

	nit = strings.TrimLeft(nit, "0")
	numRef = strings.TrimLeft(numRef, "0")

	var saldo, debito, credito float64
	if len(rec) >= 133 {
		saldo = DecodePacked(rec[114:120], 2)
		debito = DecodePacked(rec[120:126], 2)
		credito = DecodePacked(rec[126:133], 2)
	}

	return LibroAuxiliar{
		Empresa:           empresa,
		CuentaContable:    cuenta,
		TipoComprobante:   tipoComp,
		CodigoComprobante: codComp,
		FechaDocumento:    fechaDoc,
		NitTercero:        nit,
		NumeroReferencia:  numRef,
		TipoCompSec:      tipoCompSec,
		CodigoCompSec:     codCompSec,
		Saldo:             saldo,
		Debito:            debito,
		Credito:           credito,
		FechaRegistro:     fechaReg,
		Hash:              fmt.Sprintf("%x", hash[:8]),
	}
}

func findLatestZ07(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z07[0-9][0-9][0-9][0-9]")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", ""
	}
	// Filter out .idx files and special codes (7777, 9999, etc.)
	var valid []string
	for _, m := range matches {
		if strings.HasSuffix(m, ".idx") {
			continue
		}
		year := m[len(m)-4:]
		if year >= "1990" && year <= "2099" {
			valid = append(valid, m)
		}
	}
	if len(valid) == 0 {
		return "", ""
	}
	sort.Strings(valid)
	best := valid[len(valid)-1]
	year := best[len(best)-4:]
	return best, year
}
