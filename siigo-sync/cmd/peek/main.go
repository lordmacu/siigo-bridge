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

	// === MOVIMIENTOS ===
	fmt.Println("=== MOVIMIENTOS (Z49) - DETALLE ===")
	movs, err := parsers.ParseMovimientos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total: %d\n", len(movs))

	emptyTipoC, emptyNumDoc, emptyDesc, emptyFecha, emptyNit := 0, 0, 0, 0, 0
	tiposComp := map[string]int{}
	tiposMov := map[string]int{}
	for _, m := range movs {
		if m.TipoComprobante == "" { emptyTipoC++ }
		if m.NumeroDoc == "" { emptyNumDoc++ }
		if m.Descripcion == "" { emptyDesc++ }
		if m.Fecha == "" { emptyFecha++ }
		if m.NitTercero == "" { emptyNit++ }
		if m.TipoComprobante != "" {
			tiposComp[m.TipoComprobante[:min(2, len(m.TipoComprobante))]]++
		}
		if m.TipoMov != "" {
			tiposMov[m.TipoMov]++
		}
	}
	fmt.Printf("Campos vacios: tipo_comp=%d, num_doc=%d, descripcion=%d, fecha=%d, nit=%d\n",
		emptyTipoC, emptyNumDoc, emptyDesc, emptyFecha, emptyNit)
	fmt.Println("Tipos comprobante:", tiposComp)
	fmt.Println("Tipos D/C:", tiposMov)

	fmt.Println("\nPrimeros 10 movimientos:")
	for i, m := range movs {
		if i >= 10 { break }
		fmt.Printf("  [%d] tipo:%s doc:%-8s fecha:%s nit:%-13s cuenta:%-13s mov:%s valor:%s | %s\n",
			i+1, m.TipoComprobante, m.NumeroDoc, m.Fecha, m.NitTercero, m.CuentaContable, m.TipoMov, m.Valor, m.Descripcion)
	}
	fmt.Println()

	// === PRODUCTOS ===
	fmt.Println("=== PRODUCTOS (Z06CP) - DETALLE ===")
	prods, err := parsers.ParseProductos(dataPath)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
	fmt.Printf("Total: %d\n", len(prods))
	for _, p := range prods {
		fmt.Printf("  %s-%s | tipo:%s grupo:%s cuenta:%-13s fecha:%s mov:%s | %s\n",
			p.Comprobante, p.Secuencia, p.TipoTercero, p.Grupo, p.CuentaContable, p.Fecha, p.TipoMov, p.Nombre)
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
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
