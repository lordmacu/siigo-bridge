package main

import (
	"bytes"
	"fmt"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

var decoder = charmap.Windows1252.NewDecoder()

func toPrintable(data []byte) string {
	decoded, err := decoder.Bytes(data)
	if err != nil {
		decoded = data
	}
	out := make([]byte, len(decoded))
	for i, b := range decoded {
		if b >= 32 && b <= 126 {
			out[i] = b
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func showZoned(label string, rec []byte, start, end int) {
	if end > len(rec) {
		end = len(rec)
	}
	if start >= len(rec) {
		fmt.Printf("  %-20s [%3d-%3d]: <out of bounds>\n", label, start, end)
		return
	}
	text := toPrintable(rec[start:end])
	fmt.Printf("  %-20s [%3d-%3d]: |%s|\n", label, start, end, text)
}

func showHex(label string, rec []byte, start, end int) {
	if end > len(rec) {
		end = len(rec)
	}
	if start >= len(rec) {
		return
	}
	fmt.Printf("  %-20s [%3d-%3d]: ", label, start, end)
	for j := start; j < end; j++ {
		fmt.Printf("%02x ", rec[j])
	}
	fmt.Println()
}

func main() {
	dataDir := `C:\SIIWI02`
	z09path := dataDir + `\Z092026`

	fmt.Println("=== Z09 Product Code Offset Finder ===")
	fmt.Printf("Reading: %s\n\n", z09path)

	recs, stats, err := isam.ReadFileV2WithStats(z09path)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Total records: %d, RecLen: %d\n\n", len(recs), stats.Header.MaxRecordLen)

	simonizNIT := []byte("800203984")
	productSearch := [][]byte{
		[]byte("589453"),
		[]byte("3589453"),
		[]byte("4589453"),
	}

	// ========================================
	// PART 1: SIMONIZ records with zone breakdown
	// ========================================
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("PART 1: Records with NIT 800203984 (SIMONIZ) at bytes 16-29")
	fmt.Println("=" + strings.Repeat("=", 79))

	simonizCount := 0
	for i, rec := range recs {
		if len(rec) < 29 {
			continue
		}
		nitZone := rec[16:29]
		if !bytes.Contains(nitZone, simonizNIT) {
			continue
		}
		simonizCount++
		if simonizCount > 30 {
			continue // count but don't print more than 30
		}

		recLen := len(rec)
		fmt.Printf("\n--- Record #%d (index %d, len=%d) ---\n", simonizCount, i, recLen)

		// Full record as text with position rulers
		printable := toPrintable(rec)
		// Show ruler
		fmt.Print("  POS:  ")
		for p := 0; p < len(printable) && p < 250; p++ {
			if p%25 == 0 {
				fmt.Printf("%-25d", p)
			}
		}
		fmt.Println()
		fmt.Print("  TEXT: ")
		if len(printable) > 250 {
			fmt.Println(printable[:250])
		} else {
			fmt.Println(printable)
		}

		// Zone breakdown
		fmt.Println("  --- Zone breakdown ---")
		showZoned("tipo+empresa", rec, 0, 15)
		showZoned("NIT", rec, 16, 29)
		showZoned("cuenta", rec, 29, 42)
		showZoned("fecha", rec, 42, 50)
		showZoned("UNKNOWN_50_68", rec, 50, 68)
		showZoned("UNKNOWN_68_80", rec, 68, 80)
		showZoned("UNKNOWN_80_93", rec, 80, 93)
		showZoned("descripcion", rec, 93, 133)
		showZoned("UNKNOWN_133_143", rec, 133, 143)
		showZoned("dc", rec, 143, 144)
		if recLen > 144 {
			end := 200
			if end > recLen {
				end = recLen
			}
			showZoned("AFTER_DC_144+", rec, 144, end)
		}

		// Hex dump of interesting zones
		fmt.Println("  --- Hex dump of unknown zones ---")
		showHex("hex 50-80", rec, 50, 80)
		showHex("hex 80-93", rec, 80, 93)
		showHex("hex 133-155", rec, 133, 155)

		// Search for product codes in this record
		for _, ps := range productSearch {
			idx := bytes.Index(rec, ps)
			if idx >= 0 {
				fmt.Printf("  >>> FOUND '%s' at offset %d\n", string(ps), idx)
			}
		}
	}
	fmt.Printf("\nTotal SIMONIZ records: %d\n", simonizCount)

	// ========================================
	// PART 2: Any record containing "589453"
	// ========================================
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PART 2: ALL records containing '589453' anywhere in raw bytes")
	fmt.Println(strings.Repeat("=", 80))

	searchCode := []byte("589453")
	matchCount := 0
	for i, rec := range recs {
		idx := bytes.Index(rec, searchCode)
		if idx < 0 {
			continue
		}
		matchCount++
		if matchCount > 20 {
			continue
		}

		recLen := len(rec)
		nitField := ""
		if recLen > 29 {
			nitField = strings.TrimSpace(toPrintable(rec[16:29]))
		}
		tipoField := ""
		if recLen > 5 {
			tipoField = toPrintable(rec[0:5])
		}
		descField := ""
		if recLen > 133 {
			descField = strings.TrimSpace(toPrintable(rec[93:133]))
		}

		fmt.Printf("\n  Match #%d (index %d, len=%d)\n", matchCount, i, recLen)
		fmt.Printf("    tipo: %s | NIT: %s | desc: %s\n", tipoField, nitField, descField)
		fmt.Printf("    '589453' found at offset: %d\n", idx)

		// Show context around the match
		ctxStart := idx - 10
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd := idx + 20
		if ctxEnd > recLen {
			ctxEnd = recLen
		}
		fmt.Printf("    Context [%d-%d]: |%s|\n", ctxStart, ctxEnd, toPrintable(rec[ctxStart:ctxEnd]))
		showHex("hex context", rec, ctxStart, ctxEnd)

		// Also show the full unknown zone 50-93
		showZoned("zone 50-93", rec, 50, 93)

		// Check for longer codes
		for _, ps := range productSearch {
			if len(ps) > 6 {
				idx2 := bytes.Index(rec, ps)
				if idx2 >= 0 {
					fmt.Printf("    Also found '%s' at offset %d\n", string(ps), idx2)
				}
			}
		}
	}
	fmt.Printf("\nTotal records with '589453': %d\n", matchCount)

	// ========================================
	// PART 3: Frequency analysis of "product-like" data in bytes 50-80
	// ========================================
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PART 3: Sample of unique values at various offset ranges (SIMONIZ records only)")
	fmt.Println(strings.Repeat("=", 80))

	// Collect unique values at different offset ranges from SIMONIZ records
	type rangeInfo struct {
		label      string
		start, end int
	}
	ranges := []rangeInfo{
		{"bytes 4-10", 4, 10},
		{"bytes 50-57", 50, 57},
		{"bytes 57-64", 57, 64},
		{"bytes 64-71", 64, 71},
		{"bytes 68-75", 68, 75},
		{"bytes 71-78", 71, 78},
		{"bytes 75-82", 75, 82},
		{"bytes 78-85", 78, 85},
		{"bytes 82-93", 82, 93},
	}

	for _, r := range ranges {
		fmt.Printf("\n  %s:\n", r.label)
		seen := map[string]int{}
		for _, rec := range recs {
			if len(rec) < 29 {
				continue
			}
			if !bytes.Contains(rec[16:29], simonizNIT) {
				continue
			}
			if len(rec) <= r.end {
				continue
			}
			val := strings.TrimSpace(toPrintable(rec[r.start:r.end]))
			seen[val]++
		}
		count := 0
		for val, n := range seen {
			fmt.Printf("    [%s] x%d\n", val, n)
			count++
			if count >= 25 {
				fmt.Printf("    ... and %d more unique values\n", len(seen)-25)
				break
			}
		}
	}
}
