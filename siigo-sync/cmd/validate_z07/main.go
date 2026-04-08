package main

import (
	"fmt"
	"math"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\SIIWI02\`

	fmt.Println("=== VALIDACION EXHAUSTIVA Z07 (LIBROS AUXILIARES) ===")
	fmt.Println()

	entries, year, err := parsers.ParseLibrosAuxiliares(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("File: Z07%s, Total: %d records\n\n", year, len(entries))

	// Show ALL records
	fmt.Println("--- TODOS LOS REGISTROS ---")
	for i, e := range entries {
		fmt.Printf("[%2d] emp:%-3s cuenta:%-9s tipo:%s-%s nit:%-10s fechaDoc:%-8s fechaReg:%-8s saldo:%12.2f deb:%12.2f cred:%12.2f ref:%-7s sec:%s-%s\n",
			i, e.Company, e.LedgerAccount, e.VoucherType, e.VoucherCode,
			e.ThirdPartyNit, e.DocDate, e.RegDate,
			e.Balance, e.Debit, e.Credit,
			e.RefNumber, e.SecVoucherType, e.SecVoucherCode)
	}

	// Statistics
	fmt.Println("\n--- ESTADISTICAS ---")

	// Unique cuentas
	cuentas := map[string]int{}
	nits := map[string]int{}
	tipos := map[string]int{}
	tiposSec := map[string]int{}
	empresas := map[string]int{}
	refs := map[string]int{}
	for _, e := range entries {
		cuentas[e.LedgerAccount]++
		nits[e.ThirdPartyNit]++
		tipos[e.VoucherType]++
		tiposSec[e.SecVoucherType]++
		empresas[e.Company]++
		refs[e.RefNumber]++
	}

	fmt.Printf("Empresas unicas: %v\n", empresas)
	fmt.Printf("Cuentas unicas (%d): ", len(cuentas))
	for c, n := range cuentas {
		fmt.Printf("%s(%d) ", c, n)
	}
	fmt.Println()

	fmt.Printf("NITs unicos (%d): ", len(nits))
	for n, count := range nits {
		fmt.Printf("%s(%d) ", n, count)
	}
	fmt.Println()

	fmt.Printf("Tipos comprobante: %v\n", tipos)
	fmt.Printf("Tipos secundarios: %v\n", tiposSec)
	fmt.Printf("Referencias unicas (%d): %v\n", len(refs), refs)

	// Date ranges
	minDateDoc, maxDateDoc := "99999999", "00000000"
	minDateReg, maxDateReg := "99999999", "00000000"
	for _, e := range entries {
		if e.DocDate < minDateDoc {
			minDateDoc = e.DocDate
		}
		if e.DocDate > maxDateDoc {
			maxDateDoc = e.DocDate
		}
		if e.RegDate != "" {
			if e.RegDate < minDateReg {
				minDateReg = e.RegDate
			}
			if e.RegDate > maxDateReg {
				maxDateReg = e.RegDate
			}
		}
	}
	fmt.Printf("Rango fecha documento: %s a %s\n", minDateDoc, maxDateDoc)
	fmt.Printf("Rango fecha registro:  %s a %s\n", minDateReg, maxDateReg)

	// Saldo stats
	minSaldo, maxSaldo := math.MaxFloat64, -math.MaxFloat64
	sumSaldo := 0.0
	nonZeroSaldo := 0
	negativeSaldo := 0
	for _, e := range entries {
		if e.Balance < minSaldo {
			minSaldo = e.Balance
		}
		if e.Balance > maxSaldo {
			maxSaldo = e.Balance
		}
		sumSaldo += e.Balance
		if e.Balance != 0 {
			nonZeroSaldo++
		}
		if e.Balance < 0 {
			negativeSaldo++
		}
	}
	fmt.Printf("Saldos: min=%.2f max=%.2f sum=%.2f nonZero=%d negative=%d\n",
		minSaldo, maxSaldo, sumSaldo, nonZeroSaldo, negativeSaldo)

	// Check debito/credito
	nonZeroDeb, nonZeroCred := 0, 0
	for _, e := range entries {
		if e.Debit != 0 {
			nonZeroDeb++
		}
		if e.Credit != 0 {
			nonZeroCred++
		}
	}
	fmt.Printf("Debito no-cero: %d, Credito no-cero: %d\n", nonZeroDeb, nonZeroCred)

	// Cross-check: tipo principal vs secundario
	fmt.Println("\n--- CRUCE TIPO vs TIPO SECUNDARIO ---")
	cross := map[string]int{}
	for _, e := range entries {
		key := e.VoucherType + "->" + e.SecVoucherType
		cross[key]++
	}
	for k, v := range cross {
		fmt.Printf("  %s: %d\n", k, v)
	}

	// Verify fechaDoc vs fechaReg relationship
	fmt.Println("\n--- RELACION FECHA DOC vs FECHA REG ---")
	docBeforeReg, docAfterReg, docEqReg := 0, 0, 0
	for _, e := range entries {
		if e.DocDate < e.RegDate {
			docBeforeReg++
		} else if e.DocDate > e.RegDate {
			docAfterReg++
		} else {
			docEqReg++
		}
	}
	fmt.Printf("  fechaDoc < fechaReg: %d\n", docBeforeReg)
	fmt.Printf("  fechaDoc > fechaReg: %d\n", docAfterReg)
	fmt.Printf("  fechaDoc = fechaReg: %d\n", docEqReg)
}
