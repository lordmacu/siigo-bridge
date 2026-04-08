package main

import (
	"bytes"
	"fmt"
	"siigo-common/isam"

	"golang.org/x/text/encoding/charmap"
)

var decoder = charmap.Windows1252.NewDecoder()

func toPrintable(data []byte) string {
	decoded, err := decoder.Bytes(data)
	if err != nil {
		decoded = data
	}
	out := make([]byte, len(decoded))
	for i, b := range decoded {
		if b >= 32 && b <= 126 {
			out[i] = b
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func main() {
	dataDir := `C:\SIIWI02`

	// Key question: Z09 has product refs like "1320118", "750115", "589453"
	// Z04 has product codes like "1003294", "1110919"
	// Are they related? Let's check if Z04 codes appear as substrings in Z09 product refs

	// Get all Z04 product codes (the 6-digit part after group prefix)
	fmt.Println("=== Z04 product code format analysis ===")
	z04path := dataDir + `\Z042026`
	z04recs, _, _ := isam.ReadFileV2WithStats(z04path)
	fmt.Printf("Z04: %d records\n", len(z04recs))

	// Show first 20 Z04 records to understand code format
	for i := 0; i < 20 && i < len(z04recs); i++ {
		printable := toPrintable(z04recs[i])
		if len(printable) > 100 {
			printable = printable[:100]
		}
		fmt.Printf("  Z04[%d]: %s\n", i, printable)
	}

	// Now look at Z09 SIMONIZ order records - extract the product ref field
	fmt.Println("\n=== Z09 SIMONIZ records - product reference extraction ===")
	z09path := dataDir + `\Z092026`
	z09recs, _, _ := isam.ReadFileV2WithStats(z09path)
	searchNIT := []byte("800203984")

	// Look only at "O002" type records (orders/remisiones) which have products
	fmt.Println("Order records (O002) for SIMONIZ with product details:")
	for _, rec := range z09recs {
		if !bytes.Contains(rec, searchNIT) {
			continue
		}
		printable := toPrintable(rec)
		if len(printable) < 10 {
			continue
		}
		// Only "O002" type (remisiones with product line items)
		if len(rec) > 4 && string(rec[0:4]) == "O002" {
			// Show the full first 250 chars with byte positions
			display := printable
			if len(display) > 250 {
				display = display[:250]
			}
			fmt.Printf("\n  %s\n", display)
			// Show hex positions 80-120 where product code might be
			if len(rec) > 120 {
				fmt.Printf("  Bytes 80-130: ")
				for j := 80; j < 130 && j < len(rec); j++ {
					fmt.Printf("%02x", rec[j])
				}
				fmt.Printf("\n  Text  80-130: %s\n", toPrintable(rec[80:130]))
			}
		}
	}

	// Now search Z04 for the specific product codes found in Z09 SIMONIZ orders
	fmt.Println("\n\n=== Cross-reference: Z09 product refs vs Z04 catalog ===")
	// Product refs from Z09 SIMONIZ orders:
	productRefs := []struct {
		code string
		name string
	}{
		{"1320118", "SONATA B"},
		{"4000003", "NONIL FENOL"},
		{"4100000", "CRISTALINE"},
		{"1750115", "LEMONFRESH SS1B 1262"},
		{"1750139", "FASSORANGE SS2 1559"},
		{"1750310", "MANDARINE SS5 1566"},
		{"3589453", "TORONJA PLUS"},
		{"0320118", "SONATA B (without prefix?)"},
	}

	for _, pr := range productRefs {
		codeBytes := []byte(pr.code)
		found := 0
		for _, rec := range z04recs {
			if bytes.Contains(rec, codeBytes) {
				found++
				if found == 1 {
					printable := toPrintable(rec)
					if len(printable) > 120 {
						printable = printable[:120]
					}
					fmt.Printf("\n  Z09 ref '%s' (%s) -> Z04 match: %s", pr.code, pr.name, printable)
				}
			}
		}
		if found == 0 {
			// Try without leading digit (Z04 might use different prefix)
			shortCode := pr.code[1:]
			shortBytes := []byte(shortCode)
			for _, rec := range z04recs {
				if bytes.Contains(rec, shortBytes) {
					found++
					if found == 1 {
						printable := toPrintable(rec)
						if len(printable) > 120 {
							printable = printable[:120]
						}
						fmt.Printf("\n  Z09 ref '%s' (%s) -> Z04 partial match '%s': %s", pr.code, pr.name, shortCode, printable)
					}
				}
			}
			if found == 0 {
				fmt.Printf("\n  Z09 ref '%s' (%s) -> NOT FOUND in Z04", pr.code, pr.name)
			}
		}
		if found > 1 {
			fmt.Printf(" (+%d more)", found-1)
		}
	}

	// Also check: do Z04 codes appear in Z09?
	fmt.Println("\n\n\n=== Z06 (formulas) - how do formula product codes relate? ===")
	z06path := dataDir + `\Z06`
	z06recs, _, _ := isam.ReadFileV2WithStats(z06path)
	fmt.Printf("Z06: %d records\n", len(z06recs))
	// Show formula records that reference "320118" (SONATA B from Z09)
	searchRef := []byte("320118")
	for i, rec := range z06recs {
		if bytes.Contains(rec, searchRef) {
			printable := toPrintable(rec)
			if len(printable) > 200 {
				printable = printable[:200]
			}
			fmt.Printf("  Z06[%d]: %s\n", i, printable)
		}
	}

	// Check IFIS.DIS which also had product codes
	fmt.Println("\n=== IFIS.DIS - product inventory file ===")
	ifisPath := dataDir + `\IFIS.DIS`
	ifisRecs, stats, err := isam.ReadFileV2WithStats(ifisPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("IFIS.DIS: %d records, recLen=%d\n", len(ifisRecs), stats.Header.MaxRecordLen)
		// Show first 5
		for i := 0; i < 5 && i < len(ifisRecs); i++ {
			printable := toPrintable(ifisRecs[i])
			if len(printable) > 200 {
				printable = printable[:200]
			}
			fmt.Printf("  [%d]: %s\n", i, printable)
		}
		// Search for SIMONIZ NIT
		nitCount := 0
		for _, rec := range ifisRecs {
			if bytes.Contains(rec, searchNIT) {
				nitCount++
			}
		}
		fmt.Printf("  SIMONIZ NIT occurrences: %d\n", nitCount)

		// Search for SONATA B product ref
		for i, rec := range ifisRecs {
			if bytes.Contains(rec, searchRef) {
				printable := toPrintable(rec)
				if len(printable) > 200 {
					printable = printable[:200]
				}
				fmt.Printf("  IFIS[%d] with '320118': %s\n", i, printable)
				if i > 10 {
					break
				}
			}
		}
	}

	// Final: check Z04IN which is an inventory cross-ref
	fmt.Println("\n=== Z04IN - product inventory cross-reference ===")
	z04inPath := dataDir + `\Z04IN`
	z04inRecs, stats, err := isam.ReadFileV2WithStats(z04inPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Z04IN: %d records, recLen=%d\n", len(z04inRecs), stats.Header.MaxRecordLen)
		for i := 0; i < 10 && i < len(z04inRecs); i++ {
			printable := toPrintable(z04inRecs[i])
			if len(printable) > 200 {
				printable = printable[:200]
			}
			fmt.Printf("  [%d]: %s\n", i, printable)
		}
		// Search for SONATA B ref
		for i, rec := range z04inRecs {
			if bytes.Contains(rec, searchRef) {
				printable := toPrintable(rec)
				if len(printable) > 200 {
					printable = printable[:200]
				}
				fmt.Printf("  Z04IN[%d] with '320118': %s\n", i, printable)
			}
		}
	}
}
