package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"isam-admin/pkg/isam"

	"golang.org/x/text/encoding/charmap"
)

func printableStr(data []byte, start, length int) string {
	if start >= len(data) {
		return ""
	}
	end := start + length
	if end > len(data) {
		end = len(data)
	}
	decoded, _ := charmap.Windows1252.NewDecoder().Bytes(data[start:end])
	return strings.TrimSpace(string(decoded))
}

func main() {
	dir := `C:\Archivos Siigo`
	files := []string{"z06", "Z06A", "Z0620181008"}

	for _, name := range files {
		fullPath := filepath.Join(dir, name)
		fi, _, err := isam.ReadIsamFile(fullPath)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", name, err)
			continue
		}

		fmt.Printf("\n========== %s (%d registros, recsize=%d) ==========\n", name, len(fi.Records), fi.RecordSize)

		// Show first 30 records with type analysis
		limit := 30
		if len(fi.Records) < limit {
			limit = len(fi.Records)
		}

		// Collect unique "types" (first byte pattern)
		types := map[string]int{}
		for _, rec := range fi.Records {
			raw := rec.Data
			if len(raw) > 0 {
				t := printableStr(raw, 0, 1)
				types[t]++
			}
		}
		fmt.Println("Tipos encontrados (primer byte):")
		for t, count := range types {
			fmt.Printf("  '%s' = %d registros\n", t, count)
		}

		fmt.Println("\nMuestra de registros:")
		// Show distributed samples
		step := len(fi.Records) / 20
		if step < 1 {
			step = 1
		}
		shown := 0
		for i := 0; i < len(fi.Records) && shown < 25; i += step {
			raw := fi.Records[i].Data
			decoded, _ := charmap.Windows1252.NewDecoder().Bytes(raw)
			// Show key fields at known Z06 offsets
			tipo := printableStr(raw, 0, 1)
			codigo := printableStr(raw, 2, 7)
			nombre := printableStr(raw, 31, 40)

			// Also show raw printable for context
			display := make([]byte, 120)
			for j := 0; j < 120 && j < len(decoded); j++ {
				if decoded[j] >= 32 && decoded[j] < 127 {
					display[j] = decoded[j]
				} else {
					display[j] = '.'
				}
			}

			fmt.Printf("  [%4d] tipo='%s' cod='%-8s' nombre='%-40s'  raw: %s\n",
				i, tipo, codigo, nombre, string(display))
			shown++
		}

		// Also try to find distinct "groups" by analyzing tipo field
		fmt.Println("\nAgrupacion por tipo+codigo (primeros 5 de cada tipo):")
		typeExamples := map[string][]string{}
		for _, rec := range fi.Records {
			raw := rec.Data
			tipo := printableStr(raw, 0, 1)
			codigo := printableStr(raw, 2, 7)
			nombre := printableStr(raw, 31, 40)
			key := tipo
			if len(typeExamples[key]) < 5 {
				typeExamples[key] = append(typeExamples[key], fmt.Sprintf("cod=%s nombre=%s", codigo, nombre))
			}
		}
		for t, examples := range typeExamples {
			fmt.Printf("\n  Tipo '%s':\n", t)
			for _, ex := range examples {
				fmt.Printf("    %s\n", ex)
			}
		}
	}

	// Also check if Z06 data files exist without year suffix
	fmt.Println("\n\n========== Archivos Z06* en el directorio ==========")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(strings.ToUpper(e.Name()), "Z06") || strings.ToLower(e.Name()) == "z06" {
			info, _ := e.Info()
			fmt.Printf("  %s  (%d bytes)\n", e.Name(), info.Size())
		}
	}
}
