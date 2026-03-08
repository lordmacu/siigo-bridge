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
		{`C:\DEMOS01\Z91PRO`, "Z91PRO (LIBRO DIARIO? 4939 recs)", 3, 200},
		{`C:\DEMOS01\Z120`, "Z120 (CONTROL VERSION? 1557 recs)", 3, 200},
		{`C:\DEMOS01\ZICA`, "ZICA (ICA IMPUESTOS? 431 recs)", 3, 200},
		{`C:\DEMOS01\Z052014`, "Z05 (NOMINA/CREDITOS? 29 recs)", 2, 250},
		{`C:\DEMOS01\Z072016`, "Z07 (LIBROS AUXILIARES 64 recs)", 2, 200},
		{`C:\DEMOS01\Z262016`, "Z26 (??? 34 recs)", 2, 200},
		{`C:\DEMOS01\ZPILA`, "ZPILA (SEGURIDAD SOCIAL 239 recs)", 2, 200},
		{`C:\DEMOS01\Z90ES`, "Z90ES (??? 1816 recs)", 2, 200},
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

		// Find text blocks > 4 chars
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
		isText := isAlpha || b == ' ' || b == '.' || b == '-' || b == '/' || b == ',' || b == '#' || (b >= '0' && b <= '9') || (b >= 0xC0)
		if !inText && isAlpha {
			start = i
			inText = true
		}
		if inText && !isText {
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
