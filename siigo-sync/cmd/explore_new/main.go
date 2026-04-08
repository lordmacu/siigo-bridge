package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func main() {
	files := []string{
		`C:\SIIWI02\Z262026`,
		`C:\SIIWI02\Z152026`,
		`C:\SIIWI02\Z492026`,
		`C:\SIIWI02\Z162026`,
		`C:\SIIWI02\Z282026`,
		`C:\SIIWI02\Z272026`,
		`C:\SIIWI02\Z24F2026`,
		`C:\SIIWI02\Z142026`,
		`C:\SIIWI02\Z17`,
		`C:\SIIWI02\Z06CP`,
		`C:\SIIWI02\Z11B2026`,
		`C:\SIIWI02\Z09B2026`,
		`C:\SIIWI02\Z82`,
		`C:\SIIWI02\Z09ELE2026`,
		`C:\SIIWI02\ZQUE2026`,
	}

	dec := charmap.Windows1252.NewDecoder()

	for _, path := range files {
		fmt.Printf("\n========== %s ==========\n", path)
		recs, stats, err := isam.ReadFileV2WithStats(path)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		fmt.Printf("Records: %d, RecLen: %d\n", stats.TotalRecords, stats.Header.MaxRecordLen)

		// Show first 5 records as decoded text
		limit := 5
		if len(recs) < limit {
			limit = len(recs)
		}
		for i := 0; i < limit; i++ {
			decoded, _ := dec.Bytes(recs[i])
			// Show printable chars, replace non-printable with dots
			text := make([]byte, len(decoded))
			for j, b := range decoded {
				if b >= 32 && b <= 126 {
					text[j] = b
				} else {
					text[j] = '.'
				}
			}
			// Show first 200 bytes
			show := string(text)
			if len(show) > 200 {
				show = show[:200]
			}
			fmt.Printf("[%d] %s\n", i, strings.TrimRight(show, ". "))
		}
	}
}
