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
	}{
		{`C:\DEMOS01\Z052014`, "Z05 (NOMINA/CREDITOS?)", 2},
		{`C:\DEMOS01\Z072016`, "Z07 (LIBROS AUXILIARES)", 2},
		{`C:\DEMOS01\Z262016`, "Z26 (???)", 2},
		{`C:\DEMOS01\Z182013`, "Z18 (???)", 2},
		{`C:\DEMOS01\ZDANE`, "ZDANE (CODIGOS DANE)", 3},
		{`C:\DEMOS01\ZPILA`, "ZPILA (PILA/SEGURIDAD SOCIAL)", 2},
	}

	for _, f := range files {
		dumpFile(f.path, f.label, f.max)
	}
}

func dumpFile(path, label string, maxRecs int) {
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
		limit := 200
		if limit > len(rec) {
			limit = len(rec)
		}
		for off := 0; off < limit; off += 16 {
			end := off + 16
			if end > limit {
				end = limit
			}
			// Skip empty rows
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

		// Find text blocks
		findText(rec)
	}
}

func findText(rec []byte) {
	inText := false
	start := 0
	for i, b := range rec {
		isAlpha := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
		isText := isAlpha || b == ' ' || b == '.' || b == '-' || b == '/' || (b >= '0' && b <= '9') || (b >= 0xC0)
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
	if inText && len(rec)-start > 5 {
		fmt.Printf("    TEXT@%d-%d: %q\n", start, len(rec)-1, strings.TrimSpace(string(rec[start:])))
	}
}
