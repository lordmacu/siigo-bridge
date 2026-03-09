package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"siigo-common/isam"
	"siigo-common/parsers"
	"strings"
)

func main() {
	dataPath := `C:\DEMOS01\`

	fmt.Println("=================================================================")
	fmt.Println("  VALIDACION PROFUNDA - Comparacion Hex vs Parsed")
	fmt.Println("=================================================================")

	validateZ17(dataPath)
	validateZ04(dataPath)
	validateZ49(dataPath)
	validateZ09(dataPath)
	validateZ11(dataPath)
	validateZ08A(dataPath)
	validateZ18(dataPath)
	validateZ05(dataPath)
	validateZ03(dataPath)
	validateZ27(dataPath)
	validateZ25(dataPath)
	validateZ28(dataPath)
	validateZ07(dataPath)
	validateZ07T(dataPath)
	validateZ26(dataPath)
	validateZDANE(dataPath)
	validateZICA(dataPath)
	validateZPILA(dataPath)

	fmt.Println("\n=================================================================")
	fmt.Println("  VALIDACION PROFUNDA COMPLETA")
	fmt.Println("=================================================================")
}

func hexSlice(rec []byte, start, length int) string {
	if start+length > len(rec) {
		length = len(rec) - start
	}
	if length <= 0 {
		return "<out of bounds>"
	}
	hex := ""
	for i := start; i < start+length; i++ {
		hex += fmt.Sprintf("%02X ", rec[i])
	}
	return strings.TrimSpace(hex)
}

func asciiSlice(rec []byte, start, length int) string {
	if start+length > len(rec) {
		length = len(rec) - start
	}
	if length <= 0 {
		return "<out of bounds>"
	}
	s := ""
	for i := start; i < start+length; i++ {
		if rec[i] >= 0x20 && rec[i] <= 0x7E {
			s += string(rec[i])
		} else {
			s += "."
		}
	}
	return s
}

func readFile(path string) [][]byte {
	records, _, err := isam.ReadIsamFile(path)
	if err != nil {
		fmt.Printf("  ERROR leyendo %s: %v\n", path, err)
		return nil
	}
	return records
}

func printRecordContext(rec []byte, label string, offset, length int) {
	if offset >= len(rec) {
		fmt.Printf("    %s: <offset %d beyond record len %d>\n", label, offset, len(rec))
		return
	}
	fmt.Printf("    %s: hex[%d:%d] = %s\n", label, offset, offset+length, hexSlice(rec, offset, length))
	fmt.Printf("      ascii = |%s|\n", asciiSlice(rec, offset, length))
}

func hasGarbage(s string) bool {
	for _, c := range s {
		if c < 0x20 && c != 0 {
			return true
		}
	}
	return false
}

func validateZ17(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z17 (TERCEROS/CLIENTES)")
	fmt.Println("--------------------------------------------------")
	path := dataPath + "Z17"
	records := readFile(path)
	if records == nil {
		return
	}

	terceros, _ := parsers.ParseTerceros(dataPath)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(terceros))

	issues := 0
	for i := 0; i < len(terceros); i++ {
		t := terceros[i]
		if hasGarbage(t.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, t.Nombre)
			issues++
		}
		for _, c := range t.NumeroDoc {
			if (c < '0' || c > '9') && c != '-' {
				fmt.Printf("  ISSUE: rec[%d] numDoc non-numeric: %q nombre:%s\n", i, t.NumeroDoc, t.Nombre)
				issues++
				break
			}
		}
		if t.FechaCreacion != "" && len(t.FechaCreacion) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fecha bad length: %q\n", i, t.FechaCreacion)
			issues++
		}
		if len(t.Nombre) < 3 && t.Nombre != "" {
			fmt.Printf("  WARN: rec[%d] nombre very short: %q\n", i, t.Nombre)
		}
	}

	for i := 0; i < 5 && i < len(terceros); i++ {
		idx := i * len(terceros) / 5
		t := terceros[idx]
		fmt.Printf("  [%3d] tipo:%s emp:%s cod:%s doc:%s fecha:%s | %s\n",
			idx, t.TipoClave, t.Empresa, t.Codigo, t.NumeroDoc, t.FechaCreacion, t.Nombre)
	}

	if len(records) > 0 {
		rec := records[0]
		fmt.Println("\n  Primer registro raw:")
		printRecordContext(rec, "tipo@0(1)", 0, 1)
		printRecordContext(rec, "empresa@1(3)", 1, 3)
		printRecordContext(rec, "tipoDoc@18(2)", 18, 2)
		printRecordContext(rec, "nombre@36(40)", 36, 40)
		printRecordContext(rec, "fecha@28(8)", 28, 8)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ04(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z04 (INVENTARIO/PRODUCTOS)")
	fmt.Println("--------------------------------------------------")
	path, _ := parsers.FindLatestZ04(dataPath)
	if path == "" {
		fmt.Println("  No Z04 file found")
		return
	}
	records := readFile(path)
	if records == nil {
		return
	}

	productos, _, _ := parsers.ParseInventario(dataPath)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(productos))

	issues := 0
	for i := 0; i < len(productos); i++ {
		p := productos[i]
		if hasGarbage(p.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, p.Nombre)
			issues++
		}
		if hasGarbage(p.Codigo) {
			fmt.Printf("  ISSUE: rec[%d] codigo garbage: %q\n", i, p.Codigo)
			issues++
		}
	}

	for i := 0; i < 5 && i < len(productos); i++ {
		idx := i * len(productos) / 5
		p := productos[idx]
		fmt.Printf("  [%3d] cod:%s grp:%s ref:%s corto:%s | %s\n", idx, p.Codigo, p.Grupo, p.Referencia, p.NombreCorto, p.Nombre)
	}

	if len(records) > 0 {
		rec := records[0]
		fmt.Println("\n  Primer registro raw:")
		printRecordContext(rec, "empresa@0(5)", 0, 5)
		printRecordContext(rec, "grupo@5(3)", 5, 3)
		printRecordContext(rec, "codigo@8(6)", 8, 6)
		printRecordContext(rec, "nombre@14(50)", 14, 50)
		printRecordContext(rec, "nombreCorto@64(40)", 64, 40)
		printRecordContext(rec, "referencia@104(30)", 104, 30)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ49(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z49 (MOVIMIENTOS)")
	fmt.Println("--------------------------------------------------")
	path := dataPath + "Z49"
	records := readFile(path)
	if records == nil {
		return
	}

	movs, _ := parsers.ParseMovimientos(dataPath)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(movs))

	issues := 0
	garbageCount := 0
	shortNames := 0
	emptyDescs := 0

	for _, m := range movs {
		if hasGarbage(m.NombreTercero) {
			garbageCount++
			if garbageCount <= 3 {
				fmt.Printf("  ISSUE: nombreTercero garbage: %q (tipo:%s)\n", m.NombreTercero, m.TipoComprobante)
			}
		}
		if hasGarbage(m.Descripcion) {
			garbageCount++
			if garbageCount <= 3 {
				fmt.Printf("  ISSUE: desc garbage: %q\n", m.Descripcion)
			}
		}
		if len(m.NombreTercero) > 0 && len(m.NombreTercero) < 5 {
			shortNames++
		}
		if m.Descripcion == "" && m.Descripcion2 == "" {
			emptyDescs++
		}
	}
	issues += garbageCount

	fmt.Printf("  Nombres cortos (<5 chars): %d/%d\n", shortNames, len(movs))
	fmt.Printf("  Sin descripcion: %d/%d\n", emptyDescs, len(movs))
	fmt.Printf("  Con garbage en texto: %d\n", garbageCount)

	for i := 0; i < 5 && i < len(movs); i++ {
		idx := i * len(movs) / 5
		m := movs[idx]
		fmt.Printf("  [%4d] tipo:%-6s num:%-8s nombre:%-40s | d1:%s | d2:%s\n",
			idx, m.TipoComprobante, m.NumeroDoc,
			truncStr(m.NombreTercero, 40),
			truncStr(m.Descripcion, 50),
			truncStr(m.Descripcion2, 30))
	}

	for i := 0; i < 3 && i < len(records); i++ {
		idx := i * len(records) / 3
		rec := records[idx]
		if len(rec) < 72 {
			continue
		}
		fmt.Printf("\n  Raw[%d] (len=%d):\n", idx, len(rec))
		printRecordContext(rec, "tipo@0(1)", 0, 1)
		printRecordContext(rec, "codigo@1(3)", 1, 3)
		printRecordContext(rec, "numDoc@4(11)", 4, 11)
		printRecordContext(rec, "nombre@15(57)", 15, 57)
		if len(rec) > 128 {
			printRecordContext(rec, "desc1@72(56)", 72, 56)
		}
		if len(rec) > 189 {
			printRecordContext(rec, "desc2@129(60)", 129, 60)
		}
	}

	if issues == 0 {
		fmt.Println("\n  OK: Sin problemas detectados")
	} else {
		fmt.Printf("\n  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ09(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z09 (CARTERA)")
	fmt.Println("--------------------------------------------------")
	_, year := parsers.FindLatestZ09(dataPath)
	if year == "" {
		fmt.Println("  No Z09 file found")
		return
	}
	path := dataPath + "Z09" + year
	records := readFile(path)
	if records == nil {
		return
	}

	cartera, _ := parsers.ParseCartera(dataPath, year)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(cartera))

	issues := 0
	badNits := 0
	badCuentas := 0
	badFechas := 0
	badDescs := 0
	truncDescs := 0

	for i, c := range cartera {
		for _, ch := range c.NitTercero {
			if ch < '0' || ch > '9' {
				badNits++
				if badNits <= 3 {
					fmt.Printf("  ISSUE: NIT non-numeric: %q (tipo:%s desc:%s)\n", c.NitTercero, c.TipoRegistro, c.Descripcion)
				}
				break
			}
		}
		if c.CuentaContable != "" && len(c.CuentaContable) >= 4 && (c.CuentaContable[0] < '0' || c.CuentaContable[0] > '9') {
			badCuentas++
			if badCuentas <= 3 {
				fmt.Printf("  ISSUE: rec[%d] cuenta invalid: %q\n", i, c.CuentaContable)
			}
		}
		if c.Fecha != "" && len(c.Fecha) != 8 {
			badFechas++
		}
		if hasGarbage(c.Descripcion) {
			badDescs++
		}
		if len(c.Descripcion) == 50 {
			last := c.Descripcion[len(c.Descripcion)-1]
			if (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z') {
				truncDescs++
			}
		}
	}

	issues += badNits + badCuentas + badFechas + badDescs
	fmt.Printf("  NITs non-numeric: %d, Cuentas invalid: %d, Fechas bad: %d\n", badNits, badCuentas, badFechas)
	fmt.Printf("  Desc garbage: %d, Desc possibly truncated @50: %d\n", badDescs, truncDescs)

	for i := 0; i < 5 && i < len(cartera); i++ {
		idx := i * len(cartera) / 5
		c := cartera[idx]
		fmt.Printf("  [%4d] tipo:%s nit:%-13s cuenta:%-13s fecha:%s D/C:%s | %s\n",
			idx, c.TipoRegistro, c.NitTercero, c.CuentaContable, c.Fecha, c.TipoMov, c.Descripcion)
	}

	for i := 0; i < 3 && i < len(records); i++ {
		idx := i * len(records) / 3
		rec := records[idx]
		if len(rec) < 144 {
			continue
		}
		fmt.Printf("\n  Raw[%d] (len=%d):\n", idx, len(rec))
		printRecordContext(rec, "tipo@0(1)", 0, 1)
		printRecordContext(rec, "nit@16(13)", 16, 13)
		printRecordContext(rec, "cuenta@29(13)", 29, 13)
		printRecordContext(rec, "fecha@42(8)", 42, 8)
		printRecordContext(rec, "desc@93(50)", 93, 50)
		printRecordContext(rec, "D/C@143(1)", 143, 1)
	}

	if issues == 0 {
		fmt.Println("\n  OK: Sin problemas detectados")
	} else {
		fmt.Printf("\n  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ11(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z11 (DOCUMENTOS)")
	fmt.Println("--------------------------------------------------")
	path, year := parsers.FindLatestZ11(dataPath)
	if path == "" {
		fmt.Println("  No Z11 file found")
		return
	}
	records := readFile(path)
	if records == nil {
		return
	}

	docs, _, _ := parsers.ParseDocumentosFile(path, year)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(docs))

	issues := 0
	for i, d := range docs {
		for _, ch := range d.NitTercero {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: doc[%d] NIT non-numeric: %q desc:%s\n", i, d.NitTercero, d.Descripcion)
				issues++
				break
			}
		}
		if d.CuentaContable != "" && len(d.CuentaContable) >= 4 && (d.CuentaContable[0] < '0' || d.CuentaContable[0] > '9') {
			fmt.Printf("  ISSUE: doc[%d] cuenta invalid: %q\n", i, d.CuentaContable)
			issues++
		}
		if hasGarbage(d.Descripcion) {
			fmt.Printf("  ISSUE: doc[%d] desc garbage: %q\n", i, d.Descripcion)
			issues++
		}
		if d.TipoMov != "D" && d.TipoMov != "C" && d.TipoMov != "" {
			fmt.Printf("  ISSUE: doc[%d] D/C invalid: %q\n", i, d.TipoMov)
			issues++
		}
	}

	for i, d := range docs {
		if i >= 10 {
			break
		}
		fmt.Printf("  [%2d] tipo:%s cod:%s seq:%s nit:%-13s cuenta:%-13s fecha:%s D/C:%s | %s\n",
			i, d.TipoComprobante, d.CodigoComp, d.Secuencia, d.NitTercero, d.CuentaContable, d.Fecha, d.TipoMov, d.Descripcion)
	}

	for i := 0; i < 3 && i < len(records); i++ {
		rec := records[i]
		if len(rec) < 144 {
			continue
		}
		fmt.Printf("\n  Raw[%d] (len=%d):\n", i, len(rec))
		printRecordContext(rec, "tipo@0(1)", 0, 1)
		printRecordContext(rec, "codigo@1(3)", 1, 3)
		printRecordContext(rec, "seq@10(5)", 10, 5)
		printRecordContext(rec, "tipoDoc@26(1)", 26, 1)
		printRecordContext(rec, "nit@27(13)", 27, 13)
		printRecordContext(rec, "cuenta@40(13)", 40, 13)
		printRecordContext(rec, "fecha@53(8)", 53, 8)
		printRecordContext(rec, "desc@93(50)", 93, 50)
		printRecordContext(rec, "D/C@143(1)", 143, 1)
		printRecordContext(rec, "ref@167(7)", 167, 7)
	}

	if issues == 0 {
		fmt.Println("\n  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ08A(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z08A (TERCEROS AMPLIADOS)")
	fmt.Println("--------------------------------------------------")
	matches, _ := filepath.Glob(dataPath + "Z08*A")
	if len(matches) == 0 {
		fmt.Println("  No Z08A file found")
		return
	}
	path := matches[0]
	records := readFile(path)
	if records == nil {
		return
	}

	ampliados, _, _ := parsers.ParseTercerosAmpliados(dataPath)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(ampliados))

	issues := 0
	for i, a := range ampliados {
		for _, ch := range a.Nit {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q nombre:%s\n", i, a.Nit, a.Nombre)
				issues++
				break
			}
		}
		if hasGarbage(a.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, a.Nombre)
			issues++
		}
		if a.Email != "" && !strings.Contains(a.Email, "@") && len(a.Email) > 3 {
			fmt.Printf("  WARN: rec[%d] email no @: %q nombre:%s\n", i, a.Email, a.Nombre)
		}
		if hasGarbage(a.Direccion) {
			fmt.Printf("  ISSUE: rec[%d] dir garbage: %q\n", i, a.Direccion)
			issues++
		}
		if len(a.Nombre) < 3 && a.Nombre != "" {
			fmt.Printf("  WARN: rec[%d] nombre very short: %q nit:%s\n", i, a.Nombre, a.Nit)
		}
	}

	for i := 0; i < 5 && i < len(ampliados); i++ {
		idx := i * len(ampliados) / 5
		a := ampliados[idx]
		fmt.Printf("  [%2d] emp:%s nit:%-12s tipo:%s dir:%-30s email:%-30s | %s\n",
			idx, a.Empresa, a.Nit, a.TipoPersona,
			truncStr(a.Direccion, 30), truncStr(a.Email, 30), a.Nombre)
	}

	for i := 0; i < 3 && i < len(records); i++ {
		idx := i * len(records) / 3
		rec := records[idx]
		fmt.Printf("\n  Raw[%d] (len=%d):\n", idx, len(rec))
		printRecordContext(rec, "empresa@0(3)", 0, 3)
		printRecordContext(rec, "nit@3(10)", 3, 10)
		printRecordContext(rec, "tipoPersona@16(2)", 16, 2)
		printRecordContext(rec, "nombre@18(60)", 18, 60)
		if len(rec) > 156 {
			printRecordContext(rec, "repLegal@96(60)", 96, 60)
		}
		if len(rec) > 250 {
			printRecordContext(rec, "dir@194(56)", 194, 56)
		}
		if len(rec) > 393 {
			printRecordContext(rec, "email@323(70)", 323, 70)
		}
	}

	if issues == 0 {
		fmt.Println("\n  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ18(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z18 (HISTORIAL DOCUMENTOS)")
	fmt.Println("--------------------------------------------------")
	path, year := parsers.FindLatestZ18(dataPath)
	if path == "" {
		fmt.Println("  No Z18 file found")
		return
	}
	records := readFile(path)
	if records == nil {
		return
	}

	docs, _, _ := parsers.ParseHistorialFile(path, year)
	fmt.Printf("  Registros raw: %d, Parseados: %d\n", len(records), len(docs))

	issues := 0
	for i, d := range docs {
		if hasGarbage(d.NombreOrigen) {
			fmt.Printf("  ISSUE: rec[%d] nombre1 garbage: %q\n", i, d.NombreOrigen)
			issues++
		}
		if hasGarbage(d.NombreDestin) {
			fmt.Printf("  ISSUE: rec[%d] nombre2 garbage: %q\n", i, d.NombreDestin)
			issues++
		}
		if len(d.NombreDestin) > 0 && d.NombreDestin[0] >= '0' && d.NombreDestin[0] <= '9' {
			fmt.Printf("  WARN: rec[%d] nombre2 starts with digit: %q (posible offset mal)\n", i, d.NombreDestin)
		}
	}

	for i, d := range docs {
		fmt.Printf("  [%2d] tipo:%s sub:%s emp:%s fecha:%s nit:%s | %s / %s\n",
			i, d.TipoRegistro, d.SubTipo, d.Empresa, d.Fecha, d.NitOrigen, d.NombreOrigen, d.NombreDestin)
	}

	for i := 0; i < 3 && i < len(records); i++ {
		rec := records[i]
		if len(rec) < 100 {
			continue
		}
		fmt.Printf("\n  Raw[%d] (len=%d):\n", i, len(rec))
		printRecordContext(rec, "tipo@0(1)", 0, 1)
		printRecordContext(rec, "sub@1(2)", 1, 2)
		printRecordContext(rec, "empresa@3(3)", 3, 3)
		printRecordContext(rec, "fecha@63(8)", 63, 8)
		printRecordContext(rec, "nombre1@77(40)", 77, 40)
		printRecordContext(rec, "nit@137(13)", 137, 13)
		if len(rec) > 210 {
			printRecordContext(rec, "pre-nombre2@161(4)", 161, 4)
			printRecordContext(rec, "nombre2@165(40)", 165, 40)
		}
	}

	if issues == 0 {
		fmt.Println("\n  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ05(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z05 (CONDICIONES DE PAGO)")
	fmt.Println("--------------------------------------------------")
	conds, year, err := parsers.ParseCondicionesPago(dataPath)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}

	pattern := filepath.Join(dataPath, "Z05[0-9][0-9][0-9][0-9]")
	matches, _ := filepath.Glob(pattern)
	var records [][]byte
	for _, m := range matches {
		if !strings.HasSuffix(m, ".idx") {
			recs := readFile(m)
			if recs != nil {
				records = recs
			}
		}
	}

	fmt.Printf("  Registros parseados: %d (año: %s), raw: %d\n", len(conds), year, len(records))

	issues := 0
	valoresPositivos := 0
	valoresAbsurdos := 0

	for i, c := range conds {
		for _, ch := range c.NIT {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q\n", i, c.NIT)
				issues++
				break
			}
		}
		if c.Valor > 0 {
			valoresPositivos++
		}
		if math.Abs(c.Valor) > 1e12 {
			valoresAbsurdos++
			fmt.Printf("  ISSUE: rec[%d] valor absurdo: %.2f\n", i, c.Valor)
			issues++
		}
		if c.Fecha != "" && len(c.Fecha) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fecha bad: %q\n", i, c.Fecha)
			issues++
		}
	}

	fmt.Printf("  Valores positivos: %d/%d, absurdos: %d\n", valoresPositivos, len(conds), valoresAbsurdos)

	for i := 0; i < len(conds) && i < 8; i++ {
		c := conds[i]
		fmt.Printf("  [%d] tipo:%s nit:%-13s seq:%s fecha:%s valor:%15.2f flag:%s tipoSec:%s fechaReg:%s\n",
			i, c.Tipo, c.NIT, c.Secuencia, c.Fecha, c.Valor, c.FlagByte, c.TipoSecundario, c.FechaRegistro)
	}

	for i := 0; i < 3 && i < len(records); i++ {
		rec := records[i]
		if len(rec) < 220 {
			continue
		}
		fmt.Printf("\n  Raw[%d] (len=%d) BCD area:\n", i, len(rec))
		printRecordContext(rec, "nit@27(13)", 27, 13)
		printRecordContext(rec, "BCD@205(14)", 205, 14)
		printRecordContext(rec, "BCD@208(7)", 208, 7)
		printRecordContext(rec, "fechaReg@224(8)", 224, 8)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ03(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z03 (PLAN DE CUENTAS)")
	fmt.Println("--------------------------------------------------")
	cuentas, _, _ := parsers.ParsePlanCuentas(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(cuentas))

	issues := 0
	for i, c := range cuentas {
		if hasGarbage(c.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, c.Nombre)
			issues++
		}
		for _, ch := range c.CodigoCuenta {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] cuenta non-numeric: %q nombre:%s\n", i, c.CodigoCuenta, c.Nombre)
				issues++
				break
			}
		}
	}

	for i := 0; i < 5 && i < len(cuentas); i++ {
		idx := i * len(cuentas) / 5
		c := cuentas[idx]
		fmt.Printf("  [%4d] emp:%s cuenta:%s act:%v aux:%v nat:%s | %s\n",
			idx, c.Empresa, c.CodigoCuenta, c.Activa, c.Auxiliar, c.Naturaleza, c.Nombre)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ27(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z27 (ACTIVOS FIJOS)")
	fmt.Println("--------------------------------------------------")
	activos, _, _ := parsers.ParseActivosFijos(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(activos))

	issues := 0
	for i, a := range activos {
		if hasGarbage(a.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, a.Nombre)
			issues++
		}
		for _, ch := range a.NitResponsable {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q\n", i, a.NitResponsable)
				issues++
				break
			}
		}
	}

	for i, a := range activos {
		if i >= 8 {
			break
		}
		fmt.Printf("  [%d] emp:%s cod:%s nit:%s fecha:%s | %s\n",
			i, a.Empresa, a.Codigo, a.NitResponsable, a.FechaAdquisicion, a.Nombre)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ25(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z25 (SALDOS TERCEROS)")
	fmt.Println("--------------------------------------------------")

	matches, _ := filepath.Glob(dataPath + "Z25[0-9][0-9][0-9][0-9]")
	var rawPath string
	for _, m := range matches {
		if !strings.HasSuffix(m, ".idx") {
			rawPath = m
		}
	}

	saldos, _, _ := parsers.ParseSaldosTerceros(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(saldos))

	var records [][]byte
	if rawPath != "" {
		records = readFile(rawPath)
	}

	issues := 0
	absurdValues := 0
	for i, s := range saldos {
		for _, ch := range s.NitTercero {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q cuenta:%s\n", i, s.NitTercero, s.CuentaContable)
				issues++
				break
			}
		}
		if math.Abs(s.SaldoAnterior) > 1e12 || math.Abs(s.Debito) > 1e12 || math.Abs(s.Credito) > 1e12 {
			absurdValues++
			if absurdValues <= 3 {
				fmt.Printf("  ISSUE: rec[%d] absurd BCD: ant=%.2f deb=%.2f cred=%.2f\n",
					i, s.SaldoAnterior, s.Debito, s.Credito)
			}
			issues++
		}
	}

	fmt.Printf("  Valores absurdos (>1T): %d\n", absurdValues)

	for i := 0; i < 5 && i < len(saldos); i++ {
		idx := i * len(saldos) / 5
		s := saldos[idx]
		fmt.Printf("  [%3d] emp:%s cuenta:%s nit:%-13s ant:%15.2f deb:%15.2f cred:%15.2f final:%15.2f\n",
			idx, s.Empresa, s.CuentaContable, s.NitTercero, s.SaldoAnterior, s.Debito, s.Credito, s.SaldoFinal)
	}

	if len(records) > 0 {
		for i := 0; i < 2 && i < len(records); i++ {
			rec := records[i]
			if len(rec) < 165 {
				continue
			}
			fmt.Printf("\n  Raw[%d] (len=%d) BCD zone:\n", i, len(rec))
			printRecordContext(rec, "empresa@0(3)", 0, 3)
			printRecordContext(rec, "cuenta@3(9)", 3, 9)
			printRecordContext(rec, "nit@12(13)", 12, 13)
			printRecordContext(rec, "BCD-saldo@140(8)", 140, 8)
			printRecordContext(rec, "BCD-debito@148(8)", 148, 8)
			printRecordContext(rec, "BCD-credito@156(8)", 156, 8)
		}
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ28(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z28 (SALDOS CONSOLIDADOS)")
	fmt.Println("--------------------------------------------------")

	matches, _ := filepath.Glob(dataPath + "Z28[0-9][0-9][0-9][0-9]")
	var rawPath string
	for _, m := range matches {
		if !strings.HasSuffix(m, ".idx") {
			rawPath = m
		}
	}

	saldos, _, _ := parsers.ParseSaldosConsolidados(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(saldos))

	var records [][]byte
	if rawPath != "" {
		records = readFile(rawPath)
	}

	issues := 0
	absurdValues := 0
	for i, s := range saldos {
		if math.Abs(s.SaldoAnterior) > 1e12 || math.Abs(s.Debito) > 1e12 || math.Abs(s.Credito) > 1e12 {
			absurdValues++
			if absurdValues <= 3 {
				fmt.Printf("  ISSUE: rec[%d] absurd BCD: ant=%.2f deb=%.2f cred=%.2f\n",
					i, s.SaldoAnterior, s.Debito, s.Credito)
			}
			issues++
		}
	}

	fmt.Printf("  Valores absurdos (>1T): %d\n", absurdValues)

	for i := 0; i < 5 && i < len(saldos); i++ {
		idx := i * len(saldos) / 5
		s := saldos[idx]
		fmt.Printf("  [%3d] emp:%s cuenta:%s ant:%15.2f deb:%15.2f cred:%15.2f final:%15.2f\n",
			idx, s.Empresa, s.CuentaContable, s.SaldoAnterior, s.Debito, s.Credito, s.SaldoFinal)
	}

	if len(records) > 0 {
		for i := 0; i < 2 && i < len(records); i++ {
			rec := records[i]
			if len(rec) < 62 {
				continue
			}
			fmt.Printf("\n  Raw[%d] (len=%d) BCD zone:\n", i, len(rec))
			printRecordContext(rec, "empresa@0(3)", 0, 3)
			printRecordContext(rec, "cuenta@3(9)", 3, 9)
			printRecordContext(rec, "BCD-saldo@38(8)", 38, 8)
			printRecordContext(rec, "BCD-debito@46(8)", 46, 8)
			printRecordContext(rec, "BCD-credito@54(8)", 54, 8)
		}
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ07(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z07 (LIBROS AUXILIARES)")
	fmt.Println("--------------------------------------------------")
	libros, _, _ := parsers.ParseLibrosAuxiliares(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(libros))

	issues := 0
	absurdValues := 0
	for i, l := range libros {
		for _, ch := range l.NitTercero {
			if ch < '0' || ch > '9' {
				fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q\n", i, l.NitTercero)
				issues++
				break
			}
		}
		if math.Abs(l.Saldo) > 1e12 || math.Abs(l.Debito) > 1e12 || math.Abs(l.Credito) > 1e12 {
			absurdValues++
			if absurdValues <= 3 {
				fmt.Printf("  ISSUE: rec[%d] absurd BCD: saldo=%.2f deb=%.2f cred=%.2f\n",
					i, l.Saldo, l.Debito, l.Credito)
			}
			issues++
		}
		if l.FechaDocumento != "" && len(l.FechaDocumento) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fechaDoc bad: %q\n", i, l.FechaDocumento)
			issues++
		}
		if l.FechaRegistro != "" && len(l.FechaRegistro) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fechaReg bad: %q\n", i, l.FechaRegistro)
			issues++
		}
	}

	fmt.Printf("  Valores absurdos: %d\n", absurdValues)

	for i := 0; i < 5 && i < len(libros); i++ {
		idx := i * len(libros) / 5
		l := libros[idx]
		fmt.Printf("  [%3d] emp:%s cuenta:%s nit:%-13s tipo:%s-%s fechaDoc:%s saldo:%12.2f deb:%12.2f cred:%12.2f\n",
			idx, l.Empresa, l.CuentaContable, l.NitTercero,
			l.TipoComprobante, l.CodigoComprobante, l.FechaDocumento, l.Saldo, l.Debito, l.Credito)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ07T(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z07T (TRANSACCIONES DETALLE)")
	fmt.Println("--------------------------------------------------")
	trans, _ := parsers.ParseTransaccionesDetalle(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(trans))

	issues := 0
	absurdValues := 0
	badDC := 0
	for i, t := range trans {
		for _, ch := range t.NitTercero {
			if ch < '0' || ch > '9' {
				if issues < 3 {
					fmt.Printf("  ISSUE: rec[%d] NIT non-numeric: %q\n", i, t.NitTercero)
				}
				issues++
				break
			}
		}
		if math.Abs(t.Valor) > 1e12 {
			absurdValues++
			issues++
		}
		if t.TipoMovimiento != "D" && t.TipoMovimiento != "C" && t.TipoMovimiento != "" {
			badDC++
			if badDC <= 3 {
				fmt.Printf("  ISSUE: rec[%d] D/C invalid: %q\n", i, t.TipoMovimiento)
			}
			issues++
		}
	}

	fmt.Printf("  Valores absurdos: %d, D/C invalidos: %d\n", absurdValues, badDC)

	for i := 0; i < 5 && i < len(trans); i++ {
		idx := i * len(trans) / 5
		t := trans[idx]
		fmt.Printf("  [%3d] tipo:%s nit:%-14s cuenta:%s fecha:%s D/C:%s valor:%15.2f\n",
			idx, t.TipoComprobante, t.NitTercero, t.CuentaContable, t.FechaDocumento, t.TipoMovimiento, t.Valor)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZ26(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("Z26 (PERIODOS)")
	fmt.Println("--------------------------------------------------")
	periodos, _, _ := parsers.ParsePeriodos(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(periodos))

	issues := 0
	for i, p := range periodos {
		if p.FechaInicio != "" && len(p.FechaInicio) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fechaInicio bad: %q\n", i, p.FechaInicio)
			issues++
		}
		if p.FechaFin != "" && len(p.FechaFin) != 8 {
			fmt.Printf("  ISSUE: rec[%d] fechaFin bad: %q\n", i, p.FechaFin)
			issues++
		}
		if math.Abs(p.Saldo1) > 1e12 {
			fmt.Printf("  ISSUE: rec[%d] saldo1 absurd: %.2f\n", i, p.Saldo1)
			issues++
		}
	}

	for i := 0; i < len(periodos) && i < 10; i++ {
		p := periodos[i]
		fmt.Printf("  [%2d] emp:%s num:%s ini:%s fin:%s est:%s saldo1:%15.2f saldo2:%15.2f saldo3:%15.2f\n",
			i, p.Empresa, p.NumeroPeriodo, p.FechaInicio, p.FechaFin, p.Estado, p.Saldo1, p.Saldo2, p.Saldo3)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZDANE(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("ZDANE (MUNICIPIOS)")
	fmt.Println("--------------------------------------------------")
	munis, _ := parsers.ParseDane(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(munis))

	issues := 0
	for i, m := range munis {
		if hasGarbage(m.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, m.Nombre)
			issues++
		}
		if len(m.Codigo) != 5 {
			fmt.Printf("  ISSUE: rec[%d] codigo bad length %d: %q\n", i, len(m.Codigo), m.Codigo)
			issues++
		}
	}

	for i := 0; i < 5 && i < len(munis); i++ {
		idx := i * len(munis) / 5
		fmt.Printf("  [%4d] cod:%s | %s\n", idx, munis[idx].Codigo, munis[idx].Nombre)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZICA(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("ZICA (ACTIVIDADES ICA)")
	fmt.Println("--------------------------------------------------")
	acts, _ := parsers.ParseICA(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(acts))

	issues := 0
	truncated := 0
	for i, a := range acts {
		if hasGarbage(a.Nombre) {
			fmt.Printf("  ISSUE: rec[%d] nombre garbage: %q\n", i, a.Nombre)
			issues++
		}
		if len(a.Nombre) == 50 {
			truncated++
		}
	}

	fmt.Printf("  Nombres truncados @50 chars: %d/%d (COBOL limit)\n", truncated, len(acts))

	for i := 0; i < 5 && i < len(acts); i++ {
		idx := i * len(acts) / 5
		fmt.Printf("  [%3d] cod:%s tarifa:%s | %s\n", idx, acts[idx].Codigo, acts[idx].Tarifa, acts[idx].Nombre)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func validateZPILA(dataPath string) {
	fmt.Println("\n--------------------------------------------------")
	fmt.Println("ZPILA (SEGURIDAD SOCIAL)")
	fmt.Println("--------------------------------------------------")
	items, _ := parsers.ParsePILA(dataPath)
	fmt.Printf("  Registros parseados: %d\n", len(items))

	issues := 0
	for i, p := range items {
		if hasGarbage(p.Tipo) {
			fmt.Printf("  ISSUE: rec[%d] tipo garbage: %q\n", i, p.Tipo)
			issues++
		}
		if hasGarbage(p.Fondo) {
			fmt.Printf("  ISSUE: rec[%d] fondo garbage: %q\n", i, p.Fondo)
			issues++
		}
	}

	fondos := map[string]int{}
	tipos := map[string]int{}
	for _, p := range items {
		fondos[p.Fondo]++
		tipos[p.Tipo]++
	}
	fmt.Printf("  Fondos: %v\n", fondos)
	fmt.Printf("  Tipos: %v\n", tipos)

	for i := 0; i < 5 && i < len(items); i++ {
		p := items[i]
		fmt.Printf("  [%d] tipo:%-10s fondo:%-4s conc:%-3s flags:%s base:%s calc:%s\n",
			i, p.Tipo, p.Fondo, p.Concepto, p.Flags, p.TipoBase, p.BaseCalculo)
	}

	if issues == 0 {
		fmt.Println("  OK: Sin problemas detectados")
	} else {
		fmt.Printf("  TOTAL ISSUES: %d\n", issues)
	}
}

func truncStr(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

var _ = os.Stat
