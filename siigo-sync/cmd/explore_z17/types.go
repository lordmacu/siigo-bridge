package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"
	"golang.org/x/text/encoding/charmap"
)

func main() {
	file := `C:\SIIWI02\Z17`
	records, _, err := isam.ReadFileV2WithStats(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	decoder := charmap.Windows1252.NewDecoder()

	// Show 2 examples per first-byte type
	seen := make(map[byte]int)
	for i, rec := range records {
		if len(rec) == 0 { continue }
		t := rec[0]
		if seen[t] >= 2 { continue }
		seen[t]++
		decoded, _ := decoder.Bytes(rec)
		
		// Print first 90 chars
		fmt.Printf("Rec %4d [%c] ", i, t)
		end := 90
		if end > len(decoded) { end = len(decoded) }
		for j := 0; j < end; j++ {
			b := decoded[j]
			if b >= 0x20 && b <= 0x7E {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println()
		
		// Print field breakdown hypothesis
		fmt.Printf("          tipo=%c empresa=%s producto=%s linea=%s ",
			decoded[0],
			strings.TrimLeft(string(decoded[1:4]), "0"),
			strings.TrimRight(strings.TrimLeft(string(decoded[4:16]), "0"), " "),
			strings.TrimLeft(string(decoded[16:18]), "0"),
		)
		fmt.Printf("tipoDoc=%s numDoc=%s fecha?=%s nombre=%s\n",
			string(decoded[18:20]),
			string(decoded[20:24]),
			string(decoded[24:36]),
			strings.TrimRight(string(decoded[36:86]), " "),
		)
		
		// Also bytes 86-100 (C/D field + value)
		fmt.Printf("          D/C=%c valor_ascii=%s\n",
			decoded[86],
			strings.TrimLeft(string(decoded[87:100]), "0"),
		)
		
		// bytes 150-160 for I code area
		fmt.Printf("          bytes[148:155]=%s bytes[155:165]=%s\n",
			string(decoded[148:155]),
			strings.TrimRight(string(decoded[155:175]), " "),
		)
		fmt.Println()
	}
}
