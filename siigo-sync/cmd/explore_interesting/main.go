package main

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

var decoder = charmap.Windows1252.NewDecoder()

// Archivos interesantes identificados por el scan v2
var interestingFiles = []struct {
	name string
	desc string
}{
	{"Z152014", "Inventario/movimientos con codigos y fechas (66 recs, 320B)"},
	{"Z162014", "Movimiento inventario: productos, fechas, valores (27 recs, 320B)"},
	{"Z232016", "Saldos inventario por producto - posible BCD (12 recs, 441B)"},
	{"Z232015", "Saldos inventario 2015 (12 recs, 441B)"},
	{"Z232014", "Saldos inventario 2014 (12 recs, 441B)"},
	{"Z232013", "Saldos inventario 2013 (9 recs, 441B)"},
	{"Z70", "Gestion de cobro / CRM - notas y seguimiento (5 recs, 512B)"},
	{"ZPRGN", "Programadores/usuarios con emails (6 recs, 256B)"},
	{"Z290000000001", "Notas con nombres de personas (10 recs, 257B)"},
	{"ZCONN", "Conexiones entre modulos Siigo (8 recs, 1024B)"},
	{"Z244T", "Tabla resumen por tercero (19 recs, 107B)"},
	{"Z492014", "Comprobantes 2014 (16 recs, 2295B)"},
	{"Z492013", "Comprobantes 2013 (5 recs, 2295B)"},
	{"ZW011", "Tareas internas con fechas y usuarios (17 recs, 512B)"},
	{"ZW012", "Notas internas con texto largo (13 recs, 256B)"},
	{"ZW010", "Admin tareas (8 recs, 512B)"},
	{"Z06MCCO", "Maestro conceptos CO (1 rec, 524B)"},
	{"Z06MCON", "Maestro conceptos contables"},
	{"Z05M2016", "Movimientos detallados inventario 2016"},
	{"Z05M2015", "Movimientos detallados inventario 2015"},
	{"Z05M2014", "Movimientos detallados inventario 2014"},
	{"Z07D2016", "Detalle libros aux 2016"},
	{"Z07D2015", "Detalle libros aux 2015"},
	{"Z07D2014", "Detalle libros aux 2014"},
	{"Z07D2013", "Detalle libros aux 2013"},
	{"Z07C", "Libros auxiliares consolidado"},
	{"Z070000", "Libros auxiliares base"},
	{"Z077777", "Libros auxiliares especial"},
	{"Z27A2016", "Activos fijos complemento 2016"},
	{"Z27A2015", "Activos fijos complemento 2015"},
	{"Z27A2014", "Activos fijos complemento 2014"},
	{"Z27A2013", "Activos fijos complemento 2013"},
	{"Z11N2016", "Terceros historial 2016"},
	{"Z11N2015", "Terceros historial 2015"},
	{"Z11N2014", "Terceros historial 2014"},
	{"Z11N2013", "Terceros historial 2013"},
	{"Z11I2013", "Log operaciones 2013"},
	{"Z11I2014", "Log operaciones 2014"},
	{"Z11I2015", "Log operaciones 2015"},
	{"Z03CA", "Plan cuentas auxiliar"},
	{"Z09H", "Historial cartera"},
}

func main() {
	dataPath := `C:\DEMOS01`

	for _, f := range interestingFiles {
		path := filepath.Join(dataPath, f.name)

		fmt.Printf("\n%s\n", strings.Repeat("=", 100))
		fmt.Printf("FILE: %-20s | %s\n", f.name, f.desc)
		fmt.Printf("%s\n", strings.Repeat("=", 100))

		records, stats, err := isam.ReadFileV2WithStats(path)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}

		hdr := stats.Header
		fmt.Printf("  Header: magic=0x%08X, org=%d, idxfmt=%d, recMode=%d, recSize=%d\n",
			hdr.Magic, hdr.Organization, hdr.IdxFormat, hdr.RecordMode, hdr.MaxRecordLen)
		fmt.Printf("  Records: %d data | %d deleted | indexed=%v | longRec=%v\n",
			len(records), stats.DeletedCount, hdr.IsIndexed, hdr.LongRecords)
		if hdr.CreationDate != "" {
			fmt.Printf("  Created: %s | Modified: %s\n", hdr.CreationDate, hdr.ModifiedDate)
		}

		if len(records) == 0 {
			fmt.Println("  (no records)")
			continue
		}

		// Show up to 5 records with hex + ASCII
		showCount := 5
		if len(records) < showCount {
			showCount = len(records)
		}

		for i := 0; i < showCount; i++ {
			rec := records[i]
			fmt.Printf("\n  --- Record %d/%d (len=%d) ---\n", i+1, len(records), len(rec))

			// Decode Windows-1252 for display
			decoded, _ := decoder.Bytes(rec)
			if decoded == nil {
				decoded = rec
			}

			// Show hex dump of first 200 bytes
			dumpLen := len(rec)
			if dumpLen > 200 {
				dumpLen = 200
			}

			// Hex + ASCII side by side (16 bytes per line)
			for off := 0; off < dumpLen; off += 16 {
				end := off + 16
				if end > dumpLen {
					end = dumpLen
				}
				chunk := rec[off:end]
				hexStr := hex.EncodeToString(chunk)
				// Format hex with spaces every 2 chars
				var hexParts []string
				for j := 0; j < len(hexStr); j += 2 {
					hexParts = append(hexParts, hexStr[j:j+2])
				}
				hexFormatted := strings.Join(hexParts, " ")

				// ASCII representation
				var ascii []byte
				for _, b := range decoded[off:end] {
					if b >= 0x20 && b <= 0x7E {
						ascii = append(ascii, b)
					} else {
						ascii = append(ascii, '.')
					}
				}

				fmt.Printf("    %04X: %-48s  |%s|\n", off, hexFormatted, string(ascii))
			}

			if len(rec) > 200 {
				// Also show last 50 bytes for BCD detection
				fmt.Printf("    ... (%d bytes omitted) ...\n", len(rec)-200-50)
				tailStart := len(rec) - 50
				if tailStart < 200 {
					tailStart = 200
				}
				for off := tailStart; off < len(rec); off += 16 {
					end := off + 16
					if end > len(rec) {
						end = len(rec)
					}
					chunk := rec[off:end]
					hexStr := hex.EncodeToString(chunk)
					var hexParts []string
					for j := 0; j < len(hexStr); j += 2 {
						hexParts = append(hexParts, hexStr[j:j+2])
					}
					hexFormatted := strings.Join(hexParts, " ")

					var ascii []byte
					for _, b := range decoded[off:end] {
						if b >= 0x20 && b <= 0x7E {
							ascii = append(ascii, b)
						} else {
							ascii = append(ascii, '.')
						}
					}
					fmt.Printf("    %04X: %-48s  |%s|\n", off, hexFormatted, string(ascii))
				}
			}

			// Extract readable text summary
			text := extractText(decoded, 150)
			if text != "" {
				fmt.Printf("  TEXT: %s\n", text)
			}

			// Detect potential BCD zones (consecutive bytes < 0x10)
			bcdZones := detectBCDZones(rec)
			if len(bcdZones) > 0 {
				fmt.Printf("  BCD zones: %s\n", bcdZones)
			}
		}

		// If more records, show a summary
		if len(records) > showCount {
			fmt.Printf("\n  ... and %d more records. Unique text samples from remaining:\n", len(records)-showCount)
			seen := make(map[string]bool)
			extraShown := 0
			for i := showCount; i < len(records) && extraShown < 5; i++ {
				decoded, _ := decoder.Bytes(records[i])
				text := extractText(decoded, 100)
				if text != "" && !seen[text] {
					seen[text] = true
					fmt.Printf("    [%d] %s\n", i+1, text)
					extraShown++
				}
			}
		}
	}
}

func extractText(rec []byte, maxLen int) string {
	var parts []string
	var current []byte

	for _, b := range rec {
		isText := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == ' ' ||
			b == '.' || b == '-' || b == '/' || b == '@' || b == ',' ||
			(b >= '0' && b <= '9') || (b >= 0xC0)
		if isText {
			current = append(current, b)
		} else {
			if len(current) > 3 {
				text := strings.TrimSpace(string(current))
				if len(text) > 2 {
					parts = append(parts, text)
				}
			}
			current = nil
		}
	}
	if len(current) > 3 {
		text := strings.TrimSpace(string(current))
		if len(text) > 2 {
			parts = append(parts, text)
		}
	}

	result := strings.Join(parts, " | ")
	if len(result) > maxLen {
		result = result[:maxLen] + "..."
	}
	return result
}

func detectBCDZones(rec []byte) string {
	var zones []string
	inBCD := false
	start := 0

	for i, b := range rec {
		isBCD := b <= 0x09 || (b >= 0x10 && b <= 0x99 && b&0x0F <= 0x09 && b>>4 <= 0x09)
		// Also detect sign nibbles
		if !isBCD && i > 0 {
			lastNibble := b & 0x0F
			isBCD = lastNibble == 0x0C || lastNibble == 0x0D || lastNibble == 0x0F
		}

		if isBCD && !inBCD {
			start = i
			inBCD = true
		} else if !isBCD && inBCD {
			if i-start >= 4 {
				zones = append(zones, fmt.Sprintf("@%d-%d(%dB)", start, i-1, i-start))
			}
			inBCD = false
		}
	}
	if inBCD && len(rec)-start >= 4 {
		zones = append(zones, fmt.Sprintf("@%d-%d(%dB)", start, len(rec)-1, len(rec)-start))
	}

	if len(zones) > 8 {
		zones = zones[:8]
		zones = append(zones, "...")
	}
	return strings.Join(zones, ", ")
}
