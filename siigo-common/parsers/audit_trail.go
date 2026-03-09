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

// AuditTrailTercero represents an audit trail record for third parties from Z11NYYYY.
// Tracks changes made to third-party records: who changed what and when.
type AuditTrailTercero struct {
	ChangeDate     string `json:"fecha_cambio"`          // YYYYMMDD when change was made
	ThirdPartyNit  string `json:"nit_tercero"`           // 13-digit NIT of the third party
	Timestamp      string `json:"timestamp"`             // 8-digit timestamp
	User           string `json:"usuario"`               // 5-char user/department
	PeriodDate     string `json:"fecha_periodo"`         // YYYYMMDD period end date
	DocType        string `json:"tipo_doc"`              // NO=natural, NC=company
	Name           string `json:"nombre"`                // 40-char name of third party
	RepNit         string `json:"nit_representante"`     // 13-digit NIT of legal representative
	RepName        string `json:"nombre_representante"`  // 40-char name of representative
	Address        string `json:"direccion"`             // address (40 chars)
	Email          string `json:"email"`                 // email address
	Hash           string `json:"hash"`
}

// findLatestZ11N finds the most recent Z11NYYYY file with actual data.
// Skips files that are just empty ISAM headers (<=4096 bytes).
func findLatestZ11N(dataPath string) (string, string) {
	pattern := filepath.Join(dataPath, "Z11N[0-9][0-9][0-9][0-9]")
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
		if len(base) < 8 {
			continue
		}
		year := base[4:8]
		if year < "1990" || year > "2099" {
			continue
		}
		// Skip empty files (ISAM header only = ~4096 bytes)
		info, err := os.Stat(m)
		if err != nil || info.Size() <= 4096 {
			continue
		}
		return m, year
	}
	return "", ""
}

// ParseAuditTrailTerceros reads the latest Z11NYYYY file and returns audit trail records.
func ParseAuditTrailTerceros(dataPath string) ([]AuditTrailTercero, string, error) {
	path, year := findLatestZ11N(dataPath)
	if path == "" {
		return nil, "", fmt.Errorf("no Z11NYYYY file found in %s", dataPath)
	}
	return ParseAuditTrailTercerosFile(path, year)
}

// ParseAuditTrailTercerosFile reads a specific Z11N file.
func ParseAuditTrailTercerosFile(path, year string) ([]AuditTrailTercero, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, year, fmt.Errorf("file not found: %s", path)
	}

	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		return nil, year, err
	}

	extfh := isam.ExtfhAvailable()
	var results []AuditTrailTercero
	for _, rec := range records {
		t := parseAuditTrailRecord(rec, extfh)
		if t.ThirdPartyNit == "" || t.Name == "" {
			continue
		}
		results = append(results, t)
	}

	return results, year, nil
}

func parseAuditTrailRecord(rec []byte, extfh bool) AuditTrailTercero {
	if len(rec) < 100 {
		return AuditTrailTercero{}
	}
	hash := sha256.Sum256(rec)
	if extfh {
		return parseAuditTrailEXTFH(rec, hash)
	}
	return parseAuditTrailBinary(rec, hash)
}

// parseAuditTrailEXTFH extracts Z11NYYYY records using verified EXTFH offsets.
// Z11NYYYY structure (846 bytes) — third parties enriched with contact info:
//
//	[0:8]     change date (YYYYMMDD, e.g. "20140725")
//	[8:16]    padding/spaces
//	[16:29]   third-party NIT (13 digits, zero-padded)
//	[33:41]   date2 (YYYYMMDD, duplicate)
//	[41:49]   timestamp (8 chars, e.g. "14313444")
//	[48:53]   user (5 chars, e.g. "ADMON")
//	[56:64]   period date (YYYYMMDD, e.g. "20141231")
//	[64:77]   nit2 (13 digits, duplicate)
//	[80:82]   doc type (NO=natural, NC=company)
//	[82:122]  name (40 chars)
//	[142:158] representative NIT (16 chars, zero-padded)
//	[158:198] representative name (40 chars)
//	[250:290] address (40 chars)
//	[391:438] email (47 chars)
func parseAuditTrailEXTFH(rec []byte, hash [32]byte) AuditTrailTercero {
	changeDate := strings.TrimSpace(isam.ExtractField(rec, 0, 8))
	if changeDate == "" || len(changeDate) < 8 {
		return AuditTrailTercero{}
	}

	nitRaw := strings.TrimSpace(isam.ExtractField(rec, 16, 13))
	nit := strings.TrimLeft(nitRaw, "0")
	if nit == "" {
		return AuditTrailTercero{}
	}

	timestamp := strings.TrimSpace(isam.ExtractField(rec, 41, 8))
	user := strings.TrimSpace(isam.ExtractField(rec, 48, 5))
	periodDate := strings.TrimSpace(isam.ExtractField(rec, 56, 8))

	docType := ""
	if len(rec) >= 82 {
		docType = strings.TrimSpace(isam.ExtractField(rec, 80, 2))
	}

	name := ""
	if len(rec) >= 122 {
		name = strings.TrimSpace(isam.ExtractField(rec, 82, 40))
	}
	if name == "" {
		return AuditTrailTercero{}
	}

	repNit := ""
	if len(rec) >= 158 {
		repRaw := strings.TrimSpace(isam.ExtractField(rec, 142, 16))
		repNit = strings.TrimLeft(repRaw, "0")
	}

	repName := ""
	if len(rec) >= 198 {
		repName = strings.TrimSpace(isam.ExtractField(rec, 158, 40))
	}

	address := ""
	if len(rec) >= 290 {
		address = strings.TrimSpace(isam.ExtractField(rec, 250, 40))
	}

	email := ""
	if len(rec) >= 438 {
		emailRaw := strings.TrimSpace(isam.ExtractField(rec, 391, 47))
		if strings.Contains(emailRaw, "@") && strings.Contains(emailRaw, ".") {
			email = emailRaw
		}
	}

	return AuditTrailTercero{
		ChangeDate:    changeDate,
		ThirdPartyNit: nit,
		Timestamp:     timestamp,
		User:          user,
		PeriodDate:    periodDate,
		DocType:       docType,
		Name:          name,
		RepNit:        repNit,
		RepName:       repName,
		Address:       address,
		Email:         email,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}

// parseAuditTrailBinary extracts Z11NYYYY records in binary mode (+2 byte offset for record markers).
func parseAuditTrailBinary(rec []byte, hash [32]byte) AuditTrailTercero {
	const off = 2

	if len(rec) < 100+off {
		return AuditTrailTercero{}
	}

	changeDate := strings.TrimSpace(isam.ExtractField(rec, 0+off, 8))
	if changeDate == "" || len(changeDate) < 8 {
		return AuditTrailTercero{}
	}

	nitRaw := strings.TrimSpace(isam.ExtractField(rec, 16+off, 13))
	nit := strings.TrimLeft(nitRaw, "0")
	if nit == "" {
		return AuditTrailTercero{}
	}

	timestamp := strings.TrimSpace(isam.ExtractField(rec, 41+off, 8))
	user := strings.TrimSpace(isam.ExtractField(rec, 48+off, 5))
	periodDate := strings.TrimSpace(isam.ExtractField(rec, 56+off, 8))

	docType := ""
	if len(rec) >= 82+off {
		docType = strings.TrimSpace(isam.ExtractField(rec, 80+off, 2))
	}

	name := ""
	if len(rec) >= 122+off {
		name = strings.TrimSpace(isam.ExtractField(rec, 82+off, 40))
	}
	if name == "" {
		return AuditTrailTercero{}
	}

	repNit := ""
	if len(rec) >= 158+off {
		repRaw := strings.TrimSpace(isam.ExtractField(rec, 142+off, 16))
		repNit = strings.TrimLeft(repRaw, "0")
	}

	repName := ""
	if len(rec) >= 198+off {
		repName = strings.TrimSpace(isam.ExtractField(rec, 158+off, 40))
	}

	address := ""
	if len(rec) >= 290+off {
		address = strings.TrimSpace(isam.ExtractField(rec, 250+off, 40))
	}

	email := ""
	if len(rec) >= 438+off {
		emailRaw := strings.TrimSpace(isam.ExtractField(rec, 391+off, 47))
		if strings.Contains(emailRaw, "@") && strings.Contains(emailRaw, ".") {
			email = emailRaw
		}
	}

	return AuditTrailTercero{
		ChangeDate:    changeDate,
		ThirdPartyNit: nit,
		Timestamp:     timestamp,
		User:          user,
		PeriodDate:    periodDate,
		DocType:       docType,
		Name:          name,
		RepNit:        repNit,
		RepName:       repName,
		Address:       address,
		Email:         email,
		Hash:          fmt.Sprintf("%x", hash[:8]),
	}
}
