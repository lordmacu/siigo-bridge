package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	deepDumpFile(`C:\SIIWI02\Z112013`, "Z112013 (ACTIVOS FIJOS?)")
	deepDumpFile(`C:\SIIWI02\Z072013`, "Z072013 (LIBROS AUXILIARES?)")
	deepDumpFile(`C:\SIIWI02\Z202013`, "Z202013 (?)")
	deepDumpFile(`C:\SIIWI02\Z212013`, "Z212013 (?)")
	deepDumpFile(`C:\SIIWI02\Z222013`, "Z222013 (?)")
	deepDumpFile(`C:\SIIWI02\Z252013`, "Z252013 (?)")
	deepDumpFile(`C:\SIIWI02\Z272013`, "Z272013 (?)")
	deepDumpFile(`C:\SIIWI02\Z282013`, "Z282013 (?)")
	deepDumpFile(`C:\SIIWI02\Z552013`, "Z552013 (?)")
	deepDumpFile(`C:\SIIWI02\Z592013`, "Z592013 (?)")
	deepDumpFile(`C:\SIIWI02\Z802013`, "Z802013 (?)")

	// Also check non-year files
	deepDumpFile(`C:\SIIWI02\Z001`, "Z001 (?)")
	deepDumpFile(`C:\SIIWI02\Z003`, "Z003 (?)")
	deepDumpFile(`C:\SIIWI02\Z34`, "Z34 (?)")
	deepDumpFile(`C:\SIIWI02\Z41`, "Z41 (?)")
	deepDumpFile(`C:\SIIWI02\Z56`, "Z56 (?)")
	deepDumpFile(`C:\SIIWI02\Z57`, "Z57 (?)")
	deepDumpFile(`C:\SIIWI02\Z58`, "Z58 (?)")
	deepDumpFile(`C:\SIIWI02\Z70`, "Z70 (?)")
	deepDumpFile(`C:\SIIWI02\Z71`, "Z71 (?)")
	deepDumpFile(`C:\SIIWI02\ZRET`, "ZRET (RETENCIONES?)")
	deepDumpFile(`C:\SIIWI02\ZIVA`, "ZIVA (IVA?)")
	deepDumpFile(`C:\SIIWI02\ZPAIS`, "ZPAIS (PAISES?)")
	deepDumpFile(`C:\SIIWI02\ZDANE`, "ZDANE (CODIGOS DANE?)")
}

func deepDumpFile(path, label string) {
	records, recSize, err := isam.ReadIsamFile(path)
	if err != nil {
		return // silently skip errors
	}
	if len(records) == 0 {
		return // skip empty
	}
	fmt.Printf("\n=== %s === records=%d recSize=%d\n", label, len(records), recSize)

	shown := 0
	for idx, rec := range records {
		if len(rec) < 10 {
			continue
		}
		empty := true
		for _, b := range rec {
			if b != 0x20 && b != 0x00 {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		shown++
		if shown > 3 {
			break
		}

		maxOff := 0
		for j := 0; j < len(rec); j++ {
			if rec[j] != 0x20 && rec[j] != 0x00 {
				maxOff = j
			}
		}
		fmt.Printf("\n  Record %d (data to %d):\n", idx, maxOff)
		limit := maxOff + 32
		if limit > len(rec) {
			limit = len(rec)
		}
		for off := 0; off < limit; off += 32 {
			end := off + 32
			if end > limit {
				end = limit
			}
			hasData := false
			for j := off; j < end; j++ {
				if rec[j] != 0x20 && rec[j] != 0x00 {
					hasData = true
					break
				}
			}
			if !hasData {
				continue
			}
			fmt.Printf("    [%04d] ", off)
			for j := off; j < end; j++ {
				fmt.Printf("%02x ", rec[j])
			}
			for j := end; j < off+32; j++ {
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
		inText := false
		textStart := 0
		for j := 0; j < len(rec); j++ {
			isAlpha := (rec[j] >= 'A' && rec[j] <= 'Z') || (rec[j] >= 'a' && rec[j] <= 'z')
			isReadable := isAlpha || rec[j] == ' ' || rec[j] == '.' || rec[j] == '-' || rec[j] == '/' || (rec[j] >= '0' && rec[j] <= '9') || (rec[j] >= 0xC0)
			if !inText && isAlpha {
				inText = true
				textStart = j
			}
			if inText && !isReadable {
				if j-textStart > 4 {
					fmt.Printf("    TEXT@%d: '%s'\n", textStart, strings.TrimSpace(string(rec[textStart:j])))
				}
				inText = false
			}
		}
		if inText && len(rec)-textStart > 4 {
			fmt.Printf("    TEXT@%d: '%s'\n", textStart, strings.TrimSpace(string(rec[textStart:])))
		}

		// Find dates
		for j := 0; j < len(rec)-8; j++ {
			if rec[j] >= '1' && rec[j] <= '2' {
				allD := true
				for k := j; k < j+8; k++ {
					if rec[k] < '0' || rec[k] > '9' {
						allD = false
						break
					}
				}
				if allD {
					d := string(rec[j:j+8])
					if d >= "19900101" && d <= "20301231" {
						m := (d[4]-'0')*10 + d[5] - '0'
						dy := (d[6]-'0')*10 + d[7] - '0'
						if m >= 1 && m <= 12 && dy >= 1 && dy <= 31 {
							fmt.Printf("    DATE@%d: %s\n", j, d)
						}
					}
				}
			}
		}
	}
}
