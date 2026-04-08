//go:build ignore

package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	recs, _, _ := isam.ReadFileV2All(`C:\SIIWI02\Z042026`)
	
	// Z04 code structure: EEE + GGGG + PPPPPPP (empresa 3 + grupo 4 + producto 7 = 14 chars in key)
	// But the key is 15 chars. Let's check:
	// "003000002595829" = emp=003, grupo=00000, prod=2595829
	// "003010002595829" = emp=003, grupo=01000, prod=2595829
	
	// Build map: cuenta suffix → Z04 grupo prefix
	// cuenta 4120470100 → grupo 01000? Let's verify
	
	// Find products that have both 00000 and 01000 grupo
	products := map[string][]string{} // product7 → list of grupo4
	for _, r := range recs {
		if len(r) < 15 { continue }
		emp := string(r[0:3])
		if emp != "003" { continue } // only empresa 003
		grupo := string(r[3:6])
		prod := strings.TrimSpace(string(r[6:13]))
		if prod == "" { continue }
		
		key := prod
		gp := grupo
		found := false
		for _, g := range products[key] {
			if g == gp { found = true; break }
		}
		if !found {
			products[key] = append(products[key], gp)
		}
	}
	
	// Show products with multiple grupos
	fmt.Println("=== Products with multiple grupos ===")
	count := 0
	for prod, grupos := range products {
		if len(grupos) > 1 {
			count++
			if count <= 20 {
				fmt.Printf("  prod=%s grupos=%v\n", prod, grupos)
			}
		}
	}
	fmt.Printf("Total products with multiple grupos: %d\n", count)
	
	// Now the key question: what's the mapping between cuenta and grupo?
	// cuenta 4120470100 = "Perfumeria Fina" = grupo 010?
	// cuenta 4120470200 = "Cuidado Personal" = grupo 000?
	// cuenta 4120470300 = "Aseo Hogar" = grupo 000?
	
	// Let's check by looking at the cuenta suffix pattern
	// cuenta: 000 412 0470100
	//                  ^^^^^ = 47010 → sub-cuenta
	// The "0" at position [3] of grupo might map to the sub-cuenta
	
	// Check: for product 2595829 (TINNY LOVE)
	// Z04 has: grupo 000 (code 3000002595829) and grupo 010 (code 3010002595829)
	// Z09 has: cuenta 4120470200 only
	// Excel has: 3000002595829 in one line, 3010002595829 in another
	// So grupo 000 maps to cuenta 470200, and grupo 010 maps to... 470100?
	
	// Let's check what Z04 key looks like for a product we KNOW is in cuenta 470100
	// Product 1605173 (BRIGHT BLUE) is in cuenta 470100
	fmt.Println("\n=== Z04 entries for 1605173 (BRIGHT BLUE, cuenta 470100) ===")
	for _, r := range recs {
		if len(r) < 15 { continue }
		if string(r[0:3]) != "003" { continue }
		s := string(r)
		if strings.Contains(s, "605173") {
			fmt.Printf("  key=%s\n", strings.TrimSpace(string(r[0:15])))
		}
	}
	
	// And for 2595829 (cuenta 470200)
	fmt.Println("\n=== Z04 entries for 2595829 (TINNY LOVE, cuenta 470200) ===")
	for _, r := range recs {
		if len(r) < 15 { continue }
		if string(r[0:3]) != "003" { continue }
		s := string(r)
		if strings.Contains(s, "595829") {
			fmt.Printf("  key=%s\n", strings.TrimSpace(string(r[0:15])))
		}
	}
	
	// For 3008092 (ACAI, cuenta 470300)
	fmt.Println("\n=== Z04 entries for 3008092 (ACAI, cuenta 470300) ===")
	for _, r := range recs {
		if len(r) < 15 { continue }
		if string(r[0:3]) != "003" { continue }
		s := string(r)
		if strings.Contains(s, "008092") {
			fmt.Printf("  key=%s\n", strings.TrimSpace(string(r[0:15])))
		}
	}
	
	// The Excel product code is: GG LLLL PPPPPPP (13 digits)
	// GG = first digit of producto (1, 2, 3, 6)
	// LLLL = grupo from Z04 (00000, 01000)
	// So Excel code = producto[0:1] + grupo + "00" + producto
	// Actually looking at the Excel:
	// 3000002595829 → first digit "3" doesn't match product "2595829"
	// The "3" must come from somewhere else
	
	// Let's check: in Z04, key "003000002595829"
	// emp=003, rest=000002595829
	// Excel code: 3000002595829 (without emp prefix "00")
	// So Excel code = key[2:15]? → "3000002595829" ✓!
	
	fmt.Println("\n=== VERIFY: Z04 key[2:15] = Excel product code ===")
	for _, r := range recs {
		if len(r) < 15 { continue }
		if string(r[0:3]) != "003" { continue }
		s := string(r)
		if strings.Contains(s, "595829") || strings.Contains(s, "008092") || strings.Contains(s, "605173") {
			key := string(r[0:15])
			excelCode := key[2:]
			fmt.Printf("  Z04 key=%s → Excel code=%s\n", key, excelCode)
		}
	}
}
