package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	records, recSize, err := isam.ReadIsamFile(`C:\SIIWI02\ZPILA`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("ZPILA: records=%d recSize=%d EXTFH=%v\n\n", len(records), recSize, isam.ExtfhAvailable())

	// Show first 20 non-empty records
	shown := 0
	for idx, rec := range records {
		if shown >= 20 {
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

		// Show full ASCII content
		s := strings.TrimRight(string(rec), "\x00 ")
		fmt.Printf("[%3d] len=%d: %q\n", idx, len(rec), s)

		// Hex dump first 80 bytes
		limit := 80
		if limit > len(rec) {
			limit = len(rec)
		}
		for off := 0; off < limit; off += 16 {
			end := off + 16
			if end > limit {
				end = limit
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
		fmt.Println()
	}
}
