package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"siigo-common/isam"
)

func main() {
	dataPath := `C:\SIIWI02\`

	// All 19 ISAM files we parse
	files := []struct {
		name string
		desc string
	}{
		{"Z17", "Terceros"},
		{"Z06", "Maestros"},
		{"Z49", "Movimientos"},
		{"Z032016", "Plan Cuentas"},
		{"Z042016", "Inventario"},
		{"Z092016", "Cartera"},
		{"Z112016", "Documentos"},
		{"Z182016", "Historial"},
		{"Z252016", "Saldos Terceros"},
		{"Z262016", "Periodos"},
		{"Z272016", "Activos Fijos"},
		{"Z282016", "Saldos Consol."},
		{"Z052016", "Cond. Pago"},
		{"Z072016", "Libros Auxiliares"},
		{"Z07T", "Trans. Detalle"},
		{"Z082016A", "Terceros Amp."},
		{"ZDANE", "DANE Municipios"},
		{"ZICA", "ICA"},
		{"ZPILA", "PILA"},
	}

	fmt.Println("====================================================================")
	fmt.Println("  ISAM READER 3-WAY COMPARISON: EXTFH (gold) vs V1 vs V2")
	fmt.Println("====================================================================")

	extfhOK := isam.ExtfhAvailable()
	fmt.Printf("  EXTFH available: %v\n", extfhOK)

	totalExt, totalV1, totalV2 := 0, 0, 0
	v2MatchExt, v1MatchExt := 0, 0
	totalFiles := 0

	for _, f := range files {
		path := filepath.Join(dataPath, f.name)

		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("\n%-12s %-20s SKIP (not found)\n", f.name, f.desc)
			continue
		}
		totalFiles++

		// EXTFH reader (gold standard)
		var extRecords [][]byte
		var extCount, extRecSize int
		var extErr error
		if extfhOK {
			ef, err := isam.OpenIsamFile(path)
			if err == nil {
				extRecords, err = ef.ReadAll()
				ef.Close()
				if err != nil {
					extErr = err
				} else {
					extCount = len(extRecords)
					extRecSize = ef.RecSize
				}
			} else {
				extErr = err
			}
		}

		// V1 reader (heuristic)
		v1info, v1err := isam.ReadFile(path)
		v1count := 0
		v1recSize := 0
		if v1err == nil {
			v1count = len(v1info.Records)
			v1recSize = v1info.RecordSize
		}

		// V2 reader (spec-based)
		v2records, v2stats, v2err := isam.ReadFileV2WithStats(path)
		v2count := len(v2records)

		fmt.Printf("\n%-12s %-20s\n", f.name, f.desc)

		// Show counts
		if extfhOK {
			if extErr != nil {
				fmt.Printf("  EXTFH: ERROR %v\n", extErr)
			} else {
				fmt.Printf("  EXTFH: %4d records (recSize=%d) [GOLD STANDARD]\n", extCount, extRecSize)
				totalExt += extCount
			}
		}

		if v1err != nil {
			fmt.Printf("  V1:    ERROR %v\n", v1err)
		} else {
			fmt.Printf("  V1:    %4d records (recSize=%d)\n", v1count, v1recSize)
			totalV1 += v1count
		}

		if v2err != nil {
			fmt.Printf("  V2:    ERROR %v\n", v2err)
		} else {
			fmt.Printf("  V2:    %4d records (recSize=%d, idxfmt=%d)\n",
				v2count, v2stats.Header.MaxRecordLen, v2stats.Header.IdxFormat)
			fmt.Printf("         types: normal=%d reduced=%d refdata=%d redref=%d | deleted=%d null=%d sys=%d hdr=%d ptr=%d\n",
				v2stats.DataTypes[isam.RecTypeNormal],
				v2stats.DataTypes[isam.RecTypeReduced],
				v2stats.DataTypes[isam.RecTypeRefData],
				v2stats.DataTypes[isam.RecTypeRedRef],
				v2stats.DeletedCount, v2stats.NullCount,
				v2stats.SystemCount, v2stats.DataTypes[isam.RecTypeDeleted], v2stats.PointerCount)
			totalV2 += v2count
		}

		// Compare against EXTFH gold standard
		if extfhOK && extErr == nil {
			v1Match := v1err == nil && v1count == extCount
			v2Match := v2err == nil && v2count == extCount

			if v1Match {
				v1MatchExt++
			}
			if v2Match {
				v2MatchExt++
			}

			// Status line
			v1Status := "MISMATCH"
			if v1Match {
				v1Status = "MATCH"
			} else if v1err != nil {
				v1Status = "ERROR"
			}
			v2Status := "MISMATCH"
			if v2Match {
				v2Status = "MATCH"
			} else if v2err != nil {
				v2Status = "ERROR"
			}

			fmt.Printf("  vs EXTFH:  V1=%s", v1Status)
			if !v1Match && v1err == nil {
				fmt.Printf(" (delta=%+d)", v1count-extCount)
			}
			fmt.Printf("  V2=%s", v2Status)
			if !v2Match && v2err == nil {
				fmt.Printf(" (delta=%+d)", v2count-extCount)
			}
			fmt.Println()

			// Data comparison: V2 vs EXTFH (first record)
			if v2err == nil && v2count > 0 && extCount > 0 {
				compareData("EXTFH vs V2 first", extRecords[0], v2records[0])
				if extCount > 1 && v2count > 1 {
					compareData("EXTFH vs V2 last", extRecords[extCount-1], v2records[v2count-1])
				}

				// Count how many EXTFH records have exact match in V2
				if v2count == extCount {
					matchCount := 0
					for i := 0; i < extCount; i++ {
						if bytes.Equal(extRecords[i], v2records[i]) {
							matchCount++
						}
					}
					pct := float64(matchCount) / float64(extCount) * 100
					fmt.Printf("  Data match: %d/%d records identical (%.1f%%)\n", matchCount, extCount, pct)
					if matchCount < extCount {
						// Show first mismatching record
						for i := 0; i < extCount; i++ {
							if !bytes.Equal(extRecords[i], v2records[i]) {
								fmt.Printf("  First data diff at record %d:\n", i)
								compareData("  EXTFH vs V2", extRecords[i], v2records[i])
								break
							}
						}
					}
				}
			}
		}
	}

	fmt.Println("\n====================================================================")
	fmt.Println("  SUMMARY")
	fmt.Println("====================================================================")
	fmt.Printf("  Files tested: %d\n", totalFiles)
	if extfhOK {
		fmt.Printf("  EXTFH total: %d records\n", totalExt)
		fmt.Printf("  V1 matches EXTFH: %d/%d files\n", v1MatchExt, totalFiles)
		fmt.Printf("  V2 matches EXTFH: %d/%d files\n", v2MatchExt, totalFiles)
	}
	fmt.Printf("  Total records: EXTFH=%d  V1=%d  V2=%d\n", totalExt, totalV1, totalV2)
	fmt.Println("====================================================================")
}

func compareData(label string, a, b []byte) {
	if len(a) == 0 || len(b) == 0 {
		return
	}

	maxLen := len(a)
	if len(b) < maxLen {
		maxLen = len(b)
	}

	diffCount := 0
	firstDiff := -1
	for i := 0; i < maxLen; i++ {
		if a[i] != b[i] {
			diffCount++
			if firstDiff == -1 {
				firstDiff = i
			}
		}
	}

	if diffCount == 0 && len(a) == len(b) {
		fmt.Printf("  %s: IDENTICAL (%d bytes)\n", label, len(a))
	} else {
		fmt.Printf("  %s: %d diffs (len %d vs %d)\n", label, diffCount, len(a), len(b))
		if firstDiff >= 0 {
			// Show context around first diff
			start := firstDiff
			if start > 0 {
				start--
			}
			end := firstDiff + 20
			if end > maxLen {
				end = maxLen
			}
			fmt.Printf("    diff@%d: a=[% X] b=[% X]\n", firstDiff,
				a[start:end], b[start:end])
		}
		// Show printable preview
		aText := extractPrintable(a, 80)
		bText := extractPrintable(b, 80)
		if aText != bText {
			fmt.Printf("    a: %s\n", aText)
			fmt.Printf("    b: %s\n", bText)
		}
	}
}

func extractPrintable(data []byte, maxLen int) string {
	var sb strings.Builder
	for _, b := range data {
		if sb.Len() >= maxLen {
			sb.WriteString("...")
			break
		}
		if b >= 0x20 && b <= 0x7E {
			sb.WriteByte(b)
		} else if b == 0 {
			// skip nulls
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}
