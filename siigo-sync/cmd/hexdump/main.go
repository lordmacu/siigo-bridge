package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
)

func main() {
	file := `C:\DEMOS01\Z49`
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	records, _, err := isam.ReadIsamFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("EXTFH available: %v\n", isam.ExtfhAvailable())
	fmt.Printf("Total records: %d\n\n", len(records))

	// For a few records with different types, scan for ALL non-space data
	targets := []int{1, 29, 37, 81, 229, 1154, 1216, 1668} // space, C, E, F, H, P, R, T
	for _, idx := range targets {
		if idx >= len(records) {
			continue
		}
		rec := records[idx]
		// Find all non-space regions
		fmt.Printf("=== Record %d (len=%d) first byte='%c' ===\n", idx, len(rec), rec[0])

		// Show first 15 bytes always (key area)
		fmt.Printf("  KEY: ")
		for j := 0; j < 15 && j < len(rec); j++ {
			fmt.Printf("%02x ", rec[j])
		}
		fmt.Print(" |")
		for j := 0; j < 15 && j < len(rec); j++ {
			b := rec[j]
			if b >= 0x20 && b <= 0x7E {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")

		// Scan for non-space regions
		inData := false
		dataStart := 0
		for i := 0; i < len(rec); i++ {
			if rec[i] != 0x20 {
				if !inData {
					inData = true
					dataStart = i
				}
			} else {
				if inData {
					// End of data region - show if > 0 bytes
					showRegion(rec, dataStart, i)
					inData = false
				}
			}
		}
		if inData {
			showRegion(rec, dataStart, len(rec))
		}
		fmt.Println()
	}

	// Also scan for date patterns (20YYMMDD or 19YYMMDD) across ALL records
	fmt.Println("=== Scanning for date patterns in first 100 records ===")
	dateFound := 0
	for i := 0; i < 100 && i < len(records); i++ {
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
						fmt.Printf("  Record %d offset %d (0x%04x): %s\n", i, j, j, date)
						dateFound++
					}
				}
			}
		}
	}
	fmt.Printf("Dates found in first 100 records: %d\n", dateFound)
}

func showRegion(rec []byte, start, end int) {
	length := end - start
	if length > 80 {
		// Truncate long regions
		fmt.Printf("  [%04d-%04d] (%d bytes) ", start, end-1, length)
		for j := start; j < start+40 && j < end; j++ {
			b := rec[j]
			if b >= 0x20 && b <= 0x7E {
				fmt.Printf("%c", b)
			} else {
				fmt.Printf("\\x%02x", b)
			}
		}
		fmt.Print("...")
		fmt.Println()
	} else if length >= 1 {
		fmt.Printf("  [%04d-%04d] (%d bytes) hex:", start, end-1, length)
		for j := start; j < end; j++ {
			fmt.Printf(" %02x", rec[j])
		}
		fmt.Print(" |")
		for j := start; j < end; j++ {
			b := rec[j]
			if b >= 0x20 && b <= 0x7E {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}
