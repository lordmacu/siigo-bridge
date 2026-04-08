//go:build ignore

package main

import (
	"fmt"
	"siigo-common/isam"
	"siigo-common/parsers"
	"strings"
	"math"
)

func main() {
	recs, _, _ := isam.ReadFileV2All(`C:\SIIWI02\Z092026`)
	
	// ALL S and O records for TINNY LOVE (2595829), NIT 901209350, March
	// to understand where the second $3.6M comes from
	fmt.Println("=== ALL records for 2595829, NIT 901209350, March ===")
	for _, r := range recs {
		if len(r) < 100 { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		fecha := string(r[42:50])
		if !strings.HasPrefix(fecha, "202603") { continue }
		codProd := strings.TrimSpace(string(r[67:74]))
		if codProd != "2595829" { continue }
		
		tipo := string(r[0:1])
		cuenta := strings.TrimSpace(string(r[29:42]))
		dc := strings.TrimSpace(string(r[143:144]))
		val175 := parsers.DecodePacked(r[175:185], 2)
		qty := math.Round(val175 / 1000)
		numDoc := int(parsers.DecodePacked(r[4:10], 0))
		seq := strings.TrimSpace(string(r[10:15]))
		
		fmt.Printf("  %s emp=%s numDoc=%d seq=%s cuenta=%s dc=%s val175=%.0f qty=%.0f\n",
			tipo, string(r[1:4]), numDoc, seq, cuenta, dc, val175, qty)
	}
	
	// The key question: Excel shows $7.3M for TINNY LOVE
	// We have $3.6M from F records. Where's the other $3.6M?
	// Maybe it's from the O records at offset 145?
	fmt.Println("\n=== O records with val145 for 2595829 ===")
	for _, r := range recs {
		if len(r) < 500 || r[0] != 'O' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		fecha := string(r[42:50])
		if !strings.HasPrefix(fecha, "202603") { continue }
		codProd := strings.TrimSpace(string(r[67:74]))
		if codProd != "2595829" { continue }
		
		cuenta := strings.TrimSpace(string(r[29:42]))
		val145 := parsers.DecodePacked(r[145:153], 3)
		p487 := parsers.DecodePacked(r[487:495], 2)
		val175 := parsers.DecodePacked(r[175:185], 2)
		numDoc := int(parsers.DecodePacked(r[4:10], 0))
		
		fmt.Printf("  O numDoc=%d cuenta=%s val175=%.2f val145=%.2f p487=%.2f\n",
			numDoc, cuenta, val175, val145, p487)
	}
}
