package parsers

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"strconv"
	"strings"
)

// TransaccionDetalle represents a transaction detail line from the Z07T file.
// Each record is a detail entry in the auxiliary ledger with monetary values
// encoded as ASCII strings (not BCD).
type TransaccionDetalle struct {
	TipoComprobante   string  `json:"tipo_comprobante"`    // F=factura, G=egreso, P=pago, R=recibo
	Empresa           string  `json:"empresa"`             // 001, 002
	Secuencia         string  `json:"secuencia"`           // 12-digit document sequence
	Codigo1           string  `json:"codigo_1"`            // 01, 02
	NitTercero        string  `json:"nit_tercero"`         // 14-digit NIT (leading zeros stripped)
	EmpresaCuenta     string  `json:"empresa_cuenta"`      // empresa for the account
	CuentaContable    string  `json:"cuenta_contable"`     // 9-digit PUC account
	TipoCompSec      string  `json:"tipo_comp_sec"`       // secondary voucher type (F/G/P/R)
	EmpresaSec        string  `json:"empresa_sec"`         // secondary empresa
	SecuenciaSec      string  `json:"secuencia_sec"`       // 13-digit secondary sequence
	Codigo2           string  `json:"codigo_2"`            // secondary code
	FechaDocumento    string  `json:"fecha_documento"`     // YYYYMMDD
	Correlativo       string  `json:"correlativo"`         // 0001, 0002
	NumPeriodos       string  `json:"num_periodos"`        // period count
	CamposExtra       string  `json:"campos_extra"`        // additional codes
	Referencia        string  `json:"referencia"`          // reference codes
	TipoTransaccion   string  `json:"tipo_transaccion"`   // 52, 53 (transaction subtype)
	FechaVencimiento  string  `json:"fecha_vencimiento"`  // YYYYMMDD due date
	TipoMovimiento    string  `json:"tipo_movimiento"`    // D=debito, C=credito
	Valor             float64 `json:"valor"`               // monetary value (integer pesos)
	Hash              string  `json:"hash"`
}

// ParseTransaccionesDetalle reads the Z07T file and returns transaction detail entries.
func ParseTransaccionesDetalle(dataPath string) ([]TransaccionDetalle, error) {
	path := filepath.Join(dataPath, "Z07T")
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, err
	}

	extfh := isam.ExtfhAvailable()
	var result []TransaccionDetalle
	for _, rec := range records {
		r := parseTransaccionDetalle(rec, extfh)
		if r.CuentaContable == "" {
			continue
		}
		result = append(result, r)
	}

	return result, nil
}

func parseTransaccionDetalle(rec []byte, extfh bool) TransaccionDetalle {
	if len(rec) < 120 {
		return TransaccionDetalle{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseTransaccionDetalleEXTFH(rec, hash)
	}
	return parseTransaccionDetalleBinary(rec, hash)
}

func parseTransaccionDetalleEXTFH(rec []byte, hash [32]byte) TransaccionDetalle {
	return parseTransaccionDetalleAtOffset(rec, hash, 0)
}

func parseTransaccionDetalleBinary(rec []byte, hash [32]byte) TransaccionDetalle {
	return parseTransaccionDetalleAtOffset(rec, hash, 2)
}

func parseTransaccionDetalleAtOffset(rec []byte, hash [32]byte, off int) TransaccionDetalle {
	tipo := ""
	if len(rec) > off {
		b := rec[off]
		if b == 'F' || b == 'G' || b == 'P' || b == 'R' {
			tipo = string(b)
		} else {
			return TransaccionDetalle{}
		}
	}

	empresa := strings.TrimSpace(isam.ExtractField(rec, off+1, 3))
	secuencia := strings.TrimSpace(isam.ExtractField(rec, off+4, 12))
	codigo1 := strings.TrimSpace(isam.ExtractField(rec, off+16, 2))
	nit := strings.TrimLeft(isam.ExtractField(rec, off+18, 14), "0")
	empresaCuenta := strings.TrimSpace(isam.ExtractField(rec, off+32, 3))
	cuenta := strings.TrimSpace(isam.ExtractField(rec, off+35, 9))
	tipoCompSec := strings.TrimSpace(isam.ExtractField(rec, off+44, 1))
	empresaSec := strings.TrimSpace(isam.ExtractField(rec, off+45, 3))
	secuenciaSec := strings.TrimSpace(isam.ExtractField(rec, off+48, 13))
	codigo2 := strings.TrimSpace(isam.ExtractField(rec, off+61, 3))
	fechaDoc := strings.TrimSpace(isam.ExtractField(rec, off+64, 8))
	correlativo := strings.TrimSpace(isam.ExtractField(rec, off+72, 4))
	numPeriodos := strings.TrimSpace(isam.ExtractField(rec, off+76, 4))
	camposExtra := strings.TrimSpace(isam.ExtractField(rec, off+80, 6))
	referencia := strings.TrimSpace(isam.ExtractField(rec, off+86, 6))
	tipoTrans := strings.TrimSpace(isam.ExtractField(rec, off+92, 2))
	fechaVenc := strings.TrimSpace(isam.ExtractField(rec, off+94, 8))

	tipoMov := ""
	if len(rec) > off+102 {
		mv := rec[off+102]
		if mv == 'D' || mv == 'C' {
			tipoMov = string(mv)
		}
	}

	// Parse ASCII monetary value (13 digits, integer pesos — last 4 bytes are padding zeros)
	var valor float64
	if len(rec) > off+115 {
		valorStr := strings.TrimSpace(isam.ExtractField(rec, off+103, 13))
		valorStr = strings.TrimLeft(valorStr, "0")
		if valorStr != "" {
			if v, err := strconv.ParseFloat(valorStr, 64); err == nil {
				valor = v
			}
		}
	}

	// Validate dates
	if !looksLikeDate(fechaDoc) {
		fechaDoc = ""
	}
	if !looksLikeDate(fechaVenc) {
		fechaVenc = ""
	}

	return TransaccionDetalle{
		TipoComprobante:  tipo,
		Empresa:          empresa,
		Secuencia:        secuencia,
		Codigo1:          codigo1,
		NitTercero:       nit,
		EmpresaCuenta:    empresaCuenta,
		CuentaContable:   cuenta,
		TipoCompSec:     tipoCompSec,
		EmpresaSec:       empresaSec,
		SecuenciaSec:     secuenciaSec,
		Codigo2:          codigo2,
		FechaDocumento:   fechaDoc,
		Correlativo:      correlativo,
		NumPeriodos:      numPeriodos,
		CamposExtra:      camposExtra,
		Referencia:       referencia,
		TipoTransaccion:  tipoTrans,
		FechaVencimiento: fechaVenc,
		TipoMovimiento:   tipoMov,
		Valor:            valor,
		Hash:             fmt.Sprintf("%x", hash[:8]),
	}
}
