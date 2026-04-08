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
	
	// Key insight: Excel code 3010002595829 = grupo 01, product 2595829
	// But in Z09 we only have cuenta 4120470200 for this product
	// The "grupo 01" products might be in DIFFERENT records with the SAME product code
	// but different empresa or different document
	
	// Let's check: ALL F records for NIT 901209350 in March with cuenta 470100
	// and see what products they have
	fmt.Println("=== F cuenta 4120470100, NIT 901209350, March ===")
	total470100 := 0.0
	for _, r := range recs {
		if len(r) < 500 || r[0] != 'F' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		fecha := string(r[42:50])
		if !strings.HasPrefix(fecha, "202603") { continue }
		cuenta := strings.TrimSpace(string(r[29:42]))
		if cuenta != "0004120470100" { continue }
		
		codProd := strings.TrimSpace(string(r[67:74]))
		total := parsers.DecodePacked(r[145:153], 3)
		precio := parsers.DecodePacked(r[487:495], 2)
		total470100 += total
		fmt.Printf("  cod=%s total=%12.2f precio=%10.2f\n", codProd, total, precio)
	}
	fmt.Printf("  TOTAL cuenta 470100: $%.2f\n", total470100)
	
	// Now check: how does the Excel map these?
	// Product 1605173 (BRIGHT BLUE) in cuenta 470100 = Excel 3000001605173 or 3010001605173?
	// Let's sum ALL March F records by cuenta for this NIT
	fmt.Println("\n=== March F totals by cuenta, NIT 901209350 ===")
	cuentaTotals := map[string]float64{}
	cuentaCounts := map[string]int{}
	for _, r := range recs {
		if len(r) < 500 || r[0] != 'F' { continue }
		nit := strings.TrimLeft(strings.TrimSpace(string(r[16:29])), "0")
		if nit != "901209350" { continue }
		fecha := string(r[42:50])
		if !strings.HasPrefix(fecha, "202603") { continue }
		cuenta := strings.TrimSpace(string(r[29:42]))
		if !strings.Contains(cuenta, "412") { continue }
		total := parsers.DecodePacked(r[145:153], 3)
		cuentaTotals[cuenta] += total
		cuentaCounts[cuenta]++
	}
	grandTotal := 0.0
	for c, t := range cuentaTotals {
		fmt.Printf("  %s: %3d records, $%14.2f\n", c, cuentaCounts[c], t)
		grandTotal += t
	}
	fmt.Printf("  GRAND TOTAL: $%.2f\n", grandTotal)
	fmt.Printf("  Excel total:  $110,384,855\n")
	fmt.Printf("  Diff:         $%.2f\n", 110384855 - grandTotal)
}
