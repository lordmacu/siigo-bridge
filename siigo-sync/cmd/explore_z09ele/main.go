package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func main() {
	file := `C:\SIIWI02\Z09ELE2026`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	decoder := charmap.Windows1252.NewDecoder()

	fmt.Printf("=== Exploring ISAM file: %s ===\n\n", file)

	records, stats, err := isam.ReadFileV2WithStats(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("File stats:\n")
	fmt.Printf("  Total records: %d\n", len(records))
	fmt.Printf("  Max record len: %d\n", stats.Header.MaxRecordLen)
	fmt.Printf("  Index format:   %d\n", stats.Header.IdxFormat)
	fmt.Printf("  Deleted count:  %d\n", stats.DeletedCount)
	fmt.Printf("  Data types:     %v\n", stats.DataTypes)
	fmt.Println()

	// Show first 20 records
	limit := 20
	if len(records) < limit {
		limit = len(records)
	}

	for i := 0; i < limit; i++ {
		rec := records[i]
		fmt.Printf("========== RECORD %d (len=%d) ==========\n", i, len(rec))

		// Decode full record from Windows-1252
		decoded, err := decoder.Bytes(rec)
		if err != nil {
			decoded = rec // fallback to raw
		}

		// Print full record as printable text with position markers
		fmt.Println("--- Full record (non-printable -> dot, marker every 50 chars) ---")
		printWithMarkers(decoded, 50)

		// Print labeled sections
		sections := []struct {
			name  string
			start int
			end   int
		}{
			{"bytes 0-20    (key/type area)", 0, 20},
			{"bytes 20-50   (identifiers)", 20, 50},
			{"bytes 50-100  (names/dates)", 50, 100},
			{"bytes 100-200 (description)", 100, 200},
			{"bytes 200-400 (extended data)", 200, 400},
			{"bytes 400-600 (tail/amounts)", 400, 600},
			{"bytes 600-800 (CUFE area?)", 600, 800},
			{"bytes 800-1000 (extra)", 800, 1000},
			{"bytes 1000-1152 (final)", 1000, 1152},
		}

		for _, s := range sections {
			start := s.start
			end := s.end
			if start >= len(decoded) {
				continue
			}
			if end > len(decoded) {
				end = len(decoded)
			}
			printSection(s.name, decoded, rec, start, end)
		}

		// Scan for date patterns in this record
		scanDates(rec, i)

		// Scan for non-space regions summary
		fmt.Println("--- Non-space regions ---")
		scanNonSpaceRegions(decoded, rec)

		fmt.Println()
	}

	// Summary: scan all records for common patterns
	fmt.Printf("\n=== SUMMARY across all %d records ===\n", len(records))

	// Check distinct first bytes (tipo)
	tipoMap := make(map[byte]int)
	for _, rec := range records {
		if len(rec) > 0 {
			tipoMap[rec[0]]++
		}
	}
	fmt.Println("First byte distribution (tipo):")
	for b, count := range tipoMap {
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("  '%c' (0x%02x): %d records\n", b, b, count)
		} else {
			fmt.Printf("  0x%02x: %d records\n", b, count)
		}
	}

	// Count date occurrences by offset
	fmt.Println("\nDate pattern offsets (scanning all records):")
	dateOffsets := make(map[int]int)
	scanLimit := len(records)
	if scanLimit > 500 {
		scanLimit = 500
	}
	for i := 0; i < scanLimit; i++ {
		rec := records[i]
		for j := 0; j < len(rec)-8; j++ {
			if isDate(rec, j) {
				dateOffsets[j]++
			}
		}
	}
	// Print offsets with 5+ hits
	for off, count := range dateOffsets {
		if count >= 5 {
			fmt.Printf("  Offset %d (0x%04x): %d records have dates\n", off, off, count)
		}
	}
}

func printWithMarkers(data []byte, markerEvery int) {
	// Print position ruler
	rulerLen := len(data)
	if rulerLen > 600 {
		rulerLen = 600 // limit display
	}

	// Position markers line
	var markers strings.Builder
	for i := 0; i < rulerLen; i++ {
		if i%markerEvery == 0 {
			s := fmt.Sprintf("%d", i)
			markers.WriteString(s)
			i += len(s) - 1
		} else if i%10 == 0 {
			markers.WriteByte('|')
		} else {
			markers.WriteByte(' ')
		}
	}
	fmt.Printf("POS: %s\n", markers.String())

	// Data line
	var line strings.Builder
	for i := 0; i < rulerLen; i++ {
		b := data[i]
		if b >= 0x20 && b <= 0x7E {
			line.WriteByte(b)
		} else {
			line.WriteByte('.')
		}
	}
	fmt.Printf("DAT: %s\n", line.String())

	if len(data) > 600 {
		fmt.Printf("     ... (%d more bytes not shown)\n", len(data)-600)
	}
	fmt.Println()
}

func printSection(name string, decoded []byte, raw []byte, start, end int) {
	fmt.Printf("--- %s ---\n", name)

	// Printable text
	fmt.Printf("  TEXT: |")
	for i := start; i < end; i++ {
		b := decoded[i]
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("%c", b)
		} else {
			fmt.Print(".")
		}
	}
	fmt.Println("|")

	// Hex dump (first 80 bytes of section max)
	hexEnd := end
	if hexEnd-start > 80 {
		hexEnd = start + 80
	}
	fmt.Printf("  HEX:  ")
	for i := start; i < hexEnd; i++ {
		fmt.Printf("%02x ", raw[i])
	}
	if hexEnd < end {
		fmt.Printf("... (%d more)", end-hexEnd)
	}
	fmt.Println()
}

func scanDates(rec []byte, recIdx int) {
	found := false
	for j := 0; j < len(rec)-8; j++ {
		if isDate(rec, j) {
			if !found {
				fmt.Println("--- Dates found ---")
				found = true
			}
			fmt.Printf("  Offset %d (0x%04x): %s\n", j, j, string(rec[j:j+8]))
		}
	}
	if !found {
		fmt.Println("--- No date patterns found ---")
	}
}

func isDate(rec []byte, j int) bool {
	if j+8 > len(rec) {
		return false
	}
	if !((rec[j] == '2' && rec[j+1] == '0') || (rec[j] == '1' && rec[j+1] == '9')) {
		return false
	}
	for k := j; k < j+8; k++ {
		if rec[k] < '0' || rec[k] > '9' {
			return false
		}
	}
	m := (rec[j+4]-'0')*10 + (rec[j+5] - '0')
	d := (rec[j+6]-'0')*10 + (rec[j+7] - '0')
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}

func scanNonSpaceRegions(decoded []byte, raw []byte) {
	inData := false
	dataStart := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != 0x20 && raw[i] != 0x00 {
			if !inData {
				inData = true
				dataStart = i
			}
		} else {
			if inData {
				length := i - dataStart
				if length >= 2 { // skip single-byte noise
					showCompactRegion(decoded, raw, dataStart, i)
				}
				inData = false
			}
		}
	}
	if inData {
		length := len(raw) - dataStart
		if length >= 2 {
			showCompactRegion(decoded, raw, dataStart, len(raw))
		}
	}
}

func showCompactRegion(decoded []byte, raw []byte, start, end int) {
	length := end - start
	// Show text representation
	fmt.Printf("  [%4d-%4d] (%3d bytes) |", start, end-1, length)
	showLen := length
	if showLen > 60 {
		showLen = 60
	}
	for j := start; j < start+showLen; j++ {
		b := decoded[j]
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("%c", b)
		} else {
			fmt.Print(".")
		}
	}
	if showLen < length {
		fmt.Printf("...+%d", length-showLen)
	}
	fmt.Println("|")
}
