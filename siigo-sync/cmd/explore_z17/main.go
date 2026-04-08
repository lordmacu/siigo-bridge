package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func main() {
	file := `C:\SIIWI02\Z17`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	records, stats, err := isam.ReadFileV2WithStats(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file, err)
		os.Exit(1)
	}

	decoder := charmap.Windows1252.NewDecoder()

	fmt.Printf("File: %s\n", file)
	fmt.Printf("EXTFH available: %v\n", isam.ExtfhAvailable())
	fmt.Printf("Total records: %d\n", len(records))
	if stats.Header != nil {
		fmt.Printf("Stats: MaxRecordLen=%d, IdxFormat=%d\n",
			stats.Header.MaxRecordLen, stats.Header.IdxFormat)
	}
	fmt.Printf("Stats: TotalRecords=%d, Deleted=%d, System=%d\n",
		stats.TotalRecords, stats.DeletedCount, stats.SystemCount)
	fmt.Println(strings.Repeat("=", 120))

	limit := 20
	if len(records) < limit {
		limit = len(records)
	}

	for i := 0; i < limit; i++ {
		rec := records[i]
		fmt.Printf("\n%s RECORD %d (len=%d) %s\n",
			strings.Repeat("=", 40), i, len(rec), strings.Repeat("=", 40))

		// Decode full record with Windows-1252
		decoded, _ := decoder.Bytes(rec)

		// Print full record as printable text with position markers
		fmt.Println("\n--- Full record (printable, markers every 50) ---")
		for pos := 0; pos < len(decoded); pos += 50 {
			end := pos + 50
			if end > len(decoded) {
				end = len(decoded)
			}
			fmt.Printf("[%04d] ", pos)
			for j := pos; j < end; j++ {
				b := decoded[j]
				if b >= 0x20 && b <= 0x7E {
					fmt.Printf("%c", b)
				} else {
					fmt.Print(".")
				}
			}
			fmt.Println()
		}

		// Print labeled sections with hex + printable
		sections := []struct {
			name  string
			start int
			end   int
		}{
			{"Bytes 0-20 (keys/tipo)", 0, 20},
			{"Bytes 20-50 (nombre start)", 20, 50},
			{"Bytes 50-100", 50, 100},
			{"Bytes 100-200", 100, 200},
			{"Bytes 200-400", 200, 400},
			{"Bytes 400-600", 400, 600},
			{"Bytes 600-800", 600, 800},
			{"Bytes 800-1000", 800, 1000},
			{"Bytes 1000-1200", 1000, 1200},
			{"Bytes 1200-1438", 1200, 1438},
		}

		for _, s := range sections {
			start := s.start
			end := s.end
			if start >= len(rec) {
				continue
			}
			if end > len(rec) {
				end = len(rec)
			}
			fmt.Printf("\n--- %s ---\n", s.name)

			// Hex dump in rows of 16
			for off := start; off < end; off += 16 {
				lineEnd := off + 16
				if lineEnd > end {
					lineEnd = end
				}
				// Hex
				fmt.Printf("  %04d: ", off)
				for j := off; j < lineEnd; j++ {
					fmt.Printf("%02x ", rec[j])
				}
				// Pad if short
				for j := lineEnd; j < off+16; j++ {
					fmt.Print("   ")
				}
				fmt.Print(" |")
				// ASCII
				for j := off; j < lineEnd; j++ {
					b := decoded[j]
					if b >= 0x20 && b <= 0x7E {
						fmt.Printf("%c", b)
					} else {
						fmt.Print(".")
					}
				}
				fmt.Println("|")
			}
		}

		// Known offsets check
		fmt.Printf("\n--- Known offsets ---\n")
		printField(decoded, "tipoDoc@18", 18, 18)
		printField(decoded, "nombre@36", 36, 50)

		// Scan for date patterns (8-digit YYYYMMDD)
		fmt.Printf("\n--- Date patterns found ---\n")
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
						fmt.Printf("  offset %d (0x%04x): %s\n", j, j, date)
					}
				}
			}
		}

		// Scan for non-space/non-null regions (data islands)
		fmt.Printf("\n--- Non-space data regions ---\n")
		inData := false
		dataStart := 0
		for j := 0; j < len(rec); j++ {
			if rec[j] != 0x20 && rec[j] != 0x00 {
				if !inData {
					inData = true
					dataStart = j
				}
			} else {
				if inData {
					showCompact(decoded, rec, dataStart, j)
					inData = false
				}
			}
		}
		if inData {
			showCompact(decoded, rec, dataStart, len(rec))
		}

		// Look for BCD-like regions (bytes 0x00-0x09, 0x0C, 0x0D, 0x0F in sequence)
		fmt.Printf("\n--- Possible BCD regions (4+ consecutive BCD bytes) ---\n")
		bcdStart := -1
		for j := 0; j < len(rec); j++ {
			b := rec[j]
			isBCD := b <= 0x09 || b == 0x0C || b == 0x0D || b == 0x0F
			if isBCD {
				if bcdStart == -1 {
					bcdStart = j
				}
			} else {
				if bcdStart != -1 && j-bcdStart >= 4 {
					fmt.Printf("  [%04d-%04d] (%d bytes):", bcdStart, j-1, j-bcdStart)
					for k := bcdStart; k < j; k++ {
						fmt.Printf(" %02x", rec[k])
					}
					fmt.Println()
				}
				bcdStart = -1
			}
		}
		if bcdStart != -1 && len(rec)-bcdStart >= 4 {
			fmt.Printf("  [%04d-%04d] (%d bytes):", bcdStart, len(rec)-1, len(rec)-bcdStart)
			for k := bcdStart; k < len(rec); k++ {
				fmt.Printf(" %02x", rec[k])
			}
			fmt.Println()
		}
	}

	// Summary: scan all records for consistent date offsets
	fmt.Printf("\n%s\n", strings.Repeat("=", 120))
	fmt.Println("=== DATE OFFSET FREQUENCY (first 100 records) ===")
	dateOffsets := make(map[int]int)
	scanLimit := 100
	if len(records) < scanLimit {
		scanLimit = len(records)
	}
	for i := 0; i < scanLimit; i++ {
		rec := records[i]
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
						dateOffsets[j]++
					}
				}
			}
		}
	}
	// Print sorted by frequency
	for {
		maxOff := -1
		maxCount := 0
		for off, count := range dateOffsets {
			if count > maxCount {
				maxCount = count
				maxOff = off
			}
		}
		if maxOff == -1 || maxCount < 2 {
			break
		}
		fmt.Printf("  Offset %4d (0x%04x): %d records\n", maxOff, maxOff, maxCount)
		delete(dateOffsets, maxOff)
	}

	// Summary: first byte distribution
	fmt.Println("\n=== FIRST BYTE DISTRIBUTION ===")
	firstBytes := make(map[byte]int)
	for _, rec := range records {
		if len(rec) > 0 {
			firstBytes[rec[0]]++
		}
	}
	for b, count := range firstBytes {
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("  '%c' (0x%02x): %d records\n", b, b, count)
		} else {
			fmt.Printf("  0x%02x: %d records\n", b, count)
		}
	}
}

func printField(decoded []byte, label string, start, maxLen int) {
	end := start + maxLen
	if end > len(decoded) {
		end = len(decoded)
	}
	if start >= len(decoded) {
		return
	}
	s := string(decoded[start:end])
	s = strings.TrimRight(s, " ")
	fmt.Printf("  %s: [%s]\n", label, s)
}

func showCompact(decoded, raw []byte, start, end int) {
	length := end - start
	display := end
	if length > 60 {
		display = start + 60
	}
	fmt.Printf("  [%04d-%04d] (%3d bytes) ", start, end-1, length)
	for j := start; j < display; j++ {
		b := decoded[j]
		if b >= 0x20 && b <= 0x7E {
			fmt.Printf("%c", b)
		} else {
			fmt.Printf("\\x%02x", raw[j])
		}
	}
	if display < end {
		fmt.Print("...")
	}
	fmt.Println()
}
