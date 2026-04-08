package main

import (
	"fmt"
	"strings"
	"siigo-common/isam"
)

func main() {
	recs, _, err := isam.ReadFileV2All(`C:\SIIWI02\z06`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	
	// Collect all R records and decode structure precisely
	fmt.Println("=== Decodificando estructura R ===\n")
	count := 0
	for _, r := range recs {
		if r[0] != 'R' { continue }
		count++
		if count > 20 { break }
		
		// Raw key area: bytes 2-28
		key := strings.TrimSpace(string(r[2:28]))
		// Quantity area: bytes 82-96
		qty := strings.TrimSpace(string(r[82:96]))
		
		// Break down key: empresa(3) + prod_grupo(3) + prod_codigo(7) + ing_grupo(3) + ing_codigo(7)
		// = 3+3+7+3+7 = 23 chars... but key is 26 chars
		// Let's try: empresa(3) + grupo_prod(4) + codigo_prod(6) + seq?(2) + grupo_ing(4) + codigo_ing(7)
		
		fmt.Printf("key=%q qty=%q\n", key, qty)
		
		// Also try: byte positions directly
		fmt.Printf("  [2:5]=%q [5:9]=%q [9:15]=%q [15:18]=%q [18:22]=%q [22:28]=%q\n",
			string(r[2:5]), string(r[5:9]), string(r[9:15]),
			string(r[15:18]), string(r[18:22]), string(r[22:28]))
		fmt.Printf("  [2:5]=%q [5:8]=%q [8:15]=%q [15:18]=%q [18:25]=%q\n",
			string(r[2:5]), string(r[5:8]), string(r[8:15]),
			string(r[15:18]), string(r[18:25]))
	}
	
	// Count unique products
	products := map[string]int{}
	totalR := 0
	for _, r := range recs {
		if r[0] != 'R' { continue }
		totalR++
		prod := strings.TrimSpace(string(r[2:15]))
		products[prod]++
	}
	fmt.Printf("\nTotal R: %d, Productos unicos con receta: %d\n", totalR, len(products))
	
	// Show some products with their ingredient count
	fmt.Println("\nProductos con más ingredientes:")
	type pc struct{ k string; c int }
	var top []pc
	for k, v := range products {
		if v >= 5 { top = append(top, pc{k, v}) }
	}
	for _, p := range top {
		fmt.Printf("  %s: %d ingredientes\n", p.k, p.c)
	}
}
