package main

import (
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
			GROUP_CONCAT(DISTINCT CASE WHEN lote != '' THEN lote END) as lotes
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
		Producto    string             `json:"producto"`
		Desc        string             `json:"descripcion"`
		Empresa     string             `json:"empresa"`
		Cuenta      string             `json:"cuenta"`
		Precio      float64            `json:"precio_unitario"`
		OrdenCompra string             `json:"orden_compra,omitempty"`
		Lote        string             `json:"lote,omitempty"`
		Meses       map[string]float64 `json:"valores_mes"`
		Cantidades  map[string]float64 `json:"cantidades_mes"`
		TotalValor  float64            `json:"total_valor"`
		TotalQty    float64            `json:"total_cantidad"`
	}

	grouped := make(map[prodKey]*prodData)
	var nombre string
	var totalValor float64

	for rows.Next() {
		var n, nc, emp, cuenta, codProd, desc, mes string
		var total, precio, qty float64
		var ocs, lotes *string
		if err := rows.Scan(&n, &nc, &emp, &cuenta, &codProd, &desc, &mes, &total, &precio, &qty, &ocs, &lotes); err != nil {
			continue
		}
		nombre = nc
		k := prodKey{cuenta, codProd}
		if _, ok := grouped[k]; !ok {
			grouped[k] = &prodData{
				Producto:   codProd,
				Desc:       desc,
				Empresa:    emp,
				Cuenta:     cuenta,
				Precio:     precio,
				Meses:      make(map[string]float64),
				Cantidades: make(map[string]float64),
			}
			if ocs != nil {
				grouped[k].OrdenCompra = *ocs
			}
			if lotes != nil {
				grouped[k].Lote = *lotes
			}
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

	rows, err := conn.Query(`
		SELECT nit, nombre_cliente, tipo_comprobante, num_documento, documento_ref,
			fecha, fecha_vencimiento, dias, saldo
		FROM cartera_cxc
		WHERE nit = ? AND saldo > 0
		ORDER BY fecha
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
		Saldo            float64 `json:"saldo"`
	}

	var items []item
	totalSaldo := 0.0
	var nombre string
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.NIT, &it.NombreCliente, &it.TipoComprobante, &it.NumDocumento,
			&it.DocumentoRef, &it.Fecha, &it.FechaVencimiento, &it.Dias, &it.Saldo); err != nil {
			continue
		}
		items = append(items, it)
		totalSaldo += it.Saldo
		nombre = it.NombreCliente
	}

	return map[string]interface{}{
		"nombre_cliente":  nombre,
		"total_registros": len(items),
		"total_saldo":     totalSaldo,
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
