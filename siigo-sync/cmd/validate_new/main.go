package main

import (
	"fmt"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\DEMOS01\`

	fmt.Println("============================================")
	fmt.Println("VALIDACION DE NUEVOS PARSERS")
	fmt.Println("============================================")

	validateZ07T(dataPath)
	validateZ26(dataPath)
	validateZ05(dataPath)
}

func validateZ07T(dataPath string) {
	fmt.Println("\n=== Z07T (Transacciones Detalle) ===")
	recs, err := parsers.ParseTransaccionesDetalle(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Total registros: %d\n", len(recs))

	// Stats
	tipos := map[string]int{}
	tiposMov := map[string]int{}
	tiposSec := map[string]int{}
	tiposTrans := map[string]int{}
	emptyFecha, emptyFechaVenc, emptyNit, emptyCuenta, emptyValor := 0, 0, 0, 0, 0
	var minVal, maxVal float64
	first := true

	for _, r := range recs {
		tipos[r.TipoComprobante]++
		tiposMov[r.TipoMovimiento]++
		tiposSec[r.TipoCompSec]++
		tiposTrans[r.TipoTransaccion]++
		if r.FechaDocumento == "" {
			emptyFecha++
		}
		if r.FechaVencimiento == "" {
			emptyFechaVenc++
		}
		if r.NitTercero == "" {
			emptyNit++
		}
		if r.CuentaContable == "" {
			emptyCuenta++
		}
		if r.Valor == 0 {
			emptyValor++
		}
		if first || r.Valor < minVal {
			minVal = r.Valor
		}
		if first || r.Valor > maxVal {
			maxVal = r.Valor
		}
		first = false
	}

	fmt.Printf("Tipos comprobante: %v\n", tipos)
	fmt.Printf("Tipos movimiento (D/C): %v\n", tiposMov)
	fmt.Printf("Tipos comp secundario: %v\n", tiposSec)
	fmt.Printf("Tipos transaccion: %v\n", tiposTrans)
	fmt.Printf("Vacios: fecha=%d, fechaVenc=%d, nit=%d, cuenta=%d, valor=%d\n",
		emptyFecha, emptyFechaVenc, emptyNit, emptyCuenta, emptyValor)
	fmt.Printf("Rango valores: min=%.2f max=%.2f\n", minVal, maxVal)

	// Show first 10
	fmt.Println("\nPrimeros 10 registros:")
	for i, r := range recs {
		if i >= 10 {
			break
		}
		fmt.Printf("  [%d] %s %s emp:%s nit:%-12s cta:%-9s fecha:%s mov:%s val:%.0f | sec:%s-%s fechaV:%s trans:%s\n",
			i+1, r.TipoComprobante, r.Secuencia, r.Empresa,
			r.NitTercero, r.CuentaContable, r.FechaDocumento,
			r.TipoMovimiento, r.Valor,
			r.TipoCompSec, r.SecuenciaSec, r.FechaVencimiento, r.TipoTransaccion)
	}

	// Show last 5
	fmt.Println("\nUltimos 5 registros:")
	start := len(recs) - 5
	if start < 0 {
		start = 0
	}
	for i := start; i < len(recs); i++ {
		r := recs[i]
		fmt.Printf("  [%d] %s %s emp:%s nit:%-12s cta:%-9s fecha:%s mov:%s val:%.0f | sec:%s-%s\n",
			i+1, r.TipoComprobante, r.Secuencia, r.Empresa,
			r.NitTercero, r.CuentaContable, r.FechaDocumento,
			r.TipoMovimiento, r.Valor,
			r.TipoCompSec, r.SecuenciaSec)
	}

	// Show some D and some C records
	fmt.Println("\n5 registros DEBITO:")
	shown := 0
	for _, r := range recs {
		if shown >= 5 {
			break
		}
		if r.TipoMovimiento == "D" {
			shown++
			fmt.Printf("  %s emp:%s nit:%-12s cta:%-9s fecha:%s val:%.0f | %s-%s\n",
				r.TipoComprobante, r.Empresa, r.NitTercero, r.CuentaContable,
				r.FechaDocumento, r.Valor, r.TipoCompSec, r.SecuenciaSec)
		}
	}

	fmt.Println("\n5 registros CREDITO:")
	shown = 0
	for _, r := range recs {
		if shown >= 5 {
			break
		}
		if r.TipoMovimiento == "C" {
			shown++
			fmt.Printf("  %s emp:%s nit:%-12s cta:%-9s fecha:%s val:%.0f | %s-%s\n",
				r.TipoComprobante, r.Empresa, r.NitTercero, r.CuentaContable,
				r.FechaDocumento, r.Valor, r.TipoCompSec, r.SecuenciaSec)
		}
	}
}

func validateZ26(dataPath string) {
	fmt.Println("\n\n=== Z26 (Periodos Contables) ===")
	recs, year, err := parsers.ParsePeriodos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Archivo: Z26%s, Total: %d periodos\n", year, len(recs))

	estados := map[string]int{}
	emptyInicio, emptyFin := 0, 0

	for _, r := range recs {
		estados[r.Estado]++
		if r.FechaInicio == "" {
			emptyInicio++
		}
		if r.FechaFin == "" {
			emptyFin++
		}
	}
	fmt.Printf("Estados: %v\n", estados)
	fmt.Printf("Vacios: fechaInicio=%d, fechaFin=%d\n", emptyInicio, emptyFin)

	fmt.Println("\nTodos los periodos:")
	for i, r := range recs {
		fmt.Printf("  [%d] periodo:%s emp:%s inicio:%s fin:%s estado:%s saldo1:%.2f saldo2:%.2f saldo3:%.2f\n",
			i+1, r.NumeroPeriodo, r.Empresa, r.FechaInicio, r.FechaFin, r.Estado,
			r.Saldo1, r.Saldo2, r.Saldo3)
	}
}

func validateZ05(dataPath string) {
	fmt.Println("\n\n=== Z05 (Condiciones de Pago) ===")
	recs, year, err := parsers.ParseCondicionesPago(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Archivo: Z05%s, Total: %d registros\n", year, len(recs))

	tipos := map[string]int{}
	flags := map[string]int{}
	tiposDoc := map[string]int{}
	emptyNit, emptyFecha, emptyValor := 0, 0, 0

	for _, r := range recs {
		tipos[r.Tipo]++
		flags[r.FlagByte]++
		tiposDoc[r.TipoDoc]++
		if r.NIT == "" {
			emptyNit++
		}
		if r.Fecha == "" {
			emptyFecha++
		}
		if r.Valor == 0 {
			emptyValor++
		}
	}
	fmt.Printf("Tipos: %v\n", tipos)
	fmt.Printf("Flag bytes: %v\n", flags)
	fmt.Printf("Tipos doc: %v\n", tiposDoc)
	fmt.Printf("Vacios: nit=%d, fecha=%d, valor=%d\n", emptyNit, emptyFecha, emptyValor)

	fmt.Println("\nTodos los registros:")
	for i, r := range recs {
		fmt.Printf("  [%d] tipo:%s emp:%s flag:%s sec:%s doc:%s fecha:%s nit:%-12s sec2:%s val:%.2f fechaR:%s\n",
			i+1, r.Tipo, r.Empresa, r.FlagByte, r.Secuencia, r.TipoDoc,
			r.Fecha, r.NIT, r.TipoSecundario, r.Valor, r.FechaRegistro)
	}
}
