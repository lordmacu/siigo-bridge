//go:build ignore

package main

import (
	"fmt"
	"github.com/xuri/excelize/v2"
)

func main() {
	f, err := excelize.OpenFile(`C:\Users\lordmacu\siigo\CARTERA NACIONAL (2).xlsx`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	fmt.Println("Sheet:", sheet)

	// Read headers from row 7 (as per CarteraImportService)
	for col := 1; col <= 15; col++ {
		cell, _ := excelize.CoordinatesToCellName(col, 7)
		val, _ := f.GetCellValue(sheet, cell)
		fmt.Printf("  Col %d (row 7): %q\n", col, val)
	}

	// Read first 5 data rows (from row 8)
	fmt.Println("\n--- Data rows ---")
	for row := 8; row <= 12; row++ {
		fmt.Printf("Row %d:", row)
		for col := 1; col <= 12; col++ {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			val, _ := f.GetCellValue(sheet, cell)
			if len(val) > 30 {
				val = val[:30] + "..."
			}
			fmt.Printf("  [%s]", val)
		}
		fmt.Println()
	}
}
