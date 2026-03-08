package main

import (
	"fmt"
	"siigo-common/isam"
	"siigo-common/parsers"
)

func main() {
	fmt.Println("=== Z07 DETAILED ANALYSIS ===")

	records, recSize, err := isam.ReadIsamFile(`C:\DEMOS01\Z072016`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Records: %d, RecSize: %d, EXTFH: %v\n\n", len(records), recSize, isam.ExtfhAvailable())

	// Show 10 distributed non-empty records with FULL hex
	step := len(records) / 10
	if step < 1 {
		step = 1
	}
	shown := 0
	for i := 0; i < len(records) && shown < 10; i += step {
		rec := records[i]
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		shown++

		fmt.Printf("Record %d (len=%d):\n", i, len(rec))
		// FULL record dump
		for off := 0; off < len(rec); off += 16 {
			end := off + 16
			if end > len(rec) {
				end = len(rec)
			}
			allEmpty := true
			for j := off; j < end; j++ {
				if rec[j] != 0 && rec[j] != ' ' {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				continue
			}
			fmt.Printf("  %04X: ", off)
			for j := off; j < end; j++ {
				fmt.Printf("%02X ", rec[j])
			}
			for j := end; j < off+16; j++ {
				fmt.Print("   ")
			}
			fmt.Print("| ")
			for j := off; j < end; j++ {
				if rec[j] >= 32 && rec[j] < 127 {
					fmt.Printf("%c", rec[j])
				} else {
					fmt.Print(".")
				}
			}
			fmt.Println()
		}

		// Dates anywhere in record
		for off := 0; off < len(rec)-8; off++ {
			s := string(rec[off : off+8])
			if looksLikeDate(s) {
				fmt.Printf("  FECHA@%d: %s\n", off, s)
			}
		}

		// BCD values (try sizes 5,6,7,8)
		for _, sz := range []int{5, 6, 7, 8} {
			for off := 0; off < len(rec)-sz; off++ {
				lastByte := rec[off+sz-1]
				sign := lastByte & 0x0F
				if sign == 0x0C || sign == 0x0D || sign == 0x0F {
					val := parsers.DecodePacked(rec[off:off+sz], 2)
					if val != 0 && val > -1e12 && val < 1e12 {
						fmt.Printf("  BCD@%d(%d): %.2f\n", off, sz, val)
					}
				}
			}
		}

		fmt.Println()
	}

	// Also show what first byte values look like across all records
	fmt.Println("=== First byte distribution ===")
	firstBytes := map[byte]int{}
	for _, rec := range records {
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if !empty && len(rec) > 0 {
			firstBytes[rec[0]]++
		}
	}
	for b, count := range firstBytes {
		if b >= 32 && b < 127 {
			fmt.Printf("  0x%02X '%c': %d records\n", b, b, count)
		} else {
			fmt.Printf("  0x%02X: %d records\n", b, count)
		}
	}
}

func looksLikeDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	y := s[:4]
	m := (s[4]-'0')*10 + s[5] - '0'
	d := (s[6]-'0')*10 + s[7] - '0'
	return y >= "1990" && y <= "2030" && m >= 1 && m <= 12 && d >= 1 && d <= 31
}
