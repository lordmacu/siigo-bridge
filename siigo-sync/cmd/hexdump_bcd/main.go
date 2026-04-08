package main

import (
	"fmt"
	"siigo-common/isam"
	"siigo-common/parsers"
)

func main() {
	fmt.Println("=== Z252016 (SALDOS TERCEROS) ===")
	dumpFile(`C:\SIIWI02\Z252016`, 3, 160)

	fmt.Println("\n=== Z282016 (SALDOS CONSOLIDADOS) ===")
	dumpFile(`C:\SIIWI02\Z282016`, 3, 160)

	// Test BCD decode at corrected offsets for Z28
	fmt.Println("\n=== Z28 BCD TEST (corrected offsets) ===")
	testZ28BCD(`C:\SIIWI02\Z282016`)
}

func dumpFile(path string, maxRecs, maxBytes int) {
	records, recSize, err := isam.ReadIsamFile(path)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Records: %d, RecSize: %d\n", len(records), recSize)

	shown := 0
	for idx, rec := range records {
		if shown >= maxRecs {
			break
		}
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

		fmt.Printf("\n  Record %d (len=%d):\n", idx, len(rec))
		limit := maxBytes
		if limit > len(rec) {
			limit = len(rec)
		}
		for off := 0; off < limit; off += 16 {
			end := off + 16
			if end > limit {
				end = limit
			}
			fmt.Printf("    %04X: ", off)
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
	}
}

func testZ28BCD(path string) {
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}

	shown := 0
	for _, rec := range records {
		if shown >= 10 {
			break
		}
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

		empresa := string(rec[0:3])
		cuenta := string(rec[3:12])

		if len(rec) < 62 {
			continue
		}

		// Try BCD at offset 38
		saldoAnt := parsers.DecodePacked(rec[38:46], 2)
		debito := parsers.DecodePacked(rec[46:54], 2)
		credito := parsers.DecodePacked(rec[54:62], 2)

		fmt.Printf("  emp:%s cuenta:%s ant:%.2f deb:%.2f cred:%.2f\n",
			empresa, cuenta, saldoAnt, debito, credito)
	}
}
