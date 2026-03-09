package main

import (
	"fmt"
	"siigo-common/isam"
	"siigo-common/parsers"
	"strings"
)

func main() {
	dataPath := `C:\DEMOS01\`

	fmt.Println("=================================================================")
	fmt.Println("        VALIDACION COMPLETA DE PARSERS Y OFFSETS")
	fmt.Println("=================================================================")
	fmt.Printf("EXTFH disponible: %v\n\n", isam.ExtfhAvailable())

	validateTerceros(dataPath)
	validateInventario(dataPath)
	validateMovimientos(dataPath)
	validateCartera(dataPath, "2013")
	validateCartera(dataPath, "2014")

	fmt.Println("\n=== RESUMEN FINAL ===")
}

func validateTerceros(dataPath string) {
	fmt.Println("--------------------------------------------------")
	fmt.Println("1. Z17 (TERCEROS) - recSize=1438")
	fmt.Println("   Offsets EXTFH: tipo@0(1) empresa@1(3) codigo@4(14)")
	fmt.Println("   tipoDoc@18(2) numDoc@22(6) fecha@28(8) nombre@36(40) tipoCtaPref@86(1)")
	fmt.Println("--------------------------------------------------")
	all, err := parsers.ParseTerceros(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d records\n", len(all))

	problems := 0
	for i, t := range all {
		issues := []string{}
		if t.KeyType == "" {
			issues = append(issues, "tipo_clave vacio")
		}
		if t.Company == "" {
			issues = append(issues, "empresa vacia")
		}
		if t.Name == "" {
			issues = append(issues, "nombre vacio")
		}
		if len(t.CreationDate) != 8 {
			issues = append(issues, fmt.Sprintf("fecha invalida '%s'", t.CreationDate))
		} else {
			y := t.CreationDate[:4]
			if y < "1990" || y > "2030" {
				issues = append(issues, fmt.Sprintf("year fuera de rango: %s", y))
			}
		}
		if t.DocType == "" {
			issues = append(issues, "tipo_doc vacio")
		}
		if len(issues) > 0 {
			problems++
			if problems <= 5 {
				fmt.Printf("  PROBLEMA reg[%d]: %s | nombre='%s' fecha='%s'\n", i, strings.Join(issues, ", "), t.Name, t.CreationDate)
			}
		}
	}
	if problems == 0 {
		fmt.Println("  OK: All campos parsed correctamente")
	} else {
		fmt.Printf("  FAILED: %d records with issues\n", problems)
	}

	fmt.Println("\n  Sample (5 records distributed):")
	step := len(all) / 5
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(all) && i/step < 5; i += step {
		t := all[i]
		fmt.Printf("    [%3d] clave:%-1s emp:%-3s tipoDoc:%-2s doc:%-14s fecha:%-8s ctaPref:%-1s | %s\n",
			i, t.KeyType, t.Company, t.DocType, t.DocNumber, t.CreationDate, t.PreferredAcctType, t.Name)
	}
	fmt.Println()
}

func validateInventario(dataPath string) {
	fmt.Println("--------------------------------------------------")
	fmt.Println("2. Z04YYYY (INVENTARIO) - recSize=3520")
	fmt.Println("   Offsets EXTFH: empresa@0(5) grupo@5(3) codigo@8(6)")
	fmt.Println("   nombre@14(50) nombreCorto@64(40) referencia@104(30)")
	fmt.Println("--------------------------------------------------")
	prods, year, err := parsers.ParseInventario(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  File: Z04%s, Total: %d records\n", year, len(prods))

	problems := 0
	emptyNombre, emptyCode, emptyEmp := 0, 0, 0
	for i, p := range prods {
		issues := []string{}
		if p.Name == "" {
			emptyNombre++
			issues = append(issues, "nombre vacio")
		}
		if p.Code == "" {
			emptyCode++
			issues = append(issues, "codigo vacio")
		}
		if p.Company == "" {
			emptyEmp++
			issues = append(issues, "empresa vacia")
		}
		if len(issues) > 0 {
			problems++
			if problems <= 3 {
				fmt.Printf("  PROBLEMA reg[%d]: %s | cod='%s' nombre='%s'\n", i, strings.Join(issues, ", "), p.Code, p.Name)
			}
		}
	}
	if problems == 0 {
		fmt.Println("  OK: All campos parsed correctamente")
	} else {
		fmt.Printf("  FAILED: %d problemas (nombre=%d, cod=%d, emp=%d)\n", problems, emptyNombre, emptyCode, emptyEmp)
	}

	fmt.Println("\n  Sample (5 records distributed):")
	step := len(prods) / 5
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(prods) && i/step < 5; i += step {
		p := prods[i]
		fmt.Printf("    [%3d] emp:%-5s grupo:%-3s cod:%-8s corto:%-20s ref:%-10s | %s\n",
			i, p.Company, p.Group, p.Code, p.ShortName, p.Reference, p.Name)
	}
	fmt.Println()
}

func validateMovimientos(dataPath string) {
	fmt.Println("--------------------------------------------------")
	fmt.Println("3. Z49 (MOVIMIENTOS - Encabezados) - recSize=2295")
	fmt.Println("   Offsets EXTFH: tipo@0(1) codigo@1(3) numDoc@4(11)")
	fmt.Println("   nombre@15(35) desc@72-128 desc2@129-192")
	fmt.Println("   NOTA: Solo encabezados. NO tiene fecha/cuenta/valor/D-C.")
	fmt.Println("--------------------------------------------------")
	movs, err := parsers.ParseMovimientos(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d records\n", len(movs))

	emptyTipo, emptyDoc, emptyDesc, emptyNombre, withDesc2 := 0, 0, 0, 0, 0
	tiposComp := map[string]int{}
	for _, m := range movs {
		if m.VoucherType == "" {
			emptyTipo++
		}
		if m.DocNumber == "" {
			emptyDoc++
		}
		if m.Description == "" && m.Description2 == "" {
			emptyDesc++
		}
		if m.ThirdPartyName == "" {
			emptyNombre++
		}
		if m.Description2 != "" {
			withDesc2++
		}
		if len(m.VoucherType) >= 2 {
			tiposComp[m.VoucherType[:2]]++
		}
	}
	fmt.Printf("  Vacios: tipo=%d, doc=%d, desc=%d, nombre=%d\n", emptyTipo, emptyDoc, emptyDesc, emptyNombre)
	fmt.Printf("  Con descripcion2: %d\n", withDesc2)
	fmt.Printf("  Tipos: %v\n", tiposComp)

	if emptyTipo <= 30 && emptyDoc == 0 {
		fmt.Println("  OK: Campos clave parsed correctamente")
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	fmt.Println("\n  Sample por tipo de comprobante:")
	shownTypes := map[string]bool{}
	for _, m := range movs {
		if m.VoucherType == "" {
			continue
		}
		k := m.VoucherType
		if len(k) >= 2 {
			k = k[:2]
		}
		if shownTypes[k] {
			continue
		}
		shownTypes[k] = true
		desc := m.Description
		if m.Description2 != "" {
			if desc != "" {
				desc += " | " + m.Description2
			} else {
				desc = m.Description2
			}
		}
		fmt.Printf("    %-6s doc:%-11s nombre:%-30s | %s\n", m.VoucherType, m.DocNumber, m.ThirdPartyName, desc)
		if len(shownTypes) >= 10 {
			break
		}
	}
	fmt.Println()
}

func validateCartera(dataPath string, anio string) {
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("4. Z09%s (CARTERA) - recSize=1152\n", anio)
	fmt.Println("   Offsets EXTFH: tipo@0(1) empresa@1(3) seq@10(5) tipoDoc@15(1)")
	fmt.Println("   nit@16(13) cuenta@29(13) fecha@42(8) desc@93(40) D/C@143(1)")
	fmt.Println("--------------------------------------------------")
	cart, err := parsers.ParseCartera(dataPath, anio)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	fmt.Printf("  Total: %d records\n", len(cart))

	emptyTipo, emptyEmp, emptySec, emptyNit, emptyFecha, emptyDC, emptyCuenta := 0, 0, 0, 0, 0, 0, 0
	badDates := 0
	tipos := map[string]int{}
	dc := map[string]int{}
	for _, c := range cart {
		if c.RecordType == "" {
			emptyTipo++
		}
		if c.Company == "" {
			emptyEmp++
		}
		if c.Sequence == "" {
			emptySec++
		}
		if c.ThirdPartyNit == "" {
			emptyNit++
		}
		if c.Date == "" {
			emptyFecha++
		}
		if c.MovType == "" {
			emptyDC++
		}
		if c.LedgerAccount == "" {
			emptyCuenta++
		}
		tipos[c.RecordType]++
		dc[c.MovType]++

		if c.Date != "" && len(c.Date) == 8 {
			y := c.Date[:4]
			if y < "1990" || y > "2030" {
				badDates++
			}
		} else if c.Date != "" {
			badDates++
		}
	}
	fmt.Printf("  Vacios: tipo=%d emp=%d seq=%d nit=%d fecha=%d D/C=%d cuenta=%d\n",
		emptyTipo, emptyEmp, emptySec, emptyNit, emptyFecha, emptyDC, emptyCuenta)
	fmt.Printf("  Fechas invalidas: %d\n", badDates)
	fmt.Printf("  Tipos registro: %v\n", tipos)
	fmt.Printf("  Tipos D/C: %v\n", dc)

	if emptyTipo == 0 && emptyEmp == 0 && emptyFecha == 0 && badDates == 0 && emptyDC == 0 {
		fmt.Println("  OK: All campos clave parsed correctamente")
	} else if emptyTipo == 0 && emptyEmp == 0 {
		fmt.Printf("  PARCIAL: Campos clave OK, algunos secundarios vacios (nit=%d)\n", emptyNit)
	} else {
		fmt.Println("  FAILED: Revisar campos vacios")
	}

	fmt.Println("\n  Sample (5 records distributed):")
	step := len(cart) / 5
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(cart) && i/step < 5; i += step {
		c := cart[i]
		fmt.Printf("    [%3d] tipo:%-1s emp:%-3s seq:%-5s nit:%-13s fecha:%-8s D/C:%-1s cuenta:%-13s | %s\n",
			i, c.RecordType, c.Company, c.Sequence, c.ThirdPartyNit, c.Date, c.MovType, c.LedgerAccount, c.Description)
	}
	fmt.Println()
}
