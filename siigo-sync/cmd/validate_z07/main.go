package main

import (
	"fmt"
	"math"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\DEMOS01\`

	fmt.Println("=== VALIDACION EXHAUSTIVA Z07 (LIBROS AUXILIARES) ===")
	fmt.Println()

	entries, year, err := parsers.ParseLibrosAuxiliares(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Archivo: Z07%s, Total: %d registros\n\n", year, len(entries))

	// Show ALL records
	fmt.Println("--- TODOS LOS REGISTROS ---")
	for i, e := range entries {
		fmt.Printf("[%2d] emp:%-3s cuenta:%-9s tipo:%s-%s nit:%-10s fechaDoc:%-8s fechaReg:%-8s saldo:%12.2f deb:%12.2f cred:%12.2f ref:%-7s sec:%s-%s\n",
			i, e.Empresa, e.CuentaContable, e.TipoComprobante, e.CodigoComprobante,
			e.NitTercero, e.FechaDocumento, e.FechaRegistro,
			e.Saldo, e.Debito, e.Credito,
			e.NumeroReferencia, e.TipoCompSec, e.CodigoCompSec)
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
		cuentas[e.CuentaContable]++
		nits[e.NitTercero]++
		tipos[e.TipoComprobante]++
		tiposSec[e.TipoCompSec]++
		empresas[e.Empresa]++
		refs[e.NumeroReferencia]++
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
		if e.FechaDocumento < minDateDoc {
			minDateDoc = e.FechaDocumento
		}
		if e.FechaDocumento > maxDateDoc {
			maxDateDoc = e.FechaDocumento
		}
		if e.FechaRegistro != "" {
			if e.FechaRegistro < minDateReg {
				minDateReg = e.FechaRegistro
			}
			if e.FechaRegistro > maxDateReg {
				maxDateReg = e.FechaRegistro
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
		if e.Saldo < minSaldo {
			minSaldo = e.Saldo
		}
		if e.Saldo > maxSaldo {
			maxSaldo = e.Saldo
		}
		sumSaldo += e.Saldo
		if e.Saldo != 0 {
			nonZeroSaldo++
		}
		if e.Saldo < 0 {
			negativeSaldo++
		}
	}
	fmt.Printf("Saldos: min=%.2f max=%.2f sum=%.2f nonZero=%d negative=%d\n",
		minSaldo, maxSaldo, sumSaldo, nonZeroSaldo, negativeSaldo)

	// Check debito/credito
	nonZeroDeb, nonZeroCred := 0, 0
	for _, e := range entries {
		if e.Debito != 0 {
			nonZeroDeb++
		}
		if e.Credito != 0 {
			nonZeroCred++
		}
	}
	fmt.Printf("Debito no-cero: %d, Credito no-cero: %d\n", nonZeroDeb, nonZeroCred)

	// Cross-check: tipo principal vs secundario
	fmt.Println("\n--- CRUCE TIPO vs TIPO SECUNDARIO ---")
	cross := map[string]int{}
	for _, e := range entries {
		key := e.TipoComprobante + "->" + e.TipoCompSec
		cross[key]++
	}
	for k, v := range cross {
		fmt.Printf("  %s: %d\n", k, v)
	}

	// Verify fechaDoc vs fechaReg relationship
	fmt.Println("\n--- RELACION FECHA DOC vs FECHA REG ---")
	docBeforeReg, docAfterReg, docEqReg := 0, 0, 0
	for _, e := range entries {
		if e.FechaDocumento < e.FechaRegistro {
			docBeforeReg++
		} else if e.FechaDocumento > e.FechaRegistro {
			docAfterReg++
		} else {
			docEqReg++
		}
	}
	fmt.Printf("  fechaDoc < fechaReg: %d\n", docBeforeReg)
	fmt.Printf("  fechaDoc > fechaReg: %d\n", docAfterReg)
	fmt.Printf("  fechaDoc = fechaReg: %d\n", docEqReg)
}
