package main

import (
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

var decoder = charmap.Windows1252.NewDecoder()

// Already parsed files (skip these)
var handled = map[string]bool{
	"Z17": true, "Z06": true, "Z06CP": true, "Z49": true,
	"ZDANE": true, "ZICA": true, "ZPILA": true, "Z07T": true, "Z07S": true,
}
var handledPrefixes = []string{
	"Z04", "Z09", "Z03", "Z27", "Z11", "Z08", "Z25", "Z28",
	"Z18", "Z07", "Z26", "Z05",
}

type scanResult struct {
	name       string
	size       int64
	records    int
	recSize    uint32
	deleted    int
	idxFormat  byte
	org        byte
	isIndexed  bool
	longRecs   bool
	recMode    byte
	created    string
	modified   string
	sample     string
	dataTypes  map[byte]int
	err        string
	nonEmpty   int
	hasASCII   bool
	asciiRatio float64
}

func main() {
	dataPath := `C:\DEMOS01`
	showAll := false
	showHandled := false

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--all", "-a":
			showAll = true
		case "--handled", "-h":
			showHandled = true
		}
	}

	entries, err := os.ReadDir(dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read %s: %v\n", dataPath, err)
		os.Exit(1)
	}

	var results []scanResult

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "Z") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".idx") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".gnt") {
			continue
		}

		// Skip handled unless --handled flag
		if !showHandled {
			if handled[name] {
				continue
			}
			skip := false
			for _, pat := range handledPrefixes {
				if strings.HasPrefix(name, pat) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}

		info, _ := e.Info()
		if info == nil || info.Size() < 130 {
			continue
		}

		r := scanResult{
			name: name,
			size: info.Size(),
		}

		fullPath := filepath.Join(dataPath, name)
		records, stats, readErr := isam.ReadFileV2WithStats(fullPath)
		if readErr != nil {
			r.err = readErr.Error()
			results = append(results, r)
			continue
		}

		r.records = len(records)
		r.recSize = stats.Header.MaxRecordLen
		r.deleted = stats.DeletedCount
		r.idxFormat = stats.Header.IdxFormat
		r.org = stats.Header.Organization
		r.isIndexed = stats.Header.IsIndexed
		r.longRecs = stats.Header.LongRecords
		r.recMode = stats.Header.RecordMode
		r.created = stats.Header.CreationDate
		r.modified = stats.Header.ModifiedDate
		r.dataTypes = stats.DataTypes

		// Analyze content quality
		totalASCII := 0
		totalBytes := 0
		for _, rec := range records {
			empty := true
			for _, b := range rec {
				if b != 0 && b != ' ' {
					empty = false
					break
				}
			}
			if !empty {
				r.nonEmpty++
			}
			// Count ASCII-readable bytes
			for _, b := range rec {
				totalBytes++
				if (b >= 0x20 && b <= 0x7E) || (b >= 0xC0 && b <= 0xFF) {
					totalASCII++
				}
			}
		}
		if totalBytes > 0 {
			r.asciiRatio = float64(totalASCII) / float64(totalBytes)
			r.hasASCII = r.asciiRatio > 0.3
		}

		// Extract sample text from first non-empty record
		for _, rec := range records {
			empty := true
			for _, b := range rec {
				if b != 0 && b != ' ' {
					empty = false
					break
				}
			}
			if !empty {
				r.sample = extractReadableText(rec, 120)
				break
			}
		}

		results = append(results, r)
	}

	// Sort by records descending (most data first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].records > results[j].records
	})

	// Summary
	totalFiles := len(results)
	withData := 0
	withErrors := 0
	for _, r := range results {
		if r.err != "" {
			withErrors++
		} else if r.records > 0 {
			withData++
		}
	}

	fmt.Printf("=== ISAM V2 Scanner ===\n")
	fmt.Printf("Files scanned: %d | With data: %d | Errors: %d | Empty: %d\n\n",
		totalFiles, withData, withErrors, totalFiles-withData-withErrors)

	// Show files with data
	fmt.Printf("--- FILES WITH DATA (sorted by record count) ---\n\n")
	fmt.Printf("%-18s %8s %6s %6s %4s %3s %5s %6s  %s\n",
		"FILE", "SIZE", "RECS", "ALIVE", "DEL", "FMT", "ASCII", "RECSZ", "SAMPLE")
	fmt.Println(strings.Repeat("-", 140))

	for _, r := range results {
		if r.err != "" || r.records == 0 {
			if !showAll {
				continue
			}
		}

		if r.err != "" {
			fmt.Printf("%-18s %8d %s\n", r.name, r.size, "ERROR: "+truncate(r.err, 80))
			continue
		}

		if r.records == 0 {
			fmt.Printf("%-18s %8d  (empty)\n", r.name, r.size)
			continue
		}

		asciiPct := fmt.Sprintf("%3.0f%%", r.asciiRatio*100)
		fmt.Printf("%-18s %8d %6d %6d %4d %3d %5s %6d  %s\n",
			r.name, r.size, r.records, r.nonEmpty, r.deleted, r.idxFormat,
			asciiPct, r.recSize, truncate(r.sample, 70))
	}

	// Show errors separately
	if withErrors > 0 {
		fmt.Printf("\n--- ERRORS (%d files) ---\n", withErrors)
		for _, r := range results {
			if r.err != "" {
				fmt.Printf("  %-18s %8d bytes  %s\n", r.name, r.size, truncate(r.err, 90))
			}
		}
	}

	// Interesting files analysis
	fmt.Printf("\n--- POTENTIALLY INTERESTING (>5 records, >30%% ASCII) ---\n\n")
	interesting := 0
	for _, r := range results {
		if r.err == "" && r.nonEmpty > 5 && r.asciiRatio > 0.3 {
			interesting++
			indexed := ""
			if r.isIndexed {
				indexed = " [IDX]"
			}
			fmt.Printf("  %-18s %6d recs  recSize=%-6d  ascii=%3.0f%%  modified=%s%s\n",
				r.name, r.nonEmpty, r.recSize, r.asciiRatio*100, r.modified, indexed)
			if r.sample != "" {
				fmt.Printf("    → %s\n", truncate(r.sample, 110))
			}
		}
	}
	if interesting == 0 {
		fmt.Println("  (none)")
	}
}

func extractReadableText(rec []byte, maxLen int) string {
	// Decode Windows-1252
	decoded, err := decoder.Bytes(rec)
	if err != nil {
		decoded = rec
	}

	var parts []string
	var current []byte

	for _, b := range decoded {
		isText := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == ' ' ||
			b == '.' || b == '-' || b == '/' || b == '@' ||
			(b >= '0' && b <= '9') || (b >= 0xC0)
		if isText {
			current = append(current, b)
		} else {
			if len(current) > 3 {
				text := strings.TrimSpace(string(current))
				if len(text) > 2 {
					parts = append(parts, text)
				}
			}
			current = nil
		}
	}
	if len(current) > 3 {
		text := strings.TrimSpace(string(current))
		if len(text) > 2 {
			parts = append(parts, text)
		}
	}

	result := strings.Join(parts, " | ")
	if len(result) > maxLen {
		result = result[:maxLen] + "..."
	}
	return result
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
