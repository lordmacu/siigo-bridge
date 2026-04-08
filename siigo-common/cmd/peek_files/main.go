package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	path := `C:\SIIWI02\Z042016`
	info, err := isam.ReadFile(path)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Z042016: recSize=%d, records=%d\n\n", info.RecordSize, len(info.Records))

	for i, rec := range info.Records {
		if i >= 20 {
			break
		}
		d := rec.Data
		empresa := strings.TrimSpace(isam.ExtractField(d, 0, 5))
		grupo := strings.TrimSpace(isam.ExtractField(d, 5, 3))
		codigo := strings.TrimSpace(isam.ExtractField(d, 8, 6))
		nombre := strings.TrimSpace(isam.ExtractField(d, 14, 50))
		nombreCorto := strings.TrimSpace(isam.ExtractField(d, 64, 40))
		// Look for more fields
		ref := strings.TrimSpace(isam.ExtractField(d, 104, 30))

		fmt.Printf("[%3d] emp=%s grp=%s cod=%s nombre=%-45s corto=%-30s ref=%s\n",
			i, empresa, grupo, codigo, nombre, nombreCorto, ref)
	}
}
