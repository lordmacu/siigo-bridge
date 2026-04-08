//go:build ignore

package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	// Z15 = Consecutivos de comprobantes en Siigo
	recs, err := isam.ReadFileV2("C:\SIIWI02\Z152026")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("Z15 2026: %d records, reclen=%d\n", len(recs), len(recs[0]))
	
	found := 0
	for _, r := range recs {
		s := string(r)
		if strings.Contains(s, "16810") {
			found++
			// Show first 50 chars as hex + ascii
			show := r
			if len(show) > 80 { show = show[:80] }
			fmt.Printf("  Key(0:15)=%q  raw(0:50)=%q\n", string(r[:15]), string(show[:50]))
			if found >= 10 { break }
		}
	}
	fmt.Printf("Found %d records with 16810\n", found)
}
