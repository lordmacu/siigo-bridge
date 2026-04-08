package main

import (
	"bytes"
	"fmt"
	"os"
	"siigo-common/isam"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func main() {
	filePath := `C:\SIIWI02\Z042026`
	searchStr := "232375"

	fmt.Printf("Reading ISAM file: %s\n", filePath)
	fmt.Printf("Searching for: %q\n\n", searchStr)

	records, stats, err := isam.ReadFileV2WithStats(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("V2 Stats: RecLen=%d, Records=%d, TotalScanned=%d, Deleted=%d\n",
		stats.Header.MaxRecordLen, len(records), stats.TotalRecords, stats.DeletedCount)
	fmt.Printf("Total records read: %d\n\n", len(records))

	decoder := charmap.Windows1252.NewDecoder()
	searchBytes := []byte(searchStr)
	matchCount := 0

	for i, rec := range records {
		// Search in raw bytes first
		offsets := findAllOccurrences(rec, searchBytes)

		// Also decode to text and search there
		decoded, err := decoder.Bytes(rec)
		if err != nil {
			decoded = rec
		}
		textOffsets := findAllOccurrences(decoded, searchBytes)

		// Merge unique offsets
		allOffsets := mergeOffsets(offsets, textOffsets)

		if len(allOffsets) == 0 {
			continue
		}

		matchCount++
		fmt.Printf("========== MATCH #%d — Record %d (len=%d) ==========\n", matchCount, i, len(rec))

		// Show known fields
		decodedStr := string(decoded)
		showField := func(name string, start, end int) {
			if end > len(decodedStr) {
				end = len(decodedStr)
			}
			if start >= len(decodedStr) {
				fmt.Printf("  %-15s [%d-%d]: (out of range)\n", name, start, end)
				return
			}
			val := strings.TrimRight(decodedStr[start:end], " \x00")
			fmt.Printf("  %-15s [%d-%d]: %q\n", name, start, end, val)
		}

		fmt.Println("--- Known Fields ---")
		showField("empresa", 0, 5)
		showField("grupo", 5, 8)
		showField("codigo", 8, 15)
		showField("nombre", 15, 65)
		showField("nombre_corto", 65, 105)
		showField("referencia", 105, 135)

		// Show match offsets with context
		fmt.Printf("\n--- Match Offsets (%d occurrences) ---\n", len(allOffsets))
		for _, off := range allOffsets {
			fmt.Printf("  Found at byte offset %d\n", off)

			// Context: 20 bytes before and after
			ctxStart := off - 20
			if ctxStart < 0 {
				ctxStart = 0
			}
			ctxEnd := off + len(searchStr) + 20
			if ctxEnd > len(decodedStr) {
				ctxEnd = len(decodedStr)
			}
			context := decodedStr[ctxStart:ctxEnd]
			fmt.Printf("    Context [%d-%d]: %q\n", ctxStart, ctxEnd, toPrintable(context))
		}

		// Full record in 100-byte sections
		fmt.Println("\n--- Full Record (100-byte sections) ---")
		for start := 0; start < len(decodedStr); start += 100 {
			end := start + 100
			if end > len(decodedStr) {
				end = len(decodedStr)
			}
			section := decodedStr[start:end]
			printable := toPrintable(section)

			// Mark if this section contains a match
			marker := " "
			for _, off := range allOffsets {
				if off >= start && off < end {
					marker = "*"
					break
				}
			}
			fmt.Printf("  %s [%4d-%4d]: %s\n", marker, start, end, printable)
		}
		fmt.Println()
	}

	if matchCount == 0 {
		fmt.Println("No records found containing the search string.")
	} else {
		fmt.Printf("\nTotal matches: %d records\n", matchCount)
	}
}

func findAllOccurrences(data []byte, search []byte) []int {
	var offsets []int
	start := 0
	for {
		idx := bytes.Index(data[start:], search)
		if idx < 0 {
			break
		}
		offsets = append(offsets, start+idx)
		start += idx + 1
	}
	return offsets
}

func mergeOffsets(a, b []int) []int {
	seen := make(map[int]bool)
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		seen[v] = true
	}
	var result []int
	for k := range seen {
		result = append(result, k)
	}
	// Sort
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func toPrintable(s string) string {
	var buf strings.Builder
	for _, c := range s {
		if c >= 32 && c < 127 {
			buf.WriteRune(c)
		} else if c >= 160 && c <= 255 {
			buf.WriteRune(c)
		} else {
			buf.WriteRune('.')
		}
	}
	return buf.String()
}
