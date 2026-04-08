package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	files := []struct {
		path  string
		label string
		max   int
		bytes int
	}{
		{`C:\SIIWI02\Z12201`, "Z12201 (1272 recs, 512B)", 5, 200},
		{`C:\SIIWI02\Z122`, "Z122 (1138 recs, 512B)", 5, 200},
		{`C:\SIIWI02\Z052014`, "Z05 CREDITOS/PAGOS (29 recs)", 5, 250},
		{`C:\SIIWI02\Z07T`, "Z07T RESUMEN? (116 recs)", 5, 256},
		{`C:\SIIWI02\Z07S`, "Z07S RESUMEN? (87 recs)", 5, 200},
		{`C:\SIIWI02\Z262016`, "Z26 PERIODOS (34 recs)", 3, 200},
	}

	for _, f := range files {
		dumpFile(f.path, f.label, f.max, f.bytes)
	}
}

func dumpFile(path, label string, maxRecs, maxBytes int) {
	records, recSize, err := isam.ReadIsamFile(path)
	if err != nil {
		fmt.Printf("\n=== %s === ERROR: %v\n", label, err)
		return
	}
	fmt.Printf("\n=== %s === records=%d recSize=%d\n", label, len(records), recSize)

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
			allEmpty := true
			for j := off; j < end; j++ {
				if rec[j] != 0 && rec[j] != ' ' && rec[j] != 0xFF {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				continue
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

		// Find text blocks > 5 chars
		findText(rec, limit)
	}
}

func findText(rec []byte, limit int) {
	if limit > len(rec) {
		limit = len(rec)
	}
	inText := false
	start := 0
	for i := 0; i < limit; i++ {
		b := rec[i]
		isAlpha := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
		isT := isAlpha || b == ' ' || b == '.' || b == '-' || b == '/' || b == ',' || b == '#' || (b >= '0' && b <= '9') || (b >= 0xC0)
		if !inText && isAlpha {
			start = i
			inText = true
		}
		if inText && !isT {
			if i-start > 5 {
				fmt.Printf("    TEXT@%d-%d: %q\n", start, i-1, strings.TrimSpace(string(rec[start:i])))
			}
			inText = false
		}
	}
	if inText && limit-start > 5 {
		fmt.Printf("    TEXT@%d-%d: %q\n", start, limit-1, strings.TrimSpace(string(rec[start:limit])))
	}
}
