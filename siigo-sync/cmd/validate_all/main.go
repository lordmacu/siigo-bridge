package main

import (
	"fmt"
	"siigo-common/parsers"
	"strings"
)

func main() {
	dataPath := `C:\DEMOS01\`

	fmt.Println("=================================================================")
	fmt.Println("    VALIDACION DE PARSERS ADICIONALES (9 tablas)")
	fmt.Println("=================================================================")

	validatePlanCuentas(dataPath)
	validateActivosFijos(dataPath)
	validateDocumentos(dataPath)
	validateTercerosAmpliados(dataPath)
	validateSaldosTerceros(dataPath)
	validateSaldosConsolidados(dataPath)
	validateDane(dataPath)
	validateHistorial(dataPath)
	validateICA(dataPath)
	validatePILA(dataPath)
	validateLibrosAuxiliares(dataPath)
}

func validatePlanCuentas(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("1. Z03YYYY (PLAN DE CUENTAS) - recSize=1152")
	fmt.Println("   Offsets: empresa@0(3) cuenta@3(9) activa@12(1) auxiliar@13(1)")
	fmt.Println("   naturaleza@17(8) nombre@25(70)")
	fmt.Println("--------------------------------------------------")

	cuentas, year, err := parsers.ParsePlanCuentas(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z03%s, Total: %d records\n", year, len(cuentas))

	emptyEmp, emptyNombre, emptyCodigo := 0, 0, 0
	activas, auxiliares := 0, 0
	longNames := 0
	for _, c := range cuentas {
		if c.Company == "" {
			emptyEmp++
		}
		if c.Name == "" {
			emptyNombre++
		}
		if c.AccountCode == "" {
			emptyCodigo++
		}
		if c.Active {
			activas++
		}
		if c.Auxiliary {
			auxiliares++
		}
		if len(c.Name) > 50 {
			longNames++
		}
	}
	fmt.Printf("  Vacios: empresa=%d nombre=%d codigo=%d\n", emptyEmp, emptyNombre, emptyCodigo)
	fmt.Printf("  Activas: %d/%d, Auxiliares: %d/%d\n", activas, len(cuentas), auxiliares, len(cuentas))
	fmt.Printf("  Nombres > 50 chars: %d\n", longNames)

	if emptyNombre == 0 && emptyCodigo == 0 {
		fmt.Println("  OK: All campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Plan Cuentas", len(cuentas), func(i int) string {
		c := cuentas[i]
		return fmt.Sprintf("emp:%-3s cod:%-9s act:%v aux:%v nat:%-8s | %s",
			c.Company, c.AccountCode, c.Active, c.Auxiliary, c.Nature, c.Name)
	})
}

func validateActivosFijos(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("2. Z27YYYY (ACTIVOS FIJOS) - recSize=2048")
	fmt.Println("   Offsets: empresa@0(5) codigo@5(6) nombre@11(50)")
	fmt.Println("   nit@61(13) fecha@122(8)")
	fmt.Println("--------------------------------------------------")

	activos, year, err := parsers.ParseActivosFijos(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z27%s, Total: %d records\n", year, len(activos))

	emptyEmp, emptyNombre, emptyCodigo, emptyNit, emptyFecha := 0, 0, 0, 0, 0
	badDates := 0
	for _, a := range activos {
		if a.Company == "" {
			emptyEmp++
		}
		if a.Name == "" {
			emptyNombre++
		}
		if a.Code == "" {
			emptyCodigo++
		}
		if a.ResponsibleNit == "" {
			emptyNit++
		}
		if a.AcquisitionDate == "" {
			emptyFecha++
		} else if len(a.AcquisitionDate) == 8 {
			y := a.AcquisitionDate[:4]
			if y < "1990" || y > "2030" {
				badDates++
			}
		} else {
			badDates++
		}
	}
	fmt.Printf("  Vacios: empresa=%d nombre=%d codigo=%d nit=%d fecha=%d\n",
		emptyEmp, emptyNombre, emptyCodigo, emptyNit, emptyFecha)
	fmt.Printf("  Fechas invalidas: %d\n", badDates)

	if emptyNombre == 0 && emptyCodigo == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Activos Fijos", len(activos), func(i int) string {
		a := activos[i]
		return fmt.Sprintf("emp:%-5s cod:%-6s nit:%-13s fecha:%-8s | %s",
			a.Company, a.Code, a.ResponsibleNit, a.AcquisitionDate, a.Name)
	})
}

func validateDocumentos(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("3. Z11YYYY (DOCUMENTOS) - recSize=518")
	fmt.Println("   Offsets: tipo@0(1) codigo@1(3) seq@10(5) nit@21(13)")
	fmt.Println("   cuenta@29(13) prod@42(7) bodega@49(3) cc@52(3)")
	fmt.Println("   fecha@55(8) desc@93(50) D/C@143(1) ref@167(7)")
	fmt.Println("--------------------------------------------------")

	docs, year, err := parsers.ParseDocumentos(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z11%s, Total: %d records\n", year, len(docs))

	emptyTipo, emptySeq, emptyNit, emptyCuenta, emptyFecha, emptyDesc, emptyDC := 0, 0, 0, 0, 0, 0, 0
	badDates := 0
	tipos := map[string]int{}
	dc := map[string]int{}
	for _, d := range docs {
		if d.VoucherType == "" {
			emptyTipo++
		}
		if d.Sequence == "" {
			emptySeq++
		}
		if d.ThirdPartyNit == "" {
			emptyNit++
		}
		if d.LedgerAccount == "" {
			emptyCuenta++
		}
		if d.Date == "" {
			emptyFecha++
		} else if len(d.Date) == 8 {
			y := d.Date[:4]
			if y < "1990" || y > "2030" {
				badDates++
			}
		} else {
			badDates++
		}
		if d.Description == "" {
			emptyDesc++
		}
		if d.MovType == "" {
			emptyDC++
		}
		tipos[d.VoucherType]++
		dc[d.MovType]++
	}
	fmt.Printf("  Vacios: tipo=%d seq=%d nit=%d cuenta=%d fecha=%d desc=%d D/C=%d\n",
		emptyTipo, emptySeq, emptyNit, emptyCuenta, emptyFecha, emptyDesc, emptyDC)
	fmt.Printf("  Fechas invalidas: %d\n", badDates)
	fmt.Printf("  Tipos: %v\n", tipos)
	fmt.Printf("  D/C: %v\n", dc)

	if emptyTipo == 0 && emptyCuenta == 0 && emptyFecha == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else if emptyCuenta == 0 {
		fmt.Println("  PARCIAL: Cuenta OK, otros campos parciales")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Documentos", len(docs), func(i int) string {
		d := docs[i]
		return fmt.Sprintf("tipo:%-1s cod:%-3s seq:%-5s nit:%-10s cuenta:%-13s fecha:%-8s D/C:%-1s | %s",
			d.VoucherType, d.VoucherCode, d.Sequence, d.ThirdPartyNit, d.LedgerAccount,
			d.Date, d.MovType, d.Description)
	})
}

func validateTercerosAmpliados(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("4. Z08YYYYA (TERCEROS AMPLIADOS) - recSize=1152")
	fmt.Println("   Offsets: empresa@0(3) nit@5(8) tipoPersona@16(2)")
	fmt.Println("   nombre@18(60) repLegal@96(60) direccion@194(56) email@323(70)")
	fmt.Println("--------------------------------------------------")

	terceros, year, err := parsers.ParseTercerosAmpliados(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z08%sA, Total: %d records\n", year, len(terceros))

	emptyEmp, emptyNit, emptyNombre, emptyTipo, emptyDir, emptyEmail := 0, 0, 0, 0, 0, 0
	withRepLegal := 0
	tiposPersona := map[string]int{}
	for _, t := range terceros {
		if t.Company == "" {
			emptyEmp++
		}
		if t.Nit == "" {
			emptyNit++
		}
		if t.Name == "" {
			emptyNombre++
		}
		if t.PersonType == "" {
			emptyTipo++
		}
		if t.Address == "" {
			emptyDir++
		}
		if t.Email == "" {
			emptyEmail++
		}
		if t.LegalRep != "" {
			withRepLegal++
		}
		tiposPersona[t.PersonType]++
	}
	fmt.Printf("  Vacios: empresa=%d nit=%d nombre=%d tipoPersona=%d dir=%d email=%d\n",
		emptyEmp, emptyNit, emptyNombre, emptyTipo, emptyDir, emptyEmail)
	fmt.Printf("  Con representante legal: %d\n", withRepLegal)
	fmt.Printf("  Tipos persona: %v\n", tiposPersona)

	if emptyNit == 0 && emptyNombre == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Terceros Ampliados", len(terceros), func(i int) string {
		t := terceros[i]
		email := t.Email
		if len(email) > 25 {
			email = email[:25] + "..."
		}
		return fmt.Sprintf("emp:%-3s nit:%-10s tipo:%-2s dir:%-20s email:%-28s | %s",
			t.Company, t.Nit, t.PersonType, truncate(t.Address, 20), email, t.Name)
	})
}

func validateSaldosTerceros(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("5. Z25YYYY (SALDOS TERCEROS) - recSize=512")
	fmt.Println("   Offsets: empresa@0(3) cuenta@3(9) nit@12(13)")
	fmt.Println("   BCD: saldoAnt@25(8) debito@33(8) credito@41(8)")
	fmt.Println("--------------------------------------------------")

	saldos, year, err := parsers.ParseSaldosTerceros(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z25%s, Total: %d records\n", year, len(saldos))

	emptyEmp, emptyCuenta, emptyNit := 0, 0, 0
	zeroAll, negSaldo := 0, 0
	for _, s := range saldos {
		if s.Company == "" {
			emptyEmp++
		}
		if s.LedgerAccount == "" {
			emptyCuenta++
		}
		if s.ThirdPartyNit == "" {
			emptyNit++
		}
		if s.PrevBalance == 0 && s.Debit == 0 && s.Credit == 0 {
			zeroAll++
		}
		if s.FinalBalance < 0 {
			negSaldo++
		}
	}
	fmt.Printf("  Vacios: empresa=%d cuenta=%d nit=%d\n", emptyEmp, emptyCuenta, emptyNit)
	fmt.Printf("  Todos montos en cero: %d, Saldo final negativo: %d\n", zeroAll, negSaldo)

	if emptyCuenta == 0 && emptyNit == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Saldos Terceros", len(saldos), func(i int) string {
		s := saldos[i]
		return fmt.Sprintf("emp:%-3s cuenta:%-9s nit:%-10s ant:%.2f deb:%.2f cred:%.2f final:%.2f",
			s.Company, s.LedgerAccount, s.ThirdPartyNit,
			s.PrevBalance, s.Debit, s.Credit, s.FinalBalance)
	})
}

func validateSaldosConsolidados(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("6. Z28YYYY (SALDOS CONSOLIDADOS) - recSize=512")
	fmt.Println("   Offsets: empresa@0(3) cuenta@3(9)")
	fmt.Println("   BCD: saldoAnt@12(8) debito@20(8) credito@28(8)")
	fmt.Println("--------------------------------------------------")

	saldos, year, err := parsers.ParseSaldosConsolidados(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z28%s, Total: %d records\n", year, len(saldos))

	emptyEmp, emptyCuenta := 0, 0
	zeroAll, negSaldo := 0, 0
	for _, s := range saldos {
		if s.Company == "" {
			emptyEmp++
		}
		if s.LedgerAccount == "" {
			emptyCuenta++
		}
		if s.PrevBalance == 0 && s.Debit == 0 && s.Credit == 0 {
			zeroAll++
		}
		if s.FinalBalance < 0 {
			negSaldo++
		}
	}
	fmt.Printf("  Vacios: empresa=%d cuenta=%d\n", emptyEmp, emptyCuenta)
	fmt.Printf("  Todos montos en cero: %d, Saldo final negativo: %d\n", zeroAll, negSaldo)

	if emptyCuenta == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Saldos Consolidados", len(saldos), func(i int) string {
		s := saldos[i]
		return fmt.Sprintf("emp:%-3s cuenta:%-9s ant:%.2f deb:%.2f cred:%.2f final:%.2f",
			s.Company, s.LedgerAccount,
			s.PrevBalance, s.Debit, s.Credit, s.FinalBalance)
	})
}

func showSample(label string, total int, formatter func(i int) string) {
	fmt.Printf("\n  Sample (%s, %d total):\n", label, total)
	if total == 0 {
		fmt.Println("    (no records)")
		return
	}
	step := total / 5
	if step < 1 {
		step = 1
	}
	shown := 0
	for i := 0; i < total && shown < 5; i += step {
		fmt.Printf("    [%4d] %s\n", i, formatter(i))
		shown++
	}
}

func validateDane(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("7. ZDANE (CODIGOS DANE MUNICIPIOS)")
	fmt.Println("   Offsets: codigo@0(5) nombre@5(40)")
	fmt.Println("--------------------------------------------------")

	codigos, err := parsers.ParseDane(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d municipios\n", len(codigos))

	emptyCod, emptyNombre := 0, 0
	for _, c := range codigos {
		if c.Code == "" {
			emptyCod++
		}
		if c.Name == "" {
			emptyNombre++
		}
	}
	fmt.Printf("  Vacios: codigo=%d nombre=%d\n", emptyCod, emptyNombre)

	if emptyCod == 0 && emptyNombre == 0 {
		fmt.Println("  OK: All campos parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("DANE", len(codigos), func(i int) string {
		c := codigos[i]
		return fmt.Sprintf("cod:%s | %s", c.Code, c.Name)
	})
}

func validateHistorial(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("8. Z18YYYY (HISTORIAL DOCUMENTOS)")
	fmt.Println("   Offsets: tipo@0(1) subTipo@1(3) empresa@3(3)")
	fmt.Println("   fecha@39(8) nombre1@53(40) nombre2@161(40)")
	fmt.Println("--------------------------------------------------")

	docs, year, err := parsers.ParseHistorial(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z18%s, Total: %d records\n", year, len(docs))

	emptyFecha, emptyNombre1, emptyNombre2, emptyNit := 0, 0, 0, 0
	badDates := 0
	subTipos := map[string]int{}
	for _, d := range docs {
		if d.Date == "" {
			emptyFecha++
		} else if len(d.Date) == 8 {
			y := d.Date[:4]
			if y < "1990" || y > "2030" {
				badDates++
			}
		} else {
			badDates++
		}
		if d.OriginName == "" {
			emptyNombre1++
		}
		if d.DestName == "" {
			emptyNombre2++
		}
		if d.OriginNit == "" {
			emptyNit++
		}
		subTipos[d.SubType]++
	}
	fmt.Printf("  Vacios: fecha=%d nombre1=%d nombre2=%d nit=%d\n",
		emptyFecha, emptyNombre1, emptyNombre2, emptyNit)
	fmt.Printf("  Fechas invalidas: %d\n", badDates)
	fmt.Printf("  SubTipos: %v\n", subTipos)

	if emptyNombre1 == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  PARCIAL: Algunos nombres vacios")
	}

	showSample("Historial", len(docs), func(i int) string {
		d := docs[i]
		return fmt.Sprintf("tipo:%s sub:%-3s emp:%-3s fecha:%-8s nit:%-10s | %s / %s",
			d.RecordType, d.SubType, d.Company, d.Date, d.OriginNit, d.OriginName, d.DestName)
	})
}

func validateICA(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("9. ZICA (ACTIVIDADES ICA - Impuesto Industria y Comercio)")
	fmt.Println("   Offsets: codigo@0(5) nombre@5(50) tarifa@55(6)")
	fmt.Println("--------------------------------------------------")

	actividades, err := parsers.ParseICA(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d actividades\n", len(actividades))

	emptyCod, emptyNombre, emptyTarifa := 0, 0, 0
	for _, a := range actividades {
		if a.Code == "" {
			emptyCod++
		}
		if a.Name == "" {
			emptyNombre++
		}
		if a.Rate == "" {
			emptyTarifa++
		}
	}
	fmt.Printf("  Vacios: codigo=%d nombre=%d tarifa=%d\n", emptyCod, emptyNombre, emptyTarifa)

	if emptyCod == 0 && emptyNombre == 0 {
		fmt.Println("  OK: All campos parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("ICA", len(actividades), func(i int) string {
		a := actividades[i]
		return fmt.Sprintf("cod:%s tarifa:%-6s | %s", a.Code, a.Rate, a.Name)
	})
}

func validatePILA(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("10. ZPILA (SEGURIDAD SOCIAL - Conceptos PILA)")
	fmt.Println("    Offsets: tipo@0(8) fondo@8(4) concepto@12(3)")
	fmt.Println("    flags@30(2) tipoBase@32(4) baseCalculo@36(4)")
	fmt.Println("--------------------------------------------------")

	conceptos, err := parsers.ParsePILA(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d conceptos\n", len(conceptos))

	emptyTipo, emptyFondo, emptyConcepto, emptyFlags, emptyBase := 0, 0, 0, 0, 0
	fondos := map[string]int{}
	conceptoMap := map[string]int{}
	bases := map[string]int{}
	for _, c := range conceptos {
		if c.RecType == "" {
			emptyTipo++
		}
		if c.Fund == "" {
			emptyFondo++
		}
		if c.Concept == "" {
			emptyConcepto++
		}
		if c.Flags == "" {
			emptyFlags++
		}
		if c.CalcBase == "" {
			emptyBase++
		}
		fondos[c.Fund]++
		conceptoMap[c.Concept]++
		bases[c.CalcBase]++
	}
	fmt.Printf("  Vacios: tipo=%d fondo=%d concepto=%d flags=%d base=%d\n",
		emptyTipo, emptyFondo, emptyConcepto, emptyFlags, emptyBase)
	fmt.Printf("  Fondos: %v\n", fondos)
	fmt.Printf("  Conceptos: %v\n", conceptoMap)
	fmt.Printf("  Bases calculo: %v\n", bases)

	if emptyFondo == 0 && emptyConcepto == 0 {
		fmt.Println("  OK: All campos parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("PILA", len(conceptos), func(i int) string {
		c := conceptos[i]
		return fmt.Sprintf("tipo:%-8s fondo:%-4s concepto:%-3s flags:%-2s base:%-4s calc:%s",
			c.RecType, c.Fund, c.Concept, c.Flags, c.BaseType, c.CalcBase)
	})
}

func validateLibrosAuxiliares(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("11. Z07YYYY (LIBROS AUXILIARES)")
	fmt.Println("    Offsets: empresa@7(3) cuenta@10(9) tipoComp@20(1)")
	fmt.Println("    codComp@21(3) fechaDoc@33(8) nit@41(13)")
	fmt.Println("    numRef@144(7) tipoSec@155(1) codSec@156(3)")
	fmt.Println("    BCD: saldo@112(6) debito@118(6) credito@124(7)")
	fmt.Println("    fechaReg@133(8)")
	fmt.Println("--------------------------------------------------")

	entries, year, err := parsers.ParseLibrosAuxiliares(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z07%s, Total: %d records\n", year, len(entries))

	emptyEmp, emptyCuenta, emptyNit, emptyTipo, emptyFechaDoc, emptyFechaReg := 0, 0, 0, 0, 0, 0
	badDatesDoc, badDatesReg := 0, 0
	zeroSaldo := 0
	tipos := map[string]int{}
	tiposSec := map[string]int{}
	for _, e := range entries {
		if e.Company == "" {
			emptyEmp++
		}
		if e.LedgerAccount == "" {
			emptyCuenta++
		}
		if e.ThirdPartyNit == "" {
			emptyNit++
		}
		if e.VoucherType == "" {
			emptyTipo++
		}
		if e.DocDate == "" {
			emptyFechaDoc++
		} else if len(e.DocDate) == 8 {
			y := e.DocDate[:4]
			if y < "1990" || y > "2030" {
				badDatesDoc++
			}
		} else {
			badDatesDoc++
		}
		if e.RegDate == "" {
			emptyFechaReg++
		} else if len(e.RegDate) == 8 {
			y := e.RegDate[:4]
			if y < "1990" || y > "2030" {
				badDatesReg++
			}
		} else {
			badDatesReg++
		}
		if e.Balance == 0 && e.Debit == 0 && e.Credit == 0 {
			zeroSaldo++
		}
		tipos[e.VoucherType]++
		tiposSec[e.SecVoucherType]++
	}
	fmt.Printf("  Vacios: empresa=%d cuenta=%d nit=%d tipo=%d fechaDoc=%d fechaReg=%d\n",
		emptyEmp, emptyCuenta, emptyNit, emptyTipo, emptyFechaDoc, emptyFechaReg)
	fmt.Printf("  Fechas invalidas: doc=%d reg=%d\n", badDatesDoc, badDatesReg)
	fmt.Printf("  Saldo/debito/credito todos cero: %d\n", zeroSaldo)
	fmt.Printf("  Tipos comprobante: %v\n", tipos)
	fmt.Printf("  Tipos secundarios: %v\n", tiposSec)

	if emptyCuenta == 0 && emptyNit == 0 && emptyFechaDoc == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else if emptyCuenta == 0 {
		fmt.Println("  PARCIAL: Cuenta OK, otros campos parciales")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	showSample("Libros Auxiliares", len(entries), func(i int) string {
		e := entries[i]
		return fmt.Sprintf("emp:%-3s cuenta:%-9s tipo:%s-%s nit:%-10s fechaDoc:%-8s fechaReg:%-8s saldo:%.2f deb:%.2f cred:%.2f ref:%s sec:%s-%s",
			e.Company, e.LedgerAccount, e.VoucherType, e.VoucherCode,
			e.ThirdPartyNit, e.DocDate, e.RegDate,
			e.Balance, e.Debit, e.Credit,
			e.RefNumber, e.SecVoucherType, e.SecVoucherCode)
	})
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
