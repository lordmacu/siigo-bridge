package main

import (
	"math"
	"time"
)

// dispatchReportWebhooks: builds the processed report per affected NIT and
// fires a webhook event with the same data the HTTP endpoint returns.
//
// table: "ventas_productos" | "recaudo" | "cartera_cxc"
// nits: unique NITs affected by the diff
func (s *Server) dispatchReportWebhooks(table string, nits []string) {
	if len(nits) == 0 {
		return
	}
	for _, nit := range nits {
		var report map[string]interface{}
		var event string

		switch table {
		case "ventas_productos":
			report = s.buildVentasReport(nit)
			event = "ventas_report"
		case "recaudo":
			report = s.buildRecaudoReport(nit)
			event = "recaudo_report"
		case "cartera_cxc":
			report = s.buildCarteraReport(nit)
			event = "cartera_report"
		default:
			continue
		}

		if report == nil {
			continue
		}
		report["nit"] = nit
		s.webhookDispatch(event, report)
	}
}

// buildVentasReport returns the same data structure as GET /api/ventas/{nit}
// for the current year, default filters.
func (s *Server) buildVentasReport(nit string) map[string]interface{} {
	conn := s.db.GetConn()
	desde := time.Now().Format("2006") + "-01"
	hasta := time.Now().Format("2006-01")

	rows, err := conn.Query(`
		SELECT nit, nombre_cliente, empresa, cuenta, codigo_producto, descripcion, mes,
			SUM(total_venta) as total, MAX(precio_unitario) as precio, SUM(cantidad) as qty,
			GROUP_CONCAT(DISTINCT CASE WHEN orden_compra != '' THEN orden_compra END) as ocs,
			GROUP_CONCAT(DISTINCT CASE WHEN lote != '' THEN lote END) as lotes,
			GROUP_CONCAT(DISTINCT CASE WHEN fecha_despacho != '' THEN fecha_despacho END) as fechas_despacho,
			GROUP_CONCAT(DISTINCT CASE WHEN empaque != '' THEN empaque END) as empaques,
			GROUP_CONCAT(DISTINCT CASE WHEN observaciones != '' THEN observaciones END) as obs
		FROM ventas_productos
		WHERE nit = ? AND mes >= ? AND mes <= ?
		GROUP BY nit, cuenta, codigo_producto, mes
		ORDER BY cuenta, codigo_producto, mes
	`, nit, desde, hasta)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type prodKey struct{ cuenta, codProd string }
	type prodData struct {
		Producto      string             `json:"producto"`
		Desc          string             `json:"descripcion"`
		Empresa       string             `json:"empresa"`
		Cuenta        string             `json:"cuenta"`
		Precio        float64            `json:"precio_unitario"`
		OrdenCompra   string             `json:"orden_compra,omitempty"`
		Lote          string             `json:"lote,omitempty"`
		FechaDespacho string             `json:"fecha_despacho,omitempty"`
		Empaque       string             `json:"empaque,omitempty"`
		Observaciones string             `json:"observaciones,omitempty"`
		Meses         map[string]float64 `json:"valores_mes"`
		Cantidades    map[string]float64 `json:"cantidades_mes"`
		TotalValor    float64            `json:"total_valor"`
		TotalQty      float64            `json:"total_cantidad"`
	}

	grouped := make(map[prodKey]*prodData)
	var nombre string
	var totalValor float64

	for rows.Next() {
		var n, nc, emp, cuenta, codProd, desc, mes string
		var total, precio, qty float64
		var ocs, lotes, fechasDespacho, empaques, obs *string
		if err := rows.Scan(&n, &nc, &emp, &cuenta, &codProd, &desc, &mes,
			&total, &precio, &qty, &ocs, &lotes, &fechasDespacho, &empaques, &obs); err != nil {
			continue
		}
		nombre = nc
		k := prodKey{cuenta, codProd}
		if _, ok := grouped[k]; !ok {
			pd := &prodData{
				Producto:   codProd,
				Desc:       desc,
				Empresa:    emp,
				Cuenta:     cuenta,
				Precio:     precio,
				Meses:      make(map[string]float64),
				Cantidades: make(map[string]float64),
			}
			if ocs != nil { pd.OrdenCompra = *ocs }
			if lotes != nil { pd.Lote = *lotes }
			if fechasDespacho != nil { pd.FechaDespacho = *fechasDespacho }
			if empaques != nil { pd.Empaque = *empaques }
			if obs != nil { pd.Observaciones = *obs }
			grouped[k] = pd
		}
		grouped[k].Meses[mes] += total
		grouped[k].Cantidades[mes] += qty
		grouped[k].TotalValor += total
		grouped[k].TotalQty += qty
		totalValor += total
	}

	ventas := make([]*prodData, 0, len(grouped))
	for _, v := range grouped {
		ventas = append(ventas, v)
	}

	return map[string]interface{}{
		"nombre_cliente": nombre,
		"desde":          desde,
		"hasta":          hasta,
		"total_lineas":   len(ventas),
		"total_valor":    totalValor,
		"ventas":         ventas,
	}
}

// buildRecaudoReport returns the same shape as GET /api/recaudo/{nit}
func (s *Server) buildRecaudoReport(nit string) map[string]interface{} {
	conn := s.db.GetConn()
	desde := time.Now().Format("2006") + "-01"
	hasta := time.Now().Format("2006-01")

	rows, err := conn.Query(`
		SELECT nit, nombre_cliente, num_recibo, fecha_recibo, mes,
			num_factura, fecha_vencimiento, dias, valor_cancelado, tipo_pago,
			vendedor_codigo, vendedor_nombre, descripcion
		FROM recaudo
		WHERE nit = ? AND mes >= ? AND mes <= ?
		ORDER BY fecha_recibo, num_recibo
	`, nit, desde, hasta)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type item struct {
		NIT              string  `json:"nit"`
		NombreCliente    string  `json:"nombre_cliente"`
		NumRecibo        int     `json:"num_recibo"`
		FechaRecibo      string  `json:"fecha_recibo"`
		Mes              string  `json:"mes"`
		NumFactura       int     `json:"num_factura"`
		FechaVencimiento string  `json:"fecha_vencimiento"`
		Dias             int     `json:"dias"`
		ValorCancelado   float64 `json:"valor_cancelado"`
		TipoPago         string  `json:"tipo_pago"`
		VendedorCodigo   string  `json:"vendedor_codigo"`
		VendedorNombre   string  `json:"vendedor_nombre"`
		Descripcion      string  `json:"descripcion"`
	}

	var items []item
	totalValor := 0.0
	var nombre string
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.NIT, &it.NombreCliente, &it.NumRecibo, &it.FechaRecibo, &it.Mes,
			&it.NumFactura, &it.FechaVencimiento, &it.Dias, &it.ValorCancelado, &it.TipoPago,
			&it.VendedorCodigo, &it.VendedorNombre, &it.Descripcion); err != nil {
			continue
		}
		items = append(items, it)
		totalValor += it.ValorCancelado
		nombre = it.NombreCliente
	}

	return map[string]interface{}{
		"nombre_cliente":  nombre,
		"desde":           desde,
		"hasta":           hasta,
		"total_registros": len(items),
		"total_valor":     totalValor,
		"recaudo":         items,
	}
}

// buildCarteraReport returns the same shape as GET /api/cartera-cliente/{nit}
// (reads from cartera_cxc SQLite table, NOT from Z07 directly)
func (s *Server) buildCarteraReport(nit string) map[string]interface{} {
	conn := s.db.GetConn()
	today := time.Now().Format("2006-01-02")

	rows, err := conn.Query(`
		SELECT nit, nombre_cliente, tipo_comprobante, num_documento, documento_ref,
			fecha, fecha_vencimiento, saldo
		FROM cartera_cxc
		WHERE nit = ? AND saldo > 0
		ORDER BY fecha ASC, num_documento ASC
	`, nit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type item struct {
		NIT              string  `json:"nit"`
		NombreCliente    string  `json:"nombre_cliente"`
		TipoComprobante  string  `json:"tipo_comprobante"`
		NumDocumento     int     `json:"num_documento"`
		DocumentoRef     string  `json:"documento_ref,omitempty"`
		Fecha            string  `json:"fecha"`
		FechaVencimiento string  `json:"fecha_vencimiento"`
		Dias             int     `json:"dias"`
		SaldoContable    float64 `json:"saldo_contable"`
		Vencido          float64 `json:"vencido"`
		SaldoVencido     float64 `json:"saldo_vencido"`
	}

	var items []item
	totalVencido := 0.0
	var nombre string
	var accumVencido float64

	for rows.Next() {
		var it item
		var saldo float64
		if err := rows.Scan(&it.NIT, &it.NombreCliente, &it.TipoComprobante, &it.NumDocumento,
			&it.DocumentoRef, &it.Fecha, &it.FechaVencimiento, &saldo); err != nil {
			continue
		}

		// Recompute dias live from fecha_vencimiento
		dias := 0
		if tv, err := time.Parse("2006-01-02", it.FechaVencimiento); err == nil {
			if tt, err := time.Parse("2006-01-02", today); err == nil {
				dias = int(tv.Sub(tt).Hours() / 24)
			}
		}

		vencido := 0.0
		if dias < 0 && saldo > 0 {
			vencido = saldo
		}
		accumVencido += vencido

		it.Dias = dias
		it.SaldoContable = math.Round(saldo*100) / 100
		it.Vencido = math.Round(vencido*100) / 100
		it.SaldoVencido = math.Round(accumVencido*100) / 100
		totalVencido += vencido
		nombre = it.NombreCliente
		items = append(items, it)
	}

	return map[string]interface{}{
		"nombre_cliente":  nombre,
		"total_registros": len(items),
		"total_vencidos":  math.Round(totalVencido*100) / 100,
		"cartera":         items,
	}
}

// uniqueNITsFromRecords extracts unique NITs from a list of record maps.
func uniqueNITsFromRecords(records []map[string]interface{}) []string {
	set := make(map[string]bool)
	for _, r := range records {
		if v, ok := r["nit"].(string); ok && v != "" {
			set[v] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
