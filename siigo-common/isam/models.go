package isam

// ---------------------------------------------------------------------------
// models.go — All 24 Siigo ISAM table models (Eloquent-style)
//
// Each model is a self-describing package-level variable.
// Call isam.ConnectAll(dataDir, year) to resolve file paths.
//
// Example:
//
//	isam.ConnectAll(`C:\DEMOS01`, "2016")
//	rec, _ := isam.Clients.Find("00000000002001")
//	fmt.Println(rec.Get("nombre"))
//
// ---------------------------------------------------------------------------

// Clients — Z17: Terceros (Clientes)
var Clients = DefineModel("clients", "Z17", false, "", 1438, func(m *Model) {
	m.String("tipo", 0, 1)
	m.String("empresa", 1, 3)
	m.Key("codigo", 4, 14)
	m.String("tipo_doc", 18, 2)
	m.String("numero_doc", 22, 6)
	m.Date("fecha_creacion", 28, 8)
	m.String("nombre", 36, 40)
	m.String("tipo_cta_pref", 86, 1)
})

// Products — Z04YYYY: Inventario (Productos)
var Products = DefineModel("products", "Z04", true, "", 3520, func(m *Model) {
	m.String("empresa", 0, 5)
	m.String("grupo", 5, 3)
	m.Key("codigo", 8, 6)
	m.String("nombre", 14, 50)
	m.String("nombre_corto", 64, 40)
	m.String("referencia", 104, 30)
})

// Movements — Z49: Movimientos
var Movements = DefineModel("movements", "Z49", false, "", 2295, func(m *Model) {
	m.String("tipo_comp", 0, 1)
	m.Key("codigo", 1, 3)
	m.String("num_doc", 4, 11)
	m.String("nombre_tercero", 15, 35)
})

// Cartera — Z09YYYY: Cartera
var Cartera = DefineModel("cartera", "Z09", true, "", 1152, func(m *Model) {
	m.String("tipo", 0, 1)
	m.String("empresa", 1, 3)
	m.Key("seq", 10, 5)
	m.String("tipo_doc", 15, 1)
	m.String("nit", 16, 13)
	m.String("cuenta", 29, 13)
	m.Date("fecha", 42, 8)
	m.String("descripcion", 93, 40)
	m.String("dc", 143, 1)
})

// Maestros — Z06: Maestros
var Maestros = DefineModel("maestros", "Z06", false, "", 4096, func(m *Model) {
	m.String("tipo", 0, 1)
	m.Key("codigo", 2, 7)
	m.String("nombre", 31, 20)
	m.String("responsable", 70, 20)
	m.String("direccion", 90, 30)
})

// PlanCuentas — Z03YYYY: Plan de Cuentas
var PlanCuentas = DefineModel("plan_cuentas", "Z03", true, "", 1152, func(m *Model) {
	m.String("empresa", 0, 3)
	m.Key("cuenta", 3, 9)
	m.String("activa", 12, 1)
	m.String("auxiliar", 13, 1)
	m.String("naturaleza", 17, 8)
	m.String("nombre", 25, 70)
})

// ActivosFijos — Z27YYYY: Activos Fijos
var ActivosFijos = DefineModel("activos_fijos", "Z27", true, "", 2048, func(m *Model) {
	m.String("empresa", 0, 5)
	m.Key("codigo", 5, 6)
	m.String("nombre", 11, 50)
	m.String("nit", 61, 13)
	m.Date("fecha", 122, 8)
})

// Documentos — Z11YYYY: Documentos
var Documentos = DefineModel("documentos", "Z11", true, "", 518, func(m *Model) {
	m.String("tipo", 0, 1)
	m.Key("codigo", 1, 3)
	m.String("seq", 10, 5)
	m.String("nit", 21, 13)
	m.String("cuenta", 29, 13)
	m.String("producto", 42, 7)
	m.String("bodega", 49, 3)
	m.String("cc", 52, 3)
	m.Date("fecha", 55, 8)
	m.String("descripcion", 93, 50)
	m.String("dc", 143, 1)
	m.String("referencia", 167, 7)
})

// TercerosAmpliados — Z08YYYYA: Terceros Ampliados
var TercerosAmpliados = DefineModel("terceros_ampliados", "Z08", true, "A", 1152, func(m *Model) {
	m.String("empresa", 0, 3)
	m.Key("nit", 5, 8)
	m.String("tipo_persona", 16, 2)
	m.String("nombre", 18, 60)
	m.String("rep_legal", 96, 60)
	m.String("direccion", 194, 56)
	m.String("email", 323, 70)
})

// SaldosTerceros — Z25YYYY: Saldos Terceros
var SaldosTerceros = DefineModel("saldos_terceros", "Z25", true, "", 512, func(m *Model) {
	m.String("empresa", 0, 3)
	m.Key("cuenta", 3, 9)
	m.String("nit", 12, 13)
	m.BCD("saldo_anterior", 140, 8, 2)
	m.BCD("debito", 148, 8, 2)
	m.BCD("credito", 156, 8, 2)
})

// SaldosConsolidados — Z28YYYY: Saldos Consolidados
var SaldosConsolidados = DefineModel("saldos_consolidados", "Z28", true, "", 512, func(m *Model) {
	m.String("empresa", 0, 3)
	m.Key("cuenta", 3, 9)
	m.BCD("saldo_anterior", 38, 8, 2)
	m.BCD("debito", 46, 8, 2)
	m.BCD("credito", 54, 8, 2)
})

// CodigosDane — ZDANE: Códigos DANE
var CodigosDane = DefineModel("codigos_dane", "ZDANE", false, "", 256, func(m *Model) {
	m.Key("codigo", 0, 5)
	m.String("nombre", 5, 40)
})

// Historial — Z18YYYY: Historial
var Historial = DefineModel("historial", "Z18", true, "", 524, func(m *Model) {
	m.String("tipo", 0, 1)
	m.String("sub_tipo", 1, 2)
	m.Key("empresa", 3, 3)
	m.Date("fecha", 63, 8)
	m.String("nombre", 77, 40)
	m.String("nombre2", 161, 40)
})

// ActividadesICA — ZICA: Actividades ICA
var ActividadesICA = DefineModel("actividades_ica", "ZICA", false, "", 256, func(m *Model) {
	m.Key("codigo", 0, 5)
	m.String("nombre", 5, 50)
	m.String("tarifa", 55, 6)
})

// ConceptosPILA — ZPILA: Conceptos PILA
var ConceptosPILA = DefineModel("conceptos_pila", "ZPILA", false, "", 230, func(m *Model) {
	m.Key("tipo", 0, 8)
	m.String("fondo", 8, 4)
	m.String("concepto", 12, 3)
	m.String("flags", 30, 2)
	m.String("tipo_base", 32, 4)
	m.String("base_calculo", 36, 4)
})

// LibrosAuxiliares — Z07YYYY: Libros Auxiliares
var LibrosAuxiliares = DefineModel("libros_auxiliares", "Z07", true, "", 256, func(m *Model) {
	m.String("empresa", 7, 3)
	m.Key("cuenta", 10, 9)
	m.String("tipo_comp", 20, 1)
	m.String("cod_comp", 21, 3)
	m.Date("fecha_doc", 33, 8)
	m.String("nit", 41, 13)
	m.BCD("saldo", 112, 6, 2)
	m.BCD("debito", 118, 6, 2)
	m.BCD("credito", 124, 7, 2)
	m.Date("fecha_reg", 133, 8)
	m.String("num_ref", 144, 7)
	m.String("tipo_sec", 155, 1)
	m.String("cod_sec", 156, 3)
})

// TransaccionesDetalle — Z07T: Transacciones Detalle
var TransaccionesDetalle = DefineModel("transacciones_detalle", "Z07T", false, "", 256, func(m *Model) {
	m.String("tipo", 0, 1)
	m.String("empresa", 1, 3)
	m.Key("seq", 4, 12)
	m.String("nit", 18, 14)
	m.String("empresa_cta", 32, 3)
	m.String("cuenta", 35, 9)
	m.String("tipo_sec", 44, 1)
	m.Date("fecha", 64, 8)
	m.Date("fecha_venc", 94, 8)
	m.String("dc", 102, 1)
	m.String("valor", 103, 13)
})

// PeriodosContables — Z26YYYY: Periodos Contables
var PeriodosContables = DefineModel("periodos_contables", "Z26", true, "", 1544, func(m *Model) {
	m.Key("num_periodo", 40, 4)
	m.Date("fecha_inicio", 44, 8)
	m.Date("fecha_fin", 52, 8)
	m.BCD("saldo1", 60, 7, 2)
	m.BCD("saldo2", 67, 7, 2)
	m.BCD("saldo3", 74, 7, 2)
})

// CondicionesPago — Z05YYYY: Condiciones de Pago
var CondicionesPago = DefineModel("condiciones_pago", "Z05", true, "", 1023, func(m *Model) {
	m.String("tipo", 0, 1)
	m.String("empresa", 1, 3)
	m.String("flag", 9, 1)
	m.Key("seq", 10, 3)
	m.String("tipo_doc", 13, 1)
	m.Date("fecha", 14, 8)
	m.String("nit", 27, 13)
	m.String("tipo_sec", 131, 4)
	m.BCD("valor", 211, 7, 2)
	m.Date("fecha_reg", 224, 8)
})

// MovimientosInventario — Z16YYYY: Movimientos Inventario
var MovimientosInventario = DefineModel("movimientos_inventario", "Z16", true, "", 320, func(m *Model) {
	m.String("empresa", 0, 5)
	m.Key("codigo", 5, 6)
	m.String("tipo", 11, 1)
	m.String("bodega", 12, 3)
	m.String("cantidad", 15, 10)
	m.Date("fecha", 25, 8)
})

// SaldosInventario — Z23YYYY: Saldos Inventario
var SaldosInventario = DefineModel("saldos_inventario", "Z23", true, "", 441, func(m *Model) {
	m.String("empresa", 0, 5)
	m.Key("codigo", 5, 6)
	m.String("bodega", 11, 3)
	m.String("cantidad", 14, 10)
})

// ClasificacionCuentas — Z279CP: Clasificación Cuentas
var ClasificacionCuentas = DefineModel("clasificacion_cuentas", "Z279CP", false, "", 128, func(m *Model) {
	m.Key("codigo", 0, 9)
	m.String("nombre", 9, 40)
	m.String("tipo", 49, 1)
})

// ActivosFijosDetalle — Z27YYYYA: Activos Fijos Detalle
var ActivosFijosDetalle = DefineModel("activos_fijos_detalle", "Z27", true, "A", 1536, func(m *Model) {
	m.String("empresa", 0, 5)
	m.Key("codigo", 5, 6)
	m.String("nombre", 11, 50)
	m.String("nit", 61, 13)
	m.Date("fecha", 122, 8)
	m.BCD("valor_compra", 130, 8, 2)
	m.String("ubicacion", 586, 46)
	m.String("referencia", 736, 8)
})

// AuditTrailTerceros — Z11NYYYY: Audit Trail Terceros
var AuditTrailTerceros = DefineModel("audit_trail_terceros", "Z11N", true, "", 846, func(m *Model) {
	m.Date("fecha_cambio", 0, 8)
	m.Key("nit", 16, 13)
	m.Date("timestamp", 41, 8)
	m.String("usuario", 48, 5)
	m.Date("fecha_periodo", 56, 8)
	m.String("tipo_doc", 80, 2)
	m.String("nombre", 82, 40)
	m.String("nit_rep", 142, 16)
	m.String("nom_rep", 158, 40)
	m.String("direccion", 250, 40)
	m.String("email", 391, 47)
})
