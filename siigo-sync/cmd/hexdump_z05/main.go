package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	records, recSize, err := isam.ReadIsamFile(`C:\SIIWI02\Z052014`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Z052014: records=%d recSize=%d EXTFH=%v\n\n", len(records), recSize, isam.ExtfhAvailable())

	shown := 0
	for idx, rec := range records {
		if shown >= 8 {
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

		fmt.Printf("Record %d (len=%d):\n", idx, len(rec))
		limit := 300
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

		// Find text blocks
		inText := false
		start := 0
		for i := 0; i < limit; i++ {
			b := rec[i]
			isAlpha := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
			isT := isAlpha || b == ' ' || b == '.' || b == '-' || b == '/' || (b >= '0' && b <= '9')
			if !inText && isAlpha {
				start = i
				inText = true
			}
			if inText && !isT {
				if i-start > 4 {
					fmt.Printf("  TEXT@%d-%d: %q\n", start, i-1, strings.TrimSpace(string(rec[start:i])))
				}
				inText = false
			}
		}
		if inText && limit-start > 4 {
			fmt.Printf("  TEXT@%d-%d: %q\n", start, limit-1, strings.TrimSpace(string(rec[start:limit])))
		}
		fmt.Println()
	}
}
