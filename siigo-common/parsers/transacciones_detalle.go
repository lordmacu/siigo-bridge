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
	VoucherType    string  `json:"tipo_comprobante"`    // F=factura, G=egreso, P=pago, R=recibo
	Company        string  `json:"empresa"`             // 001, 002
	Sequence       string  `json:"secuencia"`           // 12-digit document sequence
	Codigo1        string  `json:"codigo_1"`            // 01, 02
	ThirdPartyNit  string  `json:"nit_tercero"`         // 14-digit NIT (leading zeros stripped)
	CompanyAccount string  `json:"empresa_cuenta"`      // empresa for the account
	LedgerAccount  string  `json:"cuenta_contable"`     // 9-digit PUC account
	SecVoucherType string  `json:"tipo_comp_sec"`       // secondary voucher type (F/G/P/R)
	CompanySec     string  `json:"empresa_sec"`         // secondary empresa
	SequenceSec    string  `json:"secuencia_sec"`       // 13-digit secondary sequence
	Codigo2        string  `json:"codigo_2"`            // secondary code
	DocDate        string  `json:"fecha_documento"`     // YYYYMMDD
	Correlative    string  `json:"correlativo"`         // 0001, 0002
	PeriodCount    string  `json:"num_periodos"`        // period count
	ExtraFields    string  `json:"campos_extra"`        // additional codes
	Reference      string  `json:"referencia"`          // reference codes
	TransType      string  `json:"tipo_transaccion"`   // 52, 53 (transaction subtype)
	DueDate        string  `json:"fecha_vencimiento"`  // YYYYMMDD due date
	MovType        string  `json:"tipo_movimiento"`    // D=debito, C=credito
	Amount         float64 `json:"valor"`               // monetary value (integer pesos)
	Hash           string  `json:"hash"`
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
		if r.LedgerAccount == "" {
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
	voucherType := ""
	if len(rec) > off {
		b := rec[off]
		if b == 'F' || b == 'G' || b == 'P' || b == 'R' {
			voucherType = string(b)
		} else {
			return TransaccionDetalle{}
		}
	}

	company := strings.TrimSpace(isam.ExtractField(rec, off+1, 3))
	seq := strings.TrimSpace(isam.ExtractField(rec, off+4, 12))
	code1 := strings.TrimSpace(isam.ExtractField(rec, off+16, 2))
	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, off+18, 13)), "0")
	companyAccount := strings.TrimSpace(isam.ExtractField(rec, off+32, 3))
	account := strings.TrimSpace(isam.ExtractField(rec, off+35, 9))
	secVoucherType := strings.TrimSpace(isam.ExtractField(rec, off+44, 1))
	companySec := strings.TrimSpace(isam.ExtractField(rec, off+45, 3))
	seqSec := strings.TrimSpace(isam.ExtractField(rec, off+48, 13))
	code2 := strings.TrimSpace(isam.ExtractField(rec, off+61, 3))
	docDate := strings.TrimSpace(isam.ExtractField(rec, off+64, 8))
	correlative := strings.TrimSpace(isam.ExtractField(rec, off+72, 4))
	periodCount := strings.TrimSpace(isam.ExtractField(rec, off+76, 4))
	extraFields := strings.TrimSpace(isam.ExtractField(rec, off+80, 6))
	reference := strings.TrimSpace(isam.ExtractField(rec, off+86, 6))
	transType := strings.TrimSpace(isam.ExtractField(rec, off+92, 2))
	dueDate := strings.TrimSpace(isam.ExtractField(rec, off+94, 8))

	movType := ""
	if len(rec) > off+102 {
		mv := rec[off+102]
		if mv == 'D' || mv == 'C' {
			movType = string(mv)
		}
	}

	// Parse ASCII monetary value (13 digits, integer pesos -- last 4 bytes are padding zeros)
	var amount float64
	if len(rec) > off+115 {
		amountStr := strings.TrimSpace(isam.ExtractField(rec, off+103, 13))
		amountStr = strings.TrimLeft(amountStr, "0")
		if amountStr != "" {
			if v, err := strconv.ParseFloat(amountStr, 64); err == nil {
				amount = v
			}
		}
	}

	// Validate dates
	if !looksLikeDate(docDate) {
		docDate = ""
	}
	if !looksLikeDate(dueDate) {
		dueDate = ""
	}

	return TransaccionDetalle{
		VoucherType:    voucherType,
		Company:        company,
		Sequence:       seq,
		Codigo1:        code1,
		ThirdPartyNit:  nit,
		CompanyAccount: companyAccount,
		LedgerAccount:  account,
		SecVoucherType: secVoucherType,
		CompanySec:     companySec,
		SequenceSec:    seqSec,
		Codigo2:        code2,
		DocDate:        docDate,
		Correlative:    correlative,
		PeriodCount:    periodCount,
		ExtraFields:    extraFields,
		Reference:      reference,
		TransType:      transType,
		DueDate:        dueDate,
		MovType:        movType,
		Amount:         amount,
		Hash:           fmt.Sprintf("%x", hash[:8]),
	}
}
