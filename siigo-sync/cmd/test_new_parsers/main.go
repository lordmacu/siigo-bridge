package main

import (
	"fmt"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\DEMOS01`

	// Test Z16 - Movimientos Inventario
	fmt.Println("=== Z16 - Movimientos Inventario ===")
	movs, year, err := parsers.ParseMovimientosInventario(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Year: %s | Records: %d\n\n", year, len(movs))
		for i, m := range movs {
			if i >= 10 {
				fmt.Printf("... and %d more\n", len(movs)-10)
				break
			}
			fmt.Printf("  [%d] key=%s\n", i+1, m.RecordKey)
			fmt.Printf("      empresa=%s grupo=%s producto=%s\n", m.Empresa, m.Grupo, m.CodigoProducto)
			fmt.Printf("      tipo=%s comp=%s seq=%s tipoDoc=%s\n", m.TipoComprobante, m.CodigoComp, m.Secuencia, m.TipoDoc)
			fmt.Printf("      fecha=%s cant=%s valor=%s D/C=%s\n\n", m.Fecha, m.Cantidad, m.Valor, m.TipoMov)
		}
	}

	// Test Z15 - Saldos Inventario
	fmt.Println("\n=== Z15 - Saldos Inventario ===")
	saldos, year2, err := parsers.ParseSaldosInventario(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Year: %s | Records: %d\n\n", year2, len(saldos))
		for i, s := range saldos {
			if i >= 10 {
				fmt.Printf("... and %d more\n", len(saldos)-10)
				break
			}
			fmt.Printf("  [%d] key=%s\n", i+1, s.RecordKey)
			fmt.Printf("      empresa=%s grupo=%s producto=%s\n", s.Empresa, s.Grupo, s.CodigoProducto)
			fmt.Printf("      saldoIni=%.2f entradas=%.2f salidas=%.2f saldoFin=%.2f\n\n", s.SaldoInicial, s.Entradas, s.Salidas, s.SaldoFinal)
		}
	}
}
