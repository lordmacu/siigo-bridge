package main

import (
	"fmt"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\SIIWI02\`

	// === THIRD PARTIES ===
	fmt.Println("=== THIRD PARTIES (Z17) - DETAIL ===")
	all, err := parsers.ParseTerceros(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total third parties: %d\n", len(all))

	clientes, _ := parsers.ParseTercerosClientes(dataPath)
	fmt.Printf("Clients (G): %d\n", len(clientes))

	// Validate fields
	emptyNombre, emptyDoc, emptyTipo := 0, 0, 0
	tiposClave := map[string]int{}
	tiposDoc := map[string]int{}
	for _, t := range all {
		if t.Name == "" { emptyNombre++ }
		if t.DocNumber == "" { emptyDoc++ }
		if t.KeyType == "" { emptyTipo++ }
		tiposClave[t.KeyType]++
		tiposDoc[t.DocType]++
	}
	fmt.Printf("Empty fields: name=%d, document=%d, key_type=%d\n", emptyNombre, emptyDoc, emptyTipo)
	fmt.Println("Key types:", tiposClave)
	fmt.Println("Doc types:", tiposDoc)

	fmt.Println("\nFirst 10 third parties:")
	for i, t := range all {
		if i >= 10 { break }
		fmt.Printf("  [%d] key:%s comp:%s doc_type:%s doc:%-13s date:%s | %s\n",
			i+1, t.KeyType, t.Company, t.DocType, t.DocNumber, t.CreationDate, t.Name)
		if t.PreferredAcctType != "" {
			fmt.Printf("       acct_type:%s\n", t.PreferredAcctType)
		}
	}
	fmt.Println()

	// === MOVEMENTS (Z49 = document headers, NOT detailed transactions) ===
	fmt.Println("=== MOVEMENTS (Z49) - DOCUMENT HEADERS ===")
	fmt.Println("NOTE: Z49 only contains type+number+name+description. Does NOT have dates/accounts/values.")
	movs, err := parsers.ParseMovimientos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total: %d\n", len(movs))

	emptyTipoC, emptyNumDoc, emptyDesc, emptyNombre := 0, 0, 0, 0
	tiposComp := map[string]int{}
	for _, m := range movs {
		if m.VoucherType == "" { emptyTipoC++ }
		if m.DocNumber == "" { emptyNumDoc++ }
		if m.Description == "" && m.Description2 == "" { emptyDesc++ }
		if m.ThirdPartyName == "" { emptyNombre++ }
		if m.VoucherType != "" {
			tiposComp[m.VoucherType[:min(2, len(m.VoucherType))]]++
		}
	}
	fmt.Printf("Campos vacios: tipo_comp=%d, num_doc=%d, descripcion=%d, nombre=%d\n",
		emptyTipoC, emptyNumDoc, emptyDesc, emptyNombre)
	fmt.Println("Tipos comprobante:", tiposComp)

	// Show first 10 records WITH tipo_comprobante (skip space/0-type index records)
	fmt.Println("\nFirst 10 movimientos con comprobante:")
	shown := 0
	for _, m := range movs {
		if shown >= 10 { break }
		if m.VoucherType == "" { continue }
		shown++
		desc := m.Description
		if m.Description2 != "" {
			if desc != "" {
				desc += " | " + m.Description2
			} else {
				desc = m.Description2
			}
		}
		fmt.Printf("  [%d] tipo:%-6s doc:%-11s nombre:%-25s | %s\n",
			shown, m.VoucherType, m.DocNumber, m.ThirdPartyName, desc)
	}
	fmt.Println()

	// === PRODUCTOS (INVENTARIO) ===
	fmt.Println("=== PRODUCTOS (Z04 - Inventario) - DETALLE ===")
	prods, year, err := parsers.ParseInventario(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("File: Z04%s, Total: %d\n", year, len(prods))
	for _, p := range prods {
		fmt.Printf("  cod:%-8s grupo:%s emp:%s | %-40s | corto:%-30s ref:%s\n",
			p.Code, p.Group, p.Company, p.Name, p.ShortName, p.Reference)
	}
	fmt.Println()

	// === CARTERA ===
	for _, anio := range []string{"2013", "2014"} {
		fmt.Printf("=== CARTERA (Z09%s) - DETALLE ===\n", anio)
		cart, err := parsers.ParseCartera(dataPath, anio)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		fmt.Printf("Total: %d\n", len(cart))

		emptyTipoR, emptyEmp, emptySec, emptyNitC := 0, 0, 0, 0
		ctypes := map[string]int{}
		dmov := map[string]int{}
		for _, c := range cart {
			if c.RecordType == "" { emptyTipoR++ }
			if c.Company == "" { emptyEmp++ }
			if c.Sequence == "" { emptySec++ }
			if c.ThirdPartyNit == "" { emptyNitC++ }
			ctypes[c.RecordType]++
			if c.MovType != "" { dmov[c.MovType]++ }
		}
		fmt.Printf("Campos vacios: tipo_reg=%d, empresa=%d, secuencia=%d, nit=%d\n",
			emptyTipoR, emptyEmp, emptySec, emptyNitC)
		fmt.Println("Tipos registro:", ctypes)
		fmt.Println("Tipos D/C:", dmov)

		fmt.Println("\nFirst 10 records cartera records:")
		for i, c := range cart {
			if i >= 10 { break }
			fmt.Printf("  [%d] tipo:%s emp:%s sec:%-6s nit:%-13s fecha:%s mov:%s cuenta:%s | %s\n",
				i+1, c.RecordType, c.Company, c.Sequence, c.ThirdPartyNit, c.Date, c.MovType, c.LedgerAccount, c.Description)
		}
		fmt.Println()
	}

	// === DANE ===
	fmt.Println("=== CODIGOS DANE (ZDANE) ===")
	codigos, err := parsers.ParseDane(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Total: %d municipios\n", len(codigos))
		for i, c := range codigos {
			if i >= 5 { break }
			fmt.Printf("  %s: %s\n", c.Code, c.Name)
		}
	}
	fmt.Println()

	// === ICA ===
	fmt.Println("=== ACTIVIDADES ICA (ZICA) ===")
	actividades, err := parsers.ParseICA(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Total: %d actividades\n", len(actividades))
		for i, a := range actividades {
			if i >= 5 { break }
			fmt.Printf("  %s: %-50s tarifa:%s\n", a.Code, a.Name, a.Rate)
		}
	}
	fmt.Println()

	// === PILA ===
	fmt.Println("=== CONCEPTOS PILA (ZPILA) ===")
	conceptos, err := parsers.ParsePILA(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Total: %d conceptos\n", len(conceptos))
		for i, c := range conceptos {
			if i >= 5 { break }
			fmt.Printf("  %s %s-%s flags:%s base:%s calc:%s\n",
				c.RecType, c.Fund, c.Concept, c.Flags, c.BaseType, c.CalcBase)
		}
	}
	fmt.Println()

	// === LIBROS AUXILIARES ===
	fmt.Println("=== LIBROS AUXILIARES (Z07) ===")
	libros, yearLib, err := parsers.ParseLibrosAuxiliares(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z07%s, Total: %d\n", yearLib, len(libros))
		for i, l := range libros {
			if i >= 5 { break }
			fmt.Printf("  tipo:%s-%s cuenta:%-9s nit:%-10s fecha:%s saldo:%.2f | sec:%s-%s\n",
				l.VoucherType, l.VoucherCode, l.LedgerAccount,
				l.ThirdPartyNit, l.DocDate, l.Balance,
				l.SecVoucherType, l.SecVoucherCode)
		}
	}
	fmt.Println()

	// === TRANSACCIONES DETALLE ===
	fmt.Println("=== TRANSACCIONES DETALLE (Z07T) ===")
	trans, err := parsers.ParseTransaccionesDetalle(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Total: %d transacciones\n", len(trans))
		for i, t := range trans {
			if i >= 5 { break }
			fmt.Printf("  %s emp:%s nit:%-12s cta:%-9s fecha:%s mov:%s val:%.0f\n",
				t.VoucherType, t.Company, t.ThirdPartyNit, t.LedgerAccount,
				t.DocDate, t.MovType, t.Amount)
		}
	}
	fmt.Println()

	// === PERIODOS CONTABLES ===
	fmt.Println("=== PERIODOS CONTABLES (Z26) ===")
	periodos, yearPer, err := parsers.ParsePeriodos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z26%s, Total: %d periodos\n", yearPer, len(periodos))
		for i, p := range periodos {
			if i >= 5 { break }
			fmt.Printf("  periodo:%s emp:%s inicio:%s fin:%s estado:%s\n",
				p.PeriodNumber, p.Company, p.StartDate, p.EndDate, p.Status)
		}
	}
	fmt.Println()

	// === CONDICIONES DE PAGO ===
	fmt.Println("=== CONDICIONES DE PAGO (Z05) ===")
	conds, yearCond, err := parsers.ParseCondicionesPago(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z05%s, Total: %d records\n", yearCond, len(conds))
		for i, c := range conds {
			if i >= 5 { break }
			fmt.Printf("  tipo:%s emp:%s sec:%s nit:%-12s fecha:%s sec2:%s val:%.2f\n",
				c.RecType, c.Company, c.Sequence, c.NIT, c.Date, c.SecondaryType, c.Amount)
		}
	}
	fmt.Println()

	// === ACTIVOS FIJOS DETALLE ===
	fmt.Println("=== ACTIVOS FIJOS DETALLE (Z27) ===")
	activos, yearAct, err := parsers.ParseActivosFijosDetalle(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z27%s, Total: %d activos\n", yearAct, len(activos))
		for _, a := range activos {
			fmt.Printf("  grupo:%s seq:%s nit:%-13s fecha:%s val:%.2f | %s\n",
				a.Group, a.Sequence, a.ResponsibleNit, a.Date, a.PurchaseValue, a.Name)
		}
	}
	fmt.Println()

	// === AUDIT TRAIL TERCEROS ===
	fmt.Println("=== AUDIT TRAIL TERCEROS (Z11N) ===")
	audit, yearAudit, err := parsers.ParseAuditTrailTerceros(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z11N%s, Total: %d records\n", yearAudit, len(audit))
		for i, a := range audit {
			if i >= 10 { break }
			fmt.Printf("  fecha:%s nit:%-12s tipo:%s usuario:%s | %s\n",
				a.ChangeDate, a.ThirdPartyNit, a.DocType, a.User, a.Name)
		}
	}
	fmt.Println()

	// === CLASIFICACION CUENTAS ===
	fmt.Println("=== CLASIFICACION CUENTAS (Z279CP) ===")
	clasif, yearClasif, err := parsers.ParseClasificacionCuentas(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("File: Z279CP%s, Total: %d clasificaciones\n", yearClasif, len(clasif))
		for i, c := range clasif {
			if i >= 10 { break }
			fmt.Printf("  cuenta:%s grupo:%s detalle:%s | %s\n",
				c.AccountCode, c.GroupCode, c.DetailCode, c.Description)
		}
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
