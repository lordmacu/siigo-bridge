package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func main() {
	file := `C:\SIIWI02\Z11B2026`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	fmt.Printf("=== Deep analysis of %s ===\n\n", file)

	records, stats, err := isam.ReadFileV2WithStats(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total records: %d (deleted: %d)\n", len(records), stats.DeletedCount)
	fmt.Printf("Record size: %d bytes, LongRecords: %v\n\n", stats.Header.MaxRecordLen, stats.Header.LongRecords)

	decoder := charmap.Windows1252.NewDecoder()

	// Classify by first byte
	byType := make(map[byte][]int)
	for i, rec := range records {
		if len(rec) > 0 {
			byType[rec[0]] = append(byType[rec[0]], i)
		}
	}

	// Types in Siigo doc context: F=Factura, G=?, E=Entrada, L=Line/detalle, O=Orden, P=Pedido, etc
	fmt.Printf("=== Type distribution ===\n")
	for b, indices := range byType {
		fmt.Printf("  '%c': %d records\n", b, len(indices))
	}
	fmt.Println()

	// PHASE 1: Detailed hex dump of key zones for a diverse set of records
	// Focus on the data-rich zones: 0-50, 1280-1960, and 2100-2200
	fmt.Printf("=== DETAILED FIELD ANALYSIS ===\n")
	fmt.Printf("(Showing hex + ASCII for data-rich zones)\n\n")

	// Pick 5 diverse rich records from different types
	samples := []struct {
		label string
		idx   int
	}{}

	for _, t := range []byte{'F', 'L', 'E', 'P', 'O', 'G', 'H', 'R', 'S'} {
		indices, ok := byType[t]
		if !ok {
			continue
		}
		// Pick richest
		bestIdx := indices[0]
		bestScore := 0
		for _, idx := range indices {
			score := 0
			for _, b := range records[idx] {
				if b != 0x00 && b != 0x20 && b != 0x30 {
					score++
				}
			}
			if score > bestScore {
				bestScore = score
				bestIdx = idx
			}
		}
		samples = append(samples, struct {
			label string
			idx   int
		}{fmt.Sprintf("Type '%c'", t), bestIdx})
	}

	for _, s := range samples {
		rec := records[s.idx]
		decoded, _ := decoder.Bytes(rec)

		fmt.Printf("╔═══ %s — Record #%d ═══╗\n", s.label, s.idx)

		// Zone 1: Header/Key (0-50)
		fmt.Printf("\n  --- HEADER ZONE (0-50) ---\n")
		hexDumpRange(rec, decoded, 0, 50)

		// Zone 2: Area around offset 50-200 (mostly spaces but check)
		// Skip — already confirmed empty

		// Zone 3: Data zone 1 (1280-1400) — BCD values area
		fmt.Printf("\n  --- DATA ZONE 1: BCD/VALUES (1280-1400) ---\n")
		hexDumpRange(rec, decoded, 1280, 1400)

		// Zone 4: Data zone 2 (1400-1600) — more BCD + text
		fmt.Printf("\n  --- DATA ZONE 2: MIXED (1400-1600) ---\n")
		hexDumpRange(rec, decoded, 1400, 1600)

		// Zone 5: Data zone 3 (1600-1800)
		fmt.Printf("\n  --- DATA ZONE 3 (1600-1800) ---\n")
		hexDumpRange(rec, decoded, 1600, 1800)

		// Zone 6: Data zone 4 (1800-2000)
		fmt.Printf("\n  --- DATA ZONE 4 (1800-2000) ---\n")
		hexDumpRange(rec, decoded, 1800, 2000)

		// Zone 7: Tail zone (1940-2200) — may have amounts
		fmt.Printf("\n  --- TAIL ZONE (2000-2200) ---\n")
		hexDumpRange(rec, decoded, 2000, 2200)

		fmt.Println()
	}

	// PHASE 2: Cross-record comparison for specific offsets
	fmt.Printf("\n=== CROSS-RECORD FIELD COMPARISON (first 20 records of type 'F') ===\n")
	fIndices := byType['F']
	if len(fIndices) > 20 {
		fIndices = fIndices[:20]
	}
	for _, idx := range fIndices {
		rec := records[idx]
		decoded, _ := decoder.Bytes(rec)
		// Show key fields as text
		tipo := string(decoded[0:1])
		empresa := strings.TrimSpace(string(decoded[1:4]))
		numDoc := strings.TrimSpace(string(decoded[10:15]))
		field20 := strings.TrimSpace(string(decoded[20:30]))
		// BCD at 1280+ range
		fmt.Printf("  Rec#%4d: tipo=%s emp=%s numDoc=%s @20=%s | hex@7-9: %02x%02x%02x hex@1430-1440: ",
			idx, tipo, empresa, numDoc, field20,
			rec[7], rec[8], rec[9])
		for j := 1430; j < 1440 && j < len(rec); j++ {
			fmt.Printf("%02x ", rec[j])
		}
		fmt.Println()
	}

	// Same for type 'L' (most common)
	fmt.Printf("\n=== CROSS-RECORD FIELD COMPARISON (20 diverse 'L' records) ===\n")
	lIndices := byType['L']
	step := len(lIndices) / 20
	if step < 1 {
		step = 1
	}
	count := 0
	for i := 0; i < len(lIndices) && count < 20; i += step {
		idx := lIndices[i]
		rec := records[idx]
		decoded, _ := decoder.Bytes(rec)
		tipo := string(decoded[0:1])
		empresa := strings.TrimSpace(string(decoded[1:4]))
		numDoc := strings.TrimSpace(string(decoded[10:15]))
		field20 := strings.TrimSpace(string(decoded[20:30]))
		// Check for NIT-like data in common places
		nit1430 := strings.TrimSpace(safeString(decoded, 1430, 1445))
		val1500 := strings.TrimSpace(safeString(decoded, 1500, 1520))
		fmt.Printf("  Rec#%4d: tipo=%s emp=%s numDoc=%s @20=%s | @1430='%s' @1500='%s'\n",
			idx, tipo, empresa, numDoc, field20, nit1430, val1500)
		count++
	}

	// PHASE 3: Find records with actual text content (names, addresses, etc.)
	fmt.Printf("\n=== SEARCHING FOR TEXT-RICH RECORDS ===\n")
	textRichest := 0
	textRichScore := 0
	for i, rec := range records {
		// Count alphabetic chars (not digits, not spaces)
		alphaCount := 0
		for _, b := range rec {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				alphaCount++
			}
		}
		if alphaCount > textRichScore {
			textRichScore = alphaCount
			textRichest = i
		}
	}
	fmt.Printf("Most text-rich record: #%d with %d alpha chars\n", textRichest, textRichScore)
	if textRichest < len(records) {
		rec := records[textRichest]
		decoded, _ := decoder.Bytes(rec)
		fmt.Printf("  Full text extraction (non-empty zones):\n")
		// Show all non-empty text zones
		for start := 0; start < len(decoded); start += 50 {
			end := start + 50
			if end > len(decoded) {
				end = len(decoded)
			}
			chunk := string(decoded[start:end])
			cleaned := strings.Map(func(r rune) rune {
				if r >= 0x20 && r <= 0x7E {
					return r
				}
				return '.'
			}, chunk)
			trimmed := strings.ReplaceAll(cleaned, ".", "")
			trimmed = strings.ReplaceAll(trimmed, "0", "")
			trimmed = strings.ReplaceAll(trimmed, " ", "")
			if len(trimmed) > 3 {
				fmt.Printf("    @%4d: %s\n", start, cleaned)
			}
		}
	}

	// PHASE 4: BCD decode attempt on consistent binary zones
	fmt.Printf("\n=== BCD DECODE ATTEMPT on consistent binary offsets ===\n")
	fmt.Printf("(Testing offsets that have binary data in 100%% of records)\n")
	bcdOffsets := []struct {
		name   string
		offset int
		length int
	}{
		{"@1280", 1280, 10},
		{"@1290", 1290, 10},
		{"@1300", 1300, 10},
		{"@1310", 1310, 10},
		{"@1320", 1320, 10},
		{"@1330", 1330, 10},
		{"@1340", 1340, 10},
		{"@1350", 1350, 10},
		{"@1360", 1360, 10},
		{"@1370", 1370, 10},
		{"@1430", 1430, 10},
		{"@1440", 1440, 10},
		{"@1500", 1500, 10},
		{"@1510", 1510, 10},
		{"@1520", 1520, 10},
		{"@1530", 1530, 10},
		{"@1720", 1720, 10},
		{"@1730", 1730, 10},
		{"@1800", 1800, 10},
		{"@1820", 1820, 10},
		{"@1950", 1950, 10},
	}

	// Show for 5 diverse records
	sampleIndices := []int{}
	for _, t := range []byte{'F', 'L', 'P', 'E', 'O'} {
		if indices, ok := byType[t]; ok && len(indices) > 0 {
			// Pick one with data
			best := indices[0]
			bestS := 0
			for _, idx := range indices {
				s := 0
				for _, b := range records[idx] {
					if b != 0x00 && b != 0x20 && b != 0x30 {
						s++
					}
				}
				if s > bestS {
					bestS = s
					best = idx
				}
			}
			sampleIndices = append(sampleIndices, best)
		}
	}

	for _, idx := range sampleIndices {
		rec := records[idx]
		fmt.Printf("\n  Record #%d (type '%c'):\n", idx, rec[0])
		for _, bcd := range bcdOffsets {
			if bcd.offset+bcd.length > len(rec) {
				continue
			}
			chunk := rec[bcd.offset : bcd.offset+bcd.length]
			// Check if it looks like BCD (has sign nibble 0x0C, 0x0D, 0x0F in last byte)
			lastByte := chunk[bcd.length-1]
			lastNibble := lastByte & 0x0F
			isBCD := lastNibble == 0x0C || lastNibble == 0x0D || lastNibble == 0x0F
			hex := ""
			for _, b := range chunk {
				hex += fmt.Sprintf("%02x ", b)
			}
			bcdVal := ""
			if isBCD {
				bcdVal = decodeBCD(chunk)
			}
			// Only show if non-zero
			allZero := true
			for _, b := range chunk {
				if b != 0x00 && b != 0x30 && b != 0x20 {
					allZero = false
					break
				}
			}
			if !allZero {
				fmt.Printf("    %s: %s", bcd.name, hex)
				if bcdVal != "" {
					fmt.Printf(" → BCD: %s", bcdVal)
				}
				fmt.Println()
			}
		}
	}
}

func safeString(data []byte, start, end int) string {
	if start >= len(data) {
		return ""
	}
	if end > len(data) {
		end = len(data)
	}
	var sb strings.Builder
	for _, b := range data[start:end] {
		if b >= 0x20 && b <= 0x7E {
			sb.WriteByte(b)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

func hexDumpRange(raw []byte, decoded []byte, start, end int) {
	if start >= len(raw) {
		return
	}
	if end > len(raw) {
		end = len(raw)
	}
	lineWidth := 16
	for i := start; i < end; i += lineWidth {
		rowEnd := i + lineWidth
		if rowEnd > end {
			rowEnd = end
		}
		// Check if all zeros/spaces
		allEmpty := true
		for j := i; j < rowEnd; j++ {
			if raw[j] != 0x00 && raw[j] != 0x20 && raw[j] != 0x30 {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}

		fmt.Printf("  %04x: ", i)
		for j := i; j < rowEnd; j++ {
			fmt.Printf("%02x ", raw[j])
		}
		// Pad if short
		for j := rowEnd; j < i+lineWidth; j++ {
			fmt.Print("   ")
		}
		fmt.Print(" |")
		for j := i; j < rowEnd; j++ {
			if decoded[j] >= 0x20 && decoded[j] <= 0x7E {
				fmt.Printf("%c", decoded[j])
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}

func decodeBCD(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var digits strings.Builder
	for i := 0; i < len(data)-1; i++ {
		hi := (data[i] >> 4) & 0x0F
		lo := data[i] & 0x0F
		digits.WriteByte('0' + byte(hi))
		digits.WriteByte('0' + byte(lo))
	}
	// Last byte: high nibble is digit, low nibble is sign
	lastByte := data[len(data)-1]
	hi := (lastByte >> 4) & 0x0F
	sign := lastByte & 0x0F
	digits.WriteByte('0' + byte(hi))

	signStr := "+"
	if sign == 0x0D {
		signStr = "-"
	}

	result := digits.String()
	// Trim leading zeros
	trimmed := strings.TrimLeft(result, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	return signStr + trimmed
}
