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
	fmt.Printf("Total records: %d\n", len(recs))

	// Stats
	tipos := map[string]int{}
	tiposMov := map[string]int{}
	tiposSec := map[string]int{}
	tiposTrans := map[string]int{}
	emptyFecha, emptyFechaVenc, emptyNit, emptyCuenta, emptyValor := 0, 0, 0, 0, 0
	var minVal, maxVal float64
	first := true

	for _, r := range recs {
		tipos[r.VoucherType]++
		tiposMov[r.MovType]++
		tiposSec[r.SecVoucherType]++
		tiposTrans[r.TransType]++
		if r.DocDate == "" {
			emptyFecha++
		}
		if r.DueDate == "" {
			emptyFechaVenc++
		}
		if r.ThirdPartyNit == "" {
			emptyNit++
		}
		if r.LedgerAccount == "" {
			emptyCuenta++
		}
		if r.Amount == 0 {
			emptyValor++
		}
		if first || r.Amount < minVal {
			minVal = r.Amount
		}
		if first || r.Amount > maxVal {
			maxVal = r.Amount
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
	fmt.Println("\nFirst 10 records:")
	for i, r := range recs {
		if i >= 10 {
			break
		}
		fmt.Printf("  [%d] %s %s emp:%s nit:%-12s cta:%-9s fecha:%s mov:%s val:%.0f | sec:%s-%s fechaV:%s trans:%s\n",
			i+1, r.VoucherType, r.Sequence, r.Company,
			r.ThirdPartyNit, r.LedgerAccount, r.DocDate,
			r.MovType, r.Amount,
			r.SecVoucherType, r.SequenceSec, r.DueDate, r.TransType)
	}

	// Show last 5
	fmt.Println("\nLast 5 records:")
	start := len(recs) - 5
	if start < 0 {
		start = 0
	}
	for i := start; i < len(recs); i++ {
		r := recs[i]
		fmt.Printf("  [%d] %s %s emp:%s nit:%-12s cta:%-9s fecha:%s mov:%s val:%.0f | sec:%s-%s\n",
			i+1, r.VoucherType, r.Sequence, r.Company,
			r.ThirdPartyNit, r.LedgerAccount, r.DocDate,
			r.MovType, r.Amount,
			r.SecVoucherType, r.SequenceSec)
	}

	// Show some D and some C records
	fmt.Println("\n5 records DEBITO:")
	shown := 0
	for _, r := range recs {
		if shown >= 5 {
			break
		}
		if r.MovType == "D" {
			shown++
			fmt.Printf("  %s emp:%s nit:%-12s cta:%-9s fecha:%s val:%.0f | %s-%s\n",
				r.VoucherType, r.Company, r.ThirdPartyNit, r.LedgerAccount,
				r.DocDate, r.Amount, r.SecVoucherType, r.SequenceSec)
		}
	}

	fmt.Println("\n5 records CREDITO:")
	shown = 0
	for _, r := range recs {
		if shown >= 5 {
			break
		}
		if r.MovType == "C" {
			shown++
			fmt.Printf("  %s emp:%s nit:%-12s cta:%-9s fecha:%s val:%.0f | %s-%s\n",
				r.VoucherType, r.Company, r.ThirdPartyNit, r.LedgerAccount,
				r.DocDate, r.Amount, r.SecVoucherType, r.SequenceSec)
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
	fmt.Printf("File: Z26%s, Total: %d periodos\n", year, len(recs))

	estados := map[string]int{}
	emptyInicio, emptyFin := 0, 0

	for _, r := range recs {
		estados[r.Status]++
		if r.StartDate == "" {
			emptyInicio++
		}
		if r.EndDate == "" {
			emptyFin++
		}
	}
	fmt.Printf("Estados: %v\n", estados)
	fmt.Printf("Vacios: fechaInicio=%d, fechaFin=%d\n", emptyInicio, emptyFin)

	fmt.Println("\nAll periodos:")
	for i, r := range recs {
		fmt.Printf("  [%d] periodo:%s emp:%s inicio:%s fin:%s estado:%s saldo1:%.2f saldo2:%.2f saldo3:%.2f\n",
			i+1, r.PeriodNumber, r.Company, r.StartDate, r.EndDate, r.Status,
			r.Balance1, r.Balance2, r.Balance3)
	}
}

func validateZ05(dataPath string) {
	fmt.Println("\n\n=== Z05 (Condiciones de Pago) ===")
	recs, year, err := parsers.ParseCondicionesPago(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("File: Z05%s, Total: %d records\n", year, len(recs))

	tipos := map[string]int{}
	flags := map[string]int{}
	tiposDoc := map[string]int{}
	emptyNit, emptyFecha, emptyValor := 0, 0, 0

	for _, r := range recs {
		tipos[r.RecType]++
		flags[r.FlagByte]++
		tiposDoc[r.DocType]++
		if r.NIT == "" {
			emptyNit++
		}
		if r.Date == "" {
			emptyFecha++
		}
		if r.Amount == 0 {
			emptyValor++
		}
	}
	fmt.Printf("Tipos: %v\n", tipos)
	fmt.Printf("Flag bytes: %v\n", flags)
	fmt.Printf("Tipos doc: %v\n", tiposDoc)
	fmt.Printf("Vacios: nit=%d, fecha=%d, valor=%d\n", emptyNit, emptyFecha, emptyValor)

	fmt.Println("\nAll records:")
	for i, r := range recs {
		fmt.Printf("  [%d] tipo:%s emp:%s flag:%s sec:%s doc:%s fecha:%s nit:%-12s sec2:%s val:%.2f fechaR:%s\n",
			i+1, r.RecType, r.Company, r.FlagByte, r.Sequence, r.DocType,
			r.Date, r.NIT, r.SecondaryType, r.Amount, r.RegDate)
	}
}
