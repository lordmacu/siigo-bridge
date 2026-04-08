package main

import (
	"fmt"
	"os"
	"siigo-common/isam"

	"golang.org/x/text/encoding/charmap"
)

var dec = charmap.Windows1252.NewDecoder()

func toPrintable(data []byte) string {
	decoded, err := dec.Bytes(data)
	if err != nil {
		decoded = data
	}
	out := make([]byte, len(decoded))
	for i, b := range decoded {
		if b >= 0x20 && b <= 0x7E {
			out[i] = b
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func showSection(label string, rec []byte, start, end int) {
	if start >= len(rec) {
		fmt.Printf("  %-25s [%d-%d] OUT OF BOUNDS (reclen=%d)\n", label, start, end, len(rec))
		return
	}
	if end > len(rec) {
		end = len(rec)
	}
	data := rec[start:end]
	fmt.Printf("  %-25s [%03d-%03d] |%s|\n", label, start, end-1, toPrintable(data))
	// Also hex for first 30 bytes of the section
	hexLen := len(data)
	if hexLen > 40 {
		hexLen = 40
	}
	fmt.Printf("  %-25s         hex: ", "")
	for i := 0; i < hexLen; i++ {
		fmt.Printf("%02x ", data[i])
	}
	if len(data) > 40 {
		fmt.Print("...")
	}
	fmt.Println()
}

func main() {
	file := `C:\SIIWI02\Z492026`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	fmt.Printf("Reading: %s\n", file)
	records, stats, err := isam.ReadFileV2WithStats(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total records: %d\n", len(records))
	fmt.Printf("Total scanned: %d\n", stats.TotalRecords)
	fmt.Printf("Deleted count: %d\n", stats.DeletedCount)
	if stats.Header != nil {
		fmt.Printf("Record length (header): %d\n", stats.Header.MaxRecordLen)
		fmt.Printf("Index format: %d\n", stats.Header.IdxFormat)
	}
	fmt.Println()

	// Show first 20 records
	limit := 20
	if limit > len(records) {
		limit = len(records)
	}

	for i := 0; i < limit; i++ {
		rec := records[i]
		fmt.Printf("========== RECORD %d (len=%d) ==========\n", i, len(rec))

		// Full record as printable text with position markers every 50 chars
		printable := toPrintable(rec)
		for pos := 0; pos < len(printable); pos += 50 {
			end := pos + 50
			if end > len(printable) {
				end = len(printable)
			}
			fmt.Printf("  @%03d: |%s|\n", pos, printable[pos:end])
		}
		fmt.Println()

		// Known Z49 offsets from CLAUDE.md
		showSection("tipo@0(1)", rec, 0, 1)
		showSection("codigo@1(3)", rec, 1, 4)
		showSection("numDoc@4(11)", rec, 4, 15)
		showSection("nombreTercero@15(57)", rec, 15, 72)
		showSection("desc_zone1@72(57)", rec, 72, 129)
		showSection("desc_zone2@129(64)", rec, 129, 193)

		// Explore additional areas
		showSection("bytes@0-15", rec, 0, 16)
		showSection("bytes@15-75", rec, 15, 75)
		showSection("bytes@72-200", rec, 72, 200)
		showSection("bytes@200-300", rec, 200, 300)

		// Check for longer records
		if len(rec) > 300 {
			showSection("bytes@300-400", rec, 300, 400)
		}
		if len(rec) > 400 {
			showSection("bytes@400-end", rec, 400, len(rec))
		}

		// Scan for date patterns (20YYMMDD) in this record
		for j := 0; j < len(rec)-8; j++ {
			if (rec[j] == '2' && rec[j+1] == '0') || (rec[j] == '1' && rec[j+1] == '9') {
				allDigit := true
				for k := j; k < j+8; k++ {
					if rec[k] < '0' || rec[k] > '9' {
						allDigit = false
						break
					}
				}
				if allDigit {
					date := string(rec[j : j+8])
					m := (date[4]-'0')*10 + (date[5] - '0')
					d := (date[6]-'0')*10 + (date[7] - '0')
					if m >= 1 && m <= 12 && d >= 1 && d <= 31 {
						fmt.Printf("  >>> DATE found at offset %d: %s\n", j, date)
					}
				}
			}
		}

		// Scan for NIT patterns (8-13 digit numbers)
		for j := 0; j < len(rec)-8; j++ {
			digitCount := 0
			for k := j; k < len(rec) && rec[k] >= '0' && rec[k] <= '9'; k++ {
				digitCount++
			}
			if digitCount >= 8 && digitCount <= 13 {
				// Check it's not a date (already found those)
				numStr := string(rec[j : j+digitCount])
				if len(numStr) >= 8 && (numStr[:2] == "20" || numStr[:2] == "19") {
					// Skip dates
				} else {
					fmt.Printf("  >>> POSSIBLE NIT at offset %d (%d digits): %s\n", j, digitCount, numStr)
				}
				j += digitCount // skip past this number
			}
		}

		fmt.Println()
	}

	// Summary: show distribution of first byte (tipo)
	fmt.Println("========== TIPO DISTRIBUTION (first byte) ==========")
	tipos := make(map[byte]int)
	for _, rec := range records {
		if len(rec) > 0 {
			tipos[rec[0]]++
		}
	}
	for b, count := range tipos {
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("  '%c' (0x%02x): %d records\n", b, b, count)
		} else {
			fmt.Printf("  0x%02x: %d records\n", b, count)
		}
	}
}
