package main

import (
	"fmt"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"sort"
	"strings"
)

type fileInfo struct {
	path    string
	name    string
	size    int64
	records int
	recSize int
}

func main() {
	dataPath := `C:\DEMOS01`

	// Files already handled by parsers (base names without year suffix)
	handled := map[string]bool{
		"Z17": true, "Z06": true, "Z06CP": true, "Z49": true,
	}
	// Yearly patterns handled
	handledPatterns := []string{"Z04", "Z09", "Z03", "Z27", "Z11", "Z08", "Z25", "Z28"}

	entries, _ := os.ReadDir(dataPath)
	var files []fileInfo

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

		// Skip handled files
		if handled[name] {
			continue
		}
		skip := false
		for _, pat := range handledPatterns {
			if strings.HasPrefix(name, pat) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		info, _ := e.Info()
		if info == nil || info.Size() < 100 {
			continue
		}

		files = append(files, fileInfo{
			path: filepath.Join(dataPath, name),
			name: name,
			size: info.Size(),
		})
	}

	// Sort by size descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	// Show top 20 and try reading them
	fmt.Printf("Top unhandled ISAM files by size (%d total unhandled):\n\n", len(files))
	limit := 25
	if limit > len(files) {
		limit = len(files)
	}

	for i := 0; i < limit; i++ {
		f := &files[i]
		records, recSize, err := isam.ReadIsamFile(f.path)
		if err != nil {
			fmt.Printf("  %-15s %8d bytes  ERROR: %v\n", f.name, f.size, err)
			continue
		}

		// Count non-empty records
		nonEmpty := 0
		for _, rec := range records {
			for _, b := range rec {
				if b != 0 && b != ' ' {
					nonEmpty++
					break
				}
			}
		}

		// Show sample text from first non-empty record
		sample := ""
		for _, rec := range records {
			empty := true
			for _, b := range rec {
				if b != 0 && b != ' ' {
					empty = false
					break
				}
			}
			if !empty {
				sample = extractSampleText(rec, 80)
				break
			}
		}

		fmt.Printf("  %-15s %8d bytes  recs=%d/%d recSize=%d  | %s\n",
			f.name, f.size, nonEmpty, len(records), recSize, sample)
	}
}

func extractSampleText(rec []byte, maxLen int) string {
	var parts []string
	inText := false
	start := 0

	for i, b := range rec {
		isText := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == ' ' || b == '.' || b == '-' || b == '/' || (b >= '0' && b <= '9') || (b >= 0xC0)
		if !inText && ((b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')) {
			start = i
			inText = true
		}
		if inText && !isText {
			if i-start > 3 {
				text := strings.TrimSpace(string(rec[start:i]))
				if len(text) > 3 {
					parts = append(parts, text)
				}
			}
			inText = false
		}
	}
	if inText && len(rec)-start > 3 {
		text := strings.TrimSpace(string(rec[start:]))
		if len(text) > 3 {
			parts = append(parts, text)
		}
	}

	result := strings.Join(parts, " | ")
	if len(result) > maxLen {
		result = result[:maxLen] + "..."
	}
	return result
}
