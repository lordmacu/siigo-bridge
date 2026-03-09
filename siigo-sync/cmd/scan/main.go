package main

import (
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

func main() {
	dir := `C:\DEMOS01`
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading dir: %v\n", err)
		os.Exit(1)
	}

	// Collect ISAM data files (no extension, start with Z)
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(strings.ToUpper(name), "Z") {
			continue
		}
		ext := filepath.Ext(name)
		if ext != "" && strings.ToUpper(ext) != ".IDX" {
			continue
		}
		if ext == "" {
			files = append(files, name)
		}
	}
	sort.Strings(files)

	fmt.Printf("%-20s %8s %8s %6s %s\n", "FILE", "REC_SIZE", "RECORDS", "KEYS", "KEY_SAMPLE")
	fmt.Println(strings.Repeat("-", 90))

	for _, name := range files {
		path := filepath.Join(dir, name)
		scanFile(path, name)
	}
}

func scanFile(path, name string) {
	recs, meta, err := isam.ReadIsamFileWithMeta(path)
	if err != nil {
		fmt.Printf("%-20s %8s %8s %6s %s\n", name, "ERROR", "", "", err.Error())
		return
	}

	keyInfo := ""
	if len(meta.Keys) > 0 && len(meta.Keys[0].Components) > 0 {
		k := meta.Keys[0]
		parts := []string{}
		for _, c := range k.Components {
			parts = append(parts, fmt.Sprintf("off=%d,len=%d", c.Offset, c.Length))
		}
		keyInfo = fmt.Sprintf("PK[%s]", strings.Join(parts, "+"))
	}

	// Show sample of first record
	sample := ""
	if len(recs) > 0 {
		rec := recs[0]
		// Show first 60 printable chars
		var sb strings.Builder
		count := 0
		for _, b := range rec {
			if count >= 60 {
				break
			}
			if b >= 32 && b < 127 {
				sb.WriteByte(b)
			} else {
				sb.WriteByte('.')
			}
			count++
		}
		sample = sb.String()
	}

	src := "BIN"
	if meta.UsedEXTFH {
		src = "EXTFH"
	}

	fmt.Printf("%-20s %8d %8d %6d %-6s %s\n", name, meta.RecSize, len(recs), meta.NumKeys, src, keyInfo)
	if sample != "" {
		fmt.Printf("  -> %s\n", sample)
	}
	_ = recs
}
