package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	files := []struct {
		path  string
		label string
		max   int
		bytes int
	}{
		{`C:\DEMOS01\Z07T`, "Z07T (transacciones detalle)", 15, 256},
		{`C:\DEMOS01\Z07S`, "Z07S (resumen)", 15, 256},
		{`C:\DEMOS01\Z12201`, "Z12201 (mapeos)", 10, 512},
		{`C:\DEMOS01\Z052014`, "Z05 (creditos/pagos)", 10, 250},
		{`C:\DEMOS01\Z262016`, "Z26 (periodos)", 10, 200},
		{`C:\DEMOS01\Z122`, "Z122 (mapeos v2)", 10, 512},
	}

	for _, f := range files {
		dumpFile(f.path, f.label, f.max, f.bytes)
	}
}

func dumpFile(path, label string, maxRecs, maxBytes int) {
	records, recSize, err := isam.ReadIsamFile(path)
	if err != nil {
		fmt.Printf("\n=== %s === ERROR: %v\n", label, err)
		return
	}

	// Count non-empty
	nonEmpty := 0
	for _, rec := range records {
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if !empty {
			nonEmpty++
		}
	}
	fmt.Printf("\n{'='*60}\n")
	fmt.Printf("=== %s === total=%d nonEmpty=%d recSize=%d\n", label, len(records), nonEmpty, recSize)
	fmt.Printf("{'='*60}\n")

	// Show distributed records (first, middle, last)
	shown := 0
	indices := distributedIndices(len(records), maxRecs)
	for _, idx := range indices {
		if shown >= maxRecs {
			break
		}
		rec := records[idx]
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		shown++

		fmt.Printf("\n  Record %d/%d (len=%d):\n", idx, len(records), len(rec))
		limit := maxBytes
		if limit > len(rec) {
			limit = len(rec)
		}
		for off := 0; off < limit; off += 16 {
			end := off + 16
			if end > limit {
				end = limit
			}
			allEmpty := true
			for j := off; j < end; j++ {
				if rec[j] != 0 && rec[j] != ' ' && rec[j] != 0xFF {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				continue
			}
			fmt.Printf("    %04X: ", off)
			for j := off; j < end; j++ {
				fmt.Printf("%02X ", rec[j])
			}
			for j := end; j < off+16; j++ {
				fmt.Print("   ")
			}
			fmt.Print("| ")
			for j := off; j < end; j++ {
				if rec[j] >= 32 && rec[j] < 127 {
					fmt.Printf("%c", rec[j])
				} else {
					fmt.Print(".")
				}
			}
			fmt.Println()
		}

		findText(rec, limit)

		// Scan for dates
		for off := 0; off < len(rec)-8; off++ {
			s := string(rec[off : off+8])
			if looksLikeDate(s) {
				fmt.Printf("    DATE@%d: %s\n", off, s)
			}
		}
	}

	// Field analysis: find common patterns across all non-empty records
	fmt.Printf("\n  --- Field Analysis (first byte distribution) ---\n")
	firstChars := map[byte]int{}
	for _, rec := range records {
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if !empty && len(rec) > 0 {
			firstChars[rec[0]]++
		}
	}
	for b, count := range firstChars {
		if b >= 32 && b < 127 {
			fmt.Printf("    0x%02X '%c': %d records\n", b, b, count)
		} else {
			fmt.Printf("    0x%02X: %d records\n", b, count)
		}
	}

	// Find which byte positions have consistent non-zero content
	if nonEmpty > 0 {
		fmt.Printf("\n  --- Byte positions with >50%% non-zero/space content ---\n")
		maxLen := 0
		for _, rec := range records {
			if len(rec) > maxLen {
				maxLen = len(rec)
			}
		}
		checkLimit := maxBytes
		if checkLimit > maxLen {
			checkLimit = maxLen
		}
		for pos := 0; pos < checkLimit; pos++ {
			hasData := 0
			total := 0
			for _, rec := range records {
				if len(rec) <= pos {
					continue
				}
				empty := true
				for _, b := range rec {
					if b != 0 && b != ' ' {
						empty = false
						break
					}
				}
				if empty {
					continue
				}
				total++
				if rec[pos] != 0 && rec[pos] != ' ' {
					hasData++
				}
			}
			if total > 0 && hasData*100/total > 50 {
				// Sample values at this position
				samples := []string{}
				for _, rec := range records {
					if len(rec) <= pos {
						continue
					}
					e := true
					for _, b := range rec {
						if b != 0 && b != ' ' {
							e = false
							break
						}
					}
					if e {
						continue
					}
					if rec[pos] != 0 && rec[pos] != ' ' {
						if rec[pos] >= 32 && rec[pos] < 127 {
							samples = append(samples, fmt.Sprintf("%c", rec[pos]))
						} else {
							samples = append(samples, fmt.Sprintf("0x%02X", rec[pos]))
						}
					}
					if len(samples) >= 5 {
						break
					}
				}
				pct := hasData * 100 / total
				fmt.Printf("    pos %3d: %d%% filled  samples: %s\n", pos, pct, strings.Join(samples, ","))
			}
		}
	}
}

func distributedIndices(total, want int) []int {
	if total <= want {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	indices := []int{}
	// First few
	for i := 0; i < want/3 && i < total; i++ {
		indices = append(indices, i)
	}
	// Middle
	mid := total / 2
	for i := mid - want/6; i <= mid+want/6 && i < total; i++ {
		if i >= 0 {
			indices = append(indices, i)
		}
	}
	// Last few
	for i := total - want/3; i < total; i++ {
		if i >= 0 {
			indices = append(indices, i)
		}
	}
	// Deduplicate
	seen := map[int]bool{}
	result := []int{}
	for _, i := range indices {
		if !seen[i] {
			seen[i] = true
			result = append(result, i)
		}
	}
	return result
}

func findText(rec []byte, limit int) {
	if limit > len(rec) {
		limit = len(rec)
	}
	inText := false
	start := 0
	for i := 0; i < limit; i++ {
		b := rec[i]
		isAlpha := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
		isT := isAlpha || b == ' ' || b == '.' || b == '-' || b == '/' || b == ',' || b == '#' || (b >= '0' && b <= '9') || (b >= 0xC0)
		if !inText && isAlpha {
			start = i
			inText = true
		}
		if inText && !isT {
			if i-start > 5 {
				fmt.Printf("    TEXT@%d-%d: %q\n", start, i-1, strings.TrimSpace(string(rec[start:i])))
			}
			inText = false
		}
	}
	if inText && limit-start > 5 {
		fmt.Printf("    TEXT@%d-%d: %q\n", start, limit-1, strings.TrimSpace(string(rec[start:limit])))
	}
}

func looksLikeDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	y := s[:4]
	m := (s[4]-'0')*10 + s[5] - '0'
	d := (s[6]-'0')*10 + s[7] - '0'
	return y >= "1990" && y <= "2030" && m >= 1 && m <= 12 && d >= 1 && d <= 31
}
