package main

import (
	"fmt"
	"os"
	"regexp"
	"siigo-common/isam"
	"strings"
)

type fileEntry struct {
	path string
	desc string
}

func main() {
	files := []fileEntry{
		{`C:\SIIWI02\Z03CA`, "chart of accounts?"},
		{`C:\SIIWI02\Z052013`, "payroll?"},
		{`C:\SIIWI02\Z072013`, "auxiliary ledgers?"},
		{`C:\SIIWI02\Z082013`, "?"},
		{`C:\SIIWI02\Z102013`, "?"},
		{`C:\SIIWI02\Z112013`, "?"},
		{`C:\SIIWI02\Z142013`, "?"},
		{`C:\SIIWI02\Z152013`, "?"},
		{`C:\SIIWI02\Z162013`, "?"},
		{`C:\SIIWI02\Z19`, "?"},
		{`C:\SIIWI02\Z24`, "?"},
		{`C:\SIIWI02\Z06CP`, "vouchers"},
		{`C:\SIIWI02\Z17B`, "third parties extended?"},
		{`C:\SIIWI02\Z17T`, "third parties extended?"},
		{`C:\SIIWI02\Z51`, "?"},
	}

	for _, f := range files {
		analyzeFile(f.path, f.desc)
	}
}

func analyzeFile(path, desc string) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("FILE: %s  (%s)\n", path, desc)
	fmt.Printf("%s\n", strings.Repeat("=", 80))

	// Check file exists and size
	stat, err := os.Stat(path)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File size: %d bytes\n", stat.Size())

	info, err := isam.ReadFile(path)
	if err != nil {
		fmt.Printf("  ERROR reading ISAM: %v\n", err)
		return
	}

	fmt.Printf("  Record size: %d bytes\n", info.RecordSize)
	fmt.Printf("  Records found: %d\n", len(info.Records))
	fmt.Printf("  Header expected: %d\n", info.Header.ExpectedRecords)
	fmt.Printf("  Has index: %v\n", info.Header.HasIndex)
	fmt.Printf("  Magic: 0x%04X\n", info.Header.Magic)

	if len(info.Records) == 0 {
		fmt.Println("  (no records found)")
		return
	}

	// Show first 3 non-empty records
	shown := 0
	for i, rec := range info.Records {
		if shown >= 3 {
			break
		}
		if isEmptyRecord(rec.Data) {
			continue
		}
		shown++
		fmt.Printf("\n  --- Record %d (index %d, file offset 0x%X) ---\n", shown, i, rec.Offset)
		dumpHexASCII(rec.Data, 200)
		findTextFields(rec.Data)
		findDates(rec.Data)
	}

	// Summary of all records: scan for common patterns
	fmt.Printf("\n  --- SUMMARY across all %d records ---\n", len(info.Records))
	summarizeRecords(info.Records, info.RecordSize)
}

func isEmptyRecord(data []byte) bool {
	for _, b := range data {
		if b != 0 && b != ' ' {
			return false
		}
	}
	return true
}

func dumpHexASCII(data []byte, maxBytes int) {
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	for offset := 0; offset < len(data); offset += 16 {
		end := offset + 16
		if end > len(data) {
			end = len(data)
		}
		row := data[offset:end]

		// Skip rows that are all spaces or nulls
		allEmpty := true
		for _, b := range row {
			if b != 0 && b != ' ' && b != 0xFF {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}

		// Hex
		hex := ""
		for i, b := range row {
			if i > 0 {
				hex += " "
			}
			hex += fmt.Sprintf("%02X", b)
		}
		// Pad hex to 48 chars (16*3-1)
		for len(hex) < 47 {
			hex += " "
		}

		// ASCII
		ascii := ""
		for _, b := range row {
			if b >= 0x20 && b < 0x7F {
				ascii += string(rune(b))
			} else if b >= 0x80 && b < 0xFF {
				ascii += "."
			} else {
				ascii += "."
			}
		}

		fmt.Printf("    %04X: %s  |%s|\n", offset, hex, ascii)
	}
}

func findTextFields(data []byte) {
	fmt.Printf("    Text fields found:\n")
	inText := false
	start := 0
	count := 0

	for i, b := range data {
		isText := (b >= 0x20 && b < 0x7F) || (b >= 0xC0 && b <= 0xFC) // ASCII + accented chars
		if isText && !inText {
			start = i
			inText = true
		} else if !isText && inText {
			length := i - start
			if length >= 4 {
				text := isam.ExtractField(data, start, length)
				if len(strings.TrimSpace(text)) >= 3 {
					fmt.Printf("      @%d-%d (%d bytes): %q\n", start, i-1, length, text)
					count++
				}
			}
			inText = false
		}
	}
	// Check if text extends to end
	if inText {
		length := len(data) - start
		if length >= 4 {
			text := isam.ExtractField(data, start, length)
			if len(strings.TrimSpace(text)) >= 3 {
				fmt.Printf("      @%d-%d (%d bytes): %q\n", start, len(data)-1, length, text)
				count++
			}
		}
	}
	if count == 0 {
		fmt.Printf("      (none >= 4 chars)\n")
	}
}

func findDates(data []byte) {
	// Look for YYYYMMDD date patterns in text form
	dateRe := regexp.MustCompile(`20[12][0-9][01][0-9][0-3][0-9]`)
	text := string(data)
	matches := dateRe.FindAllStringIndex(text, -1)
	if len(matches) > 0 {
		fmt.Printf("    Date patterns (YYYYMMDD):\n")
		for _, m := range matches {
			fmt.Printf("      @%d: %s\n", m[0], text[m[0]:m[1]])
		}
	}

	// Also look for BCD-encoded dates (packed decimal)
	// YYYYMMDD as BCD would be: 0x20 0x13 0x01 0x15 for 20130115
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0x20 && (data[i+1] >= 0x10 && data[i+1] <= 0x29) {
			month := data[i+2]
			day := data[i+3]
			if month >= 0x01 && month <= 0x12 && day >= 0x01 && day <= 0x31 {
				fmt.Printf("    Possible BCD date @%d: %02X%02X%02X%02X (20%02x-%02x-%02x)\n",
					i, data[i], data[i+1], data[i+2], data[i+3],
					data[i+1], month, day)
			}
		}
	}
}

func summarizeRecords(records []isam.Record, recSize int) {
	if len(records) == 0 {
		return
	}

	// For each byte position, count how many records have non-zero, non-space values
	nonEmpty := make([]int, recSize)
	textChar := make([]int, recSize) // count of printable ASCII chars
	digitChar := make([]int, recSize)
	total := len(records)
	if total > 200 {
		total = 200 // sample first 200
	}

	for _, rec := range records[:total] {
		for j := 0; j < len(rec.Data) && j < recSize; j++ {
			b := rec.Data[j]
			if b != 0 && b != ' ' {
				nonEmpty[j]++
			}
			if (b >= 0x20 && b < 0x7F) || (b >= 0xC0 && b <= 0xFC) {
				textChar[j]++
			}
			if b >= '0' && b <= '9' {
				digitChar[j]++
			}
		}
	}

	// Identify field regions: contiguous positions with >50% non-empty
	threshold := total / 2
	fmt.Printf("  Field map (positions with >50%% data in %d sampled records):\n", total)
	inField := false
	fieldStart := 0
	for j := 0; j < recSize; j++ {
		if nonEmpty[j] > threshold && !inField {
			fieldStart = j
			inField = true
		} else if (nonEmpty[j] <= threshold || j == recSize-1) && inField {
			end := j
			if j == recSize-1 && nonEmpty[j] > threshold {
				end = j + 1
			}
			length := end - fieldStart

			// Determine type
			textCount := 0
			digitCount := 0
			for k := fieldStart; k < end; k++ {
				textCount += textChar[k]
				digitCount += digitChar[k]
			}
			avgText := textCount / max(length, 1)
			avgDigit := digitCount / max(length, 1)

			fieldType := "binary"
			if avgText > threshold/2 {
				if avgDigit > avgText*80/100 {
					fieldType = "numeric"
				} else {
					fieldType = "text"
				}
			}

			// Show sample values from first non-empty record
			sample := ""
			for _, rec := range records[:min(total, 5)] {
				if fieldStart < len(rec.Data) {
					endPos := min(end, len(rec.Data))
					val := isam.ExtractField(rec.Data, fieldStart, endPos-fieldStart)
					if val != "" {
						sample = val
						break
					}
				}
			}
			if len(sample) > 40 {
				sample = sample[:40] + "..."
			}

			fmt.Printf("    @%d-%d (%d bytes) [%s] sample: %q\n", fieldStart, end-1, length, fieldType, sample)
			inField = false
		}
	}

	// Count date occurrences across all records
	dateRe := regexp.MustCompile(`20[12][0-9][01][0-9][0-3][0-9]`)
	datePositions := map[int]int{}
	for _, rec := range records[:total] {
		text := string(rec.Data)
		matches := dateRe.FindAllStringIndex(text, -1)
		for _, m := range matches {
			datePositions[m[0]]++
		}
	}
	if len(datePositions) > 0 {
		fmt.Printf("  Date positions (YYYYMMDD found in >10%% of records):\n")
		dateThresh := total / 10
		for pos, count := range datePositions {
			if count > dateThresh {
				// Show sample date
				for _, rec := range records[:min(total, 3)] {
					if pos+8 <= len(rec.Data) {
						fmt.Printf("    @%d: found in %d/%d records, sample: %s\n",
							pos, count, total, string(rec.Data[pos:pos+8]))
						break
					}
				}
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
