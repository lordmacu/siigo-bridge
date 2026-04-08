package main

import (
	"fmt"
	"siigo-common/isam"
)

func main() {
	recs, _, err := isam.ReadFileV2All(`C:\SIIWI02\z06`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	
	// Detailed analysis of R records structure
	fmt.Println("=== Estructura tipo R (recetas) ===\n")
	count := 0
	lastProduct := ""
	for _, r := range recs {
		if r[0] != 'R' { continue }
		count++
		if count > 30 { break }
		
		// Try to identify fields by examining the structure
		// "R 001 0004 0759770 010 0004 288144"
		// Let's look byte by byte
		// Bytes: R(0) space(1) empresa(2-4) grupo_prod(5-7) codigo_prod(8-14) grupo_ing(15-17) codigo_ing(18-24)
		
		empresa := string(r[2:5])
		prodCode := string(r[5:12])
		ingCode := string(r[12:26])
		
		// Percentage/quantity after the "R" label area
		cantArea := string(r[68:82])
		
		// Show hex of first 30 bytes
		hex := ""
		for j := 0; j < 30; j++ {
			hex += fmt.Sprintf("%02X ", r[j])
		}
		
		prod := fmt.Sprintf("%s/%s", empresa, prodCode)
		if prod != lastProduct {
			fmt.Printf("\n--- Producto: %s ---\n", prod)
			lastProduct = prod
		}
		fmt.Printf("  ingrediente=%s cant=%s  hex: %s\n", ingCode, cantArea, hex)
	}
	
	// Better: hex dump a few records side by side
	fmt.Println("\n\n=== Hex dump primeros 5 tipo R ===")
	count = 0
	for _, r := range recs {
		if r[0] != 'R' { continue }
		count++
		if count > 5 { break }
		
		fmt.Printf("\nRecord tipo R #%d:\n", count)
		for offset := 0; offset < 100; offset += 30 {
			end := offset + 30
			if end > 100 { end = 100 }
			
			// Hex
			hex := ""
			asc := ""
			for j := offset; j < end; j++ {
				hex += fmt.Sprintf("%02X ", r[j])
				if r[j] >= 32 && r[j] < 127 { asc += string(r[j]) } else { asc += "." }
			}
			fmt.Printf("  [%3d] %-90s %s\n", offset, hex, asc)
		}
	}
}
