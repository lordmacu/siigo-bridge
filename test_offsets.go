package main

import (
	"fmt"
	"siigo-common/isam"
)

func main() {
	recs, _, err := isam.ReadFileV2All(`C:\SIIWI02\Z042012`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for i := 0; i < 8 && i < len(recs); i++ {
		r := recs[i]
		fmt.Printf("\n=== Record %d ===\n", i)
		// Hex dump bytes 0-70
		for j := 0; j < 70 && j < len(r); j++ {
			fmt.Printf("%02X ", r[j])
			if (j+1)%16 == 0 { fmt.Println() }
		}
		fmt.Println()
		// Show printable 0-120
		show := 120
		if len(r) < show { show = len(r) }
		clean := make([]byte, show)
		for j := 0; j < show; j++ {
			if r[j] >= 32 && r[j] < 127 { clean[j] = r[j] } else { clean[j] = '.' }
		}
		fmt.Printf("raw: %s\n", string(clean))
		// Try different offset combos
		fmt.Printf("  empresa[0:5]=%q\n", string(r[0:5]))
		fmt.Printf("  grupo[5:8]=%q   grupo[5:9]=%q\n", string(r[5:8]), string(r[5:9]))
		fmt.Printf("  codigo[8:14]=%q codigo[8:15]=%q codigo[9:15]=%q\n", 
			string(r[8:14]), string(r[8:15]), string(r[9:15]))
		fmt.Printf("  nombre[14:64]=%q\n", string(r[14:64]))
		fmt.Printf("  nombre[15:65]=%q\n", string(r[15:65]))
		fmt.Printf("  nombre[15:55]=%q\n", string(r[15:55]))
		fmt.Printf("  nombre_corto[65:105]=%q\n", string(r[65:105]))
		fmt.Printf("  nombre_corto[55:95]=%q\n", string(r[55:95]))
	}
}
