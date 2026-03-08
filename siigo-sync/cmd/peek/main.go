package main

import (
	"fmt"
	"siigo-common/parsers"
)

func main() {
	dataPath := `C:\DEMOS01\`

	// === TERCEROS ===
	fmt.Println("=== TERCEROS (Z17) - DETALLE ===")
	all, err := parsers.ParseTerceros(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total terceros: %d\n", len(all))

	clientes, _ := parsers.ParseTercerosClientes(dataPath)
	fmt.Printf("Clientes (G): %d\n", len(clientes))

	// Validar campos
	emptyNombre, emptyDoc, emptyTipo := 0, 0, 0
	tiposClave := map[string]int{}
	tiposDoc := map[string]int{}
	for _, t := range all {
		if t.Nombre == "" { emptyNombre++ }
		if t.NumeroDoc == "" { emptyDoc++ }
		if t.TipoClave == "" { emptyTipo++ }
		tiposClave[t.TipoClave]++
		tiposDoc[t.TipoDoc]++
	}
	fmt.Printf("Campos vacios: nombre=%d, documento=%d, tipo_clave=%d\n", emptyNombre, emptyDoc, emptyTipo)
	fmt.Println("Tipos clave:", tiposClave)
	fmt.Println("Tipos doc:", tiposDoc)

	fmt.Println("\nPrimeros 10 terceros:")
	for i, t := range all {
		if i >= 10 { break }
		fmt.Printf("  [%d] clave:%s emp:%s tipo_doc:%s doc:%-13s fecha:%s | %s\n",
			i+1, t.TipoClave, t.Empresa, t.TipoDoc, t.NumeroDoc, t.FechaCreacion, t.Nombre)
		if t.TipoCtaPref != "" {
			fmt.Printf("       tipo_cta:%s\n", t.TipoCtaPref)
		}
	}
	fmt.Println()

	// === MOVIMIENTOS (Z49 = document headers, NOT detailed transactions) ===
	fmt.Println("=== MOVIMIENTOS (Z49) - ENCABEZADOS DE DOCUMENTOS ===")
	fmt.Println("NOTA: Z49 solo contiene tipo+numero+nombre+descripcion. NO tiene fechas/cuentas/valores.")
	movs, err := parsers.ParseMovimientos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total: %d\n", len(movs))

	emptyTipoC, emptyNumDoc, emptyDesc, emptyNombre := 0, 0, 0, 0
	tiposComp := map[string]int{}
	for _, m := range movs {
		if m.TipoComprobante == "" { emptyTipoC++ }
		if m.NumeroDoc == "" { emptyNumDoc++ }
		if m.Descripcion == "" && m.Descripcion2 == "" { emptyDesc++ }
		if m.NombreTercero == "" { emptyNombre++ }
		if m.TipoComprobante != "" {
			tiposComp[m.TipoComprobante[:min(2, len(m.TipoComprobante))]]++
		}
	}
	fmt.Printf("Campos vacios: tipo_comp=%d, num_doc=%d, descripcion=%d, nombre=%d\n",
		emptyTipoC, emptyNumDoc, emptyDesc, emptyNombre)
	fmt.Println("Tipos comprobante:", tiposComp)

	// Show first 10 records WITH tipo_comprobante (skip space/0-type index records)
	fmt.Println("\nPrimeros 10 movimientos con comprobante:")
	shown := 0
	for _, m := range movs {
		if shown >= 10 { break }
		if m.TipoComprobante == "" { continue }
		shown++
		desc := m.Descripcion
		if m.Descripcion2 != "" {
			if desc != "" {
				desc += " | " + m.Descripcion2
			} else {
				desc = m.Descripcion2
			}
		}
		fmt.Printf("  [%d] tipo:%-6s doc:%-11s nombre:%-25s | %s\n",
			shown, m.TipoComprobante, m.NumeroDoc, m.NombreTercero, desc)
	}
	fmt.Println()

	// === PRODUCTOS (INVENTARIO) ===
	fmt.Println("=== PRODUCTOS (Z04 - Inventario) - DETALLE ===")
	prods, year, err := parsers.ParseInventario(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Archivo: Z04%s, Total: %d\n", year, len(prods))
	for _, p := range prods {
		fmt.Printf("  cod:%-8s grupo:%s emp:%s | %-40s | corto:%-30s ref:%s\n",
			p.Codigo, p.Grupo, p.Empresa, p.Nombre, p.NombreCorto, p.Referencia)
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
			if c.TipoRegistro == "" { emptyTipoR++ }
			if c.Empresa == "" { emptyEmp++ }
			if c.Secuencia == "" { emptySec++ }
			if c.NitTercero == "" { emptyNitC++ }
			ctypes[c.TipoRegistro]++
			if c.TipoMov != "" { dmov[c.TipoMov]++ }
		}
		fmt.Printf("Campos vacios: tipo_reg=%d, empresa=%d, secuencia=%d, nit=%d\n",
			emptyTipoR, emptyEmp, emptySec, emptyNitC)
		fmt.Println("Tipos registro:", ctypes)
		fmt.Println("Tipos D/C:", dmov)

		fmt.Println("\nPrimeros 10 registros cartera:")
		for i, c := range cart {
			if i >= 10 { break }
			fmt.Printf("  [%d] tipo:%s emp:%s sec:%-6s nit:%-13s fecha:%s mov:%s cuenta:%s | %s\n",
				i+1, c.TipoRegistro, c.Empresa, c.Secuencia, c.NitTercero, c.Fecha, c.TipoMov, c.CuentaContable, c.Descripcion)
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
			fmt.Printf("  %s: %s\n", c.Codigo, c.Nombre)
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
			fmt.Printf("  %s: %-50s tarifa:%s\n", a.Codigo, a.Nombre, a.Tarifa)
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
				c.Tipo, c.Fondo, c.Concepto, c.Flags, c.TipoBase, c.BaseCalculo)
		}
	}
	fmt.Println()

	// === LIBROS AUXILIARES ===
	fmt.Println("=== LIBROS AUXILIARES (Z07) ===")
	libros, yearLib, err := parsers.ParseLibrosAuxiliares(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z07%s, Total: %d\n", yearLib, len(libros))
		for i, l := range libros {
			if i >= 5 { break }
			fmt.Printf("  tipo:%s-%s cuenta:%-9s nit:%-10s fecha:%s saldo:%.2f | sec:%s-%s\n",
				l.TipoComprobante, l.CodigoComprobante, l.CuentaContable,
				l.NitTercero, l.FechaDocumento, l.Saldo,
				l.TipoCompSec, l.CodigoCompSec)
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
				t.TipoComprobante, t.Empresa, t.NitTercero, t.CuentaContable,
				t.FechaDocumento, t.TipoMovimiento, t.Valor)
		}
	}
	fmt.Println()

	// === PERIODOS CONTABLES ===
	fmt.Println("=== PERIODOS CONTABLES (Z26) ===")
	periodos, yearPer, err := parsers.ParsePeriodos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z26%s, Total: %d periodos\n", yearPer, len(periodos))
		for i, p := range periodos {
			if i >= 5 { break }
			fmt.Printf("  periodo:%s emp:%s inicio:%s fin:%s estado:%s\n",
				p.NumeroPeriodo, p.Empresa, p.FechaInicio, p.FechaFin, p.Estado)
		}
	}
	fmt.Println()

	// === CONDICIONES DE PAGO ===
	fmt.Println("=== CONDICIONES DE PAGO (Z05) ===")
	conds, yearCond, err := parsers.ParseCondicionesPago(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z05%s, Total: %d registros\n", yearCond, len(conds))
		for i, c := range conds {
			if i >= 5 { break }
			fmt.Printf("  tipo:%s emp:%s sec:%s nit:%-12s fecha:%s sec2:%s val:%.2f\n",
				c.Tipo, c.Empresa, c.Secuencia, c.NIT, c.Fecha, c.TipoSecundario, c.Valor)
		}
	}
	fmt.Println()

	// === ACTIVOS FIJOS DETALLE ===
	fmt.Println("=== ACTIVOS FIJOS DETALLE (Z27) ===")
	activos, yearAct, err := parsers.ParseActivosFijosDetalle(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z27%s, Total: %d activos\n", yearAct, len(activos))
		for _, a := range activos {
			fmt.Printf("  grupo:%s seq:%s nit:%-13s fecha:%s val:%.2f | %s\n",
				a.Grupo, a.Secuencia, a.NitResponsable, a.Fecha, a.ValorCompra, a.Nombre)
		}
	}
	fmt.Println()

	// === AUDIT TRAIL TERCEROS ===
	fmt.Println("=== AUDIT TRAIL TERCEROS (Z11N) ===")
	audit, yearAudit, err := parsers.ParseAuditTrailTerceros(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z11N%s, Total: %d registros\n", yearAudit, len(audit))
		for i, a := range audit {
			if i >= 10 { break }
			fmt.Printf("  fecha:%s nit:%-12s tipo:%s usuario:%s | %s\n",
				a.FechaCambio, a.NitTercero, a.TipoDoc, a.Usuario, a.Nombre)
		}
	}
	fmt.Println()

	// === CLASIFICACION CUENTAS ===
	fmt.Println("=== CLASIFICACION CUENTAS (Z279CP) ===")
	clasif, yearClasif, err := parsers.ParseClasificacionCuentas(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("Archivo: Z279CP%s, Total: %d clasificaciones\n", yearClasif, len(clasif))
		for i, c := range clasif {
			if i >= 10 { break }
			fmt.Printf("  cuenta:%s grupo:%s detalle:%s | %s\n",
				c.CodigoCuenta, c.CodigoGrupo, c.CodigoDetalle, c.Descripcion)
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
