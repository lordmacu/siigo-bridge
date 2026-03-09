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

// Documento represents a document/invoice detail line from Z11YYYY.
// Each record is one line item of a voucher (factura, egreso, ajuste, etc.).
type Documento struct {
	TipoComprobante string `json:"tipo_comprobante"` // F=factura, G=egreso, L=ajuste, P=pedido
	CodigoComp      string `json:"codigo_comp"`      // voucher code (3 chars)
	Secuencia       string `json:"secuencia"`         // line number within document (5 chars)
	NitTercero      string `json:"nit_tercero"`       // third-party NIT
	CuentaContable  string `json:"cuenta_contable"`   // PUC account (13 chars)
	ProductoRef     string `json:"producto_ref"`      // product/inventory reference (7 chars)
	Bodega          string `json:"bodega"`            // warehouse code (3 chars)
	CentroCosto     string `json:"centro_costo"`      // cost center (3 chars)
	Fecha           string `json:"fecha"`             // YYYYMMDD
	Descripcion     string `json:"descripcion"`       // transaction description (50 chars)
	TipoMov         string `json:"tipo_mov"`          // D=debit, C=credit
	Referencia      string `json:"referencia"`        // cross-reference (7 chars)
	Hash            string `json:"hash"`
}

// FindLatestZ11 finds the most recent Z11YYYY file.
func FindLatestZ11(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z11[0-9][0-9][0-9][0-9]")
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
		if len(base) != 7 {
			continue
		}
		year := base[3:]
		return m, year
	}
	return "", ""
}

// ParseDocumentos reads the latest Z11YYYY file and returns document lines.
func ParseDocumentos(dataPath string) ([]Documento, string, error) {
	path, year := FindLatestZ11(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z11YYYY file found in %s", dataPath)
	}
	return ParseDocumentosFile(path, year)
}

// ParseDocumentosFile reads a specific Z11 file.
func ParseDocumentosFile(path, year string) ([]Documento, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var docs []Documento
	for _, rec := range records {
		d := parseDocumentoRecord(rec, extfh)
		if d.Descripcion == "" && d.CuentaContable == "" {
			continue
		}
		docs = append(docs, d)
	}

	return docs, year, nil
}

func parseDocumentoRecord(rec []byte, extfh bool) Documento {
	if len(rec) < 144 {
		return Documento{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseDocumentoEXTFH(rec, hash)
	}
	return parseDocumentoHeuristic(rec, hash)
}

// parseDocumentoEXTFH extracts Z11YYYY records using EXTFH offsets.
// Z11YYYY structure (518 bytes) verified via hex dump:
//
//	[0:1]     tipo_comprobante (letter: F/G/L/P)
//	[1:4]     codigo_comp (3 chars)
//	[10:15]   secuencia (5 chars, line number)
//	[15:26]   BCD/control data + reference number
//	[26:27]   tipo_doc marker ('N')
//	[27:40]   nit_tercero (13 chars, zero-padded left)
//	[40:53]   cuenta_contable (13 chars)
//	[53:61]   fecha (YYYYMMDD) — confirmed at offset 53 via hex scan
//	[93:143]  descripcion (50 chars)
//	[143:144] tipo_mov (D/C)
//	[167:174] referencia (7 chars)
func parseDocumentoEXTFH(rec []byte, hash [32]byte) Documento {
	tipo := ""
	if rec[0] >= 'A' && rec[0] <= 'Z' {
		tipo = string(rec[0])
	}

	codigo := strings.TrimSpace(isam.ExtractField(rec, 1, 3))
	seq := strings.TrimSpace(isam.ExtractField(rec, 10, 5))

	// NIT at offset 27 (after tipo_doc 'N' marker at byte 26).
	// Previously extracted @21(13) which mixed in reference numbers and the 'N' marker.
	nit := strings.TrimLeft(strings.TrimSpace(isam.ExtractField(rec, 27, 13)), "0")

	// Cuenta contable at offset 40 (immediately after NIT field).
	// Previously extracted @29(13) which overlapped with NIT data.
	cuenta := strings.TrimSpace(isam.ExtractField(rec, 40, 13))

	// Product, bodega, cc are embedded in BCD/control area (bytes 15-26),
	// not at the previously documented ASCII offsets
	producto := ""
	bodega := ""
	centroCosto := ""

	// Fecha at offset 53 — confirmed by hex dump scan across all records.
	// Previously tried {55, 53} but offset 55 always produced invalid dates.
	fecha := ""
	if len(rec) >= 61 {
		f := isam.ExtractField(rec, 53, 8)
		if looksLikeDate(f) {
			fecha = f
		}
	}

	desc := ""
	if len(rec) >= 143 {
		desc = strings.TrimSpace(isam.ExtractField(rec, 93, 50))
	}

	tipoMov := ""
	if len(rec) > 143 {
		if rec[143] == 'D' || rec[143] == 'C' {
			tipoMov = string(rec[143])
		}
	}

	ref := ""
	if len(rec) >= 174 {
		ref = strings.TrimSpace(isam.ExtractField(rec, 167, 7))
	}

	if desc == "" && cuenta == "" {
		return Documento{}
	}

	return Documento{
		TipoComprobante: tipo,
		CodigoComp:      codigo,
		Secuencia:       seq,
		NitTercero:      nit,
		CuentaContable:  cuenta,
		ProductoRef:     producto,
		Bodega:          bodega,
		CentroCosto:     centroCosto,
		Fecha:           fecha,
		Descripcion:     desc,
		TipoMov:         tipoMov,
		Referencia:      ref,
		Hash:            fmt.Sprintf("%x", hash[:8]),
	}
}

func parseDocumentoHeuristic(rec []byte, hash [32]byte) Documento {
	desc := findDescripcion(rec, 93)
	if desc == "" {
		return Documento{}
	}
	return Documento{
		Descripcion: desc,
		Hash:        fmt.Sprintf("%x", hash[:8]),
	}
}
