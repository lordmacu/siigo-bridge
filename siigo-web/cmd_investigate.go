//go:build ignore

package main

import (
	"fmt"
	"siigo-common/isam"
	"siigo-common/parsers"
	"strings"
)

func main() {
	recs, _, _ := isam.ReadFileV2All(`C:\SIIWI02\Z092026`)
	
	// Problem 1: DIFF 50% - TINNY LOVE (2595829) Excel=$7.3M, API=$3.6M
	// Excel has TWO lines: 3000002595829 + 3010002595829
	// Let's find ALL F records for this product for ANY NIT
	fmt.Println("=== ALL F-412 records for product 2595829 (TINNY LOVE), NIT 901209350 ===")
	for _, r := range recs {
		if len(r) < 500 || r[0] != 'F' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		codProd := strings.TrimSpace(string(r[67:74]))
		if codProd != "2595829" { continue }
		cuenta := strings.TrimSpace(string(r[29:42]))
		fecha := string(r[42:50])
		total := parsers.DecodePacked(r[145:153], 3)
		precio := parsers.DecodePacked(r[487:495], 2)
		numDoc := int(parsers.DecodePacked(r[4:10], 0))
		fmt.Printf("  F: cuenta=%s fecha=%s total=%.2f precio=%.2f numDoc=%d\n", cuenta, fecha, total, precio, numDoc)
	}
	
	// Check ALL record types for this product in March
	fmt.Println("\n=== ALL record types for 2595829, NIT 901209350, March ===")
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
		val145 := 0.0
		precio487 := 0.0
		if len(r) >= 500 {
			val145 = parsers.DecodePacked(r[145:153], 3)
			precio487 = parsers.DecodePacked(r[487:495], 2)
		}
		fmt.Printf("  %s: cuenta=%s dc=%s fecha=%s val175=%.2f val145=%.2f p487=%.2f\n", 
			tipo, cuenta, dc, fecha, val175, val145, precio487)
	}
	
	// Problem 2: Products with code 3010xxx in Excel
	// In Excel: 3010002595829 = prefix 301000 + product 2595829
	// The "301" might mean empresa=001 instead of empresa=003
	// Let's check what empresa the S records have
	fmt.Println("\n=== S records for 2595829, NIT 901209350, March ===")
	for _, r := range recs {
		if len(r) < 100 || r[0] != 'S' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		fecha := string(r[42:50])
		if !strings.HasPrefix(fecha, "202603") { continue }
		codProd := strings.TrimSpace(string(r[67:74]))
		if codProd != "2595829" { continue }
		
		empresa := strings.TrimSpace(string(r[1:4]))
		cuenta := strings.TrimSpace(string(r[29:42]))
		dc := strings.TrimSpace(string(r[143:144]))
		val175 := parsers.DecodePacked(r[175:185], 2)
		fmt.Printf("  S: emp=%s cuenta=%s dc=%s fecha=%s val175=%.2f\n", empresa, cuenta, dc, fecha, val175)
	}
	
	// Check: what does the Excel prefix mean?
	// 300000 = grupo 3, linea 00000
	// 301000 = grupo 3, linea 01000
	// Let's check if there are F records with empresa 001 vs 003
	fmt.Println("\n=== F records empresa for NIT 901209350 ===")
	empCount := map[string]int{}
	for _, r := range recs {
		if len(r) < 500 || r[0] != 'F' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		cuenta := strings.TrimSpace(string(r[29:42]))
		if !strings.Contains(cuenta, "412") { continue }
		empresa := strings.TrimSpace(string(r[1:4]))
		empCount[empresa]++
	}
	for e, c := range empCount {
		fmt.Printf("  empresa=%s: %d F records\n", e, c)
	}
}
