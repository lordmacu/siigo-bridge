package isam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// tables.go — Pre-defined table schemas for all 24 Siigo ISAM tables
//
// Each table has verified field offsets (EXTFH mode) from hex dump analysis.
// The data directory and year are configurable.
//
// Usage:
//
//	registry := isam.NewRegistry(`C:\SIIWI02`, "2016")
//	clients := registry.Table("clients")
//	rec, _ := clients.Find("00000000002001")
//	fmt.Println(rec.Get("nombre"))
//
// ---------------------------------------------------------------------------

// Registry holds all table definitions for a Siigo data directory
type Registry struct {
	DataDir string // e.g. C:\SIIWI02
	Year    string // e.g. "2016" — used for year-suffixed files
	tables  map[string]*Table
}

// NewRegistry creates a registry with all 24 tables registered
func NewRegistry(dataDir, year string) *Registry {
	r := &Registry{
		DataDir: dataDir,
		Year:    year,
		tables:  make(map[string]*Table),
	}
	r.registerAll()
	return r
}

// Table returns a table by name (returns nil if not found)
func (r *Registry) Table(name string) *Table {
	return r.tables[name]
}

// Tables returns all registered table names
func (r *Registry) Tables() []string {
	names := make([]string, 0, len(r.tables))
	for k := range r.tables {
		names = append(names, k)
	}
	return names
}

// path builds a full file path, optionally with year suffix
func (r *Registry) path(base string, withYear bool) string {
	name := base
	if withYear {
		name = base + r.Year
	}
	return filepath.Join(r.DataDir, name)
}

// AvailableTables returns only tables whose ISAM files exist on disk
func (r *Registry) AvailableTables() []string {
	var available []string
	for name, t := range r.tables {
		if _, err := os.Stat(t.Path); err == nil {
			available = append(available, name)
		}
	}
	return available
}

// TableInfo returns a summary of a table (name, path, fields, record count)
func (r *Registry) TableInfo(name string) (map[string]interface{}, error) {
	t := r.tables[name]
	if t == nil {
		return nil, fmt.Errorf("table %q not found", name)
	}

	info := map[string]interface{}{
		"name":     t.Name,
		"path":     t.Path,
		"rec_size": t.RecSize,
		"fields":   len(t.Fields),
	}

	if _, err := os.Stat(t.Path); err == nil {
		count, err := t.Count()
		if err == nil {
			info["records"] = count
		}
	} else {
		info["records"] = -1
		info["error"] = "file not found"
	}

	fieldNames := make([]string, len(t.Fields))
	for i, f := range t.Fields {
		fieldNames[i] = f.Name
	}
	info["field_names"] = strings.Join(fieldNames, ", ")

	return info, nil
}

// registerAll registers all 24 Siigo ISAM tables
func (r *Registry) registerAll() {
	// --- Z17: Terceros (Clientes) ---
	r.tables["clients"] = NewTable("clients", r.path("Z17", false), 1438).
		String("tipo", 0, 1).
		String("empresa", 1, 3).
		Key("codigo", 4, 14).
		String("tipo_doc", 18, 2).
		String("numero_doc", 22, 6).
		Date("fecha_creacion", 28, 8).
		String("nombre", 36, 40).
		String("tipo_cta_pref", 86, 1)

	// --- Z04YYYY: Inventario (Productos) ---
	r.tables["products"] = NewTable("products", r.path("Z04", true), 3520).
		String("empresa", 0, 5).
		String("grupo", 5, 3).
		Key("codigo", 8, 6).
		String("nombre", 14, 50).
		String("nombre_corto", 64, 40).
		String("referencia", 104, 30)

	// --- Z49: Movimientos ---
	r.tables["movements"] = NewTable("movements", r.path("Z49", false), 2295).
		String("tipo_comp", 0, 1).
		Key("codigo", 1, 3).
		String("num_doc", 4, 11).
		String("nombre_tercero", 15, 35)

	// --- Z09YYYY: Cartera ---
	r.tables["cartera"] = NewTable("cartera", r.path("Z09", true), 1152).
		String("tipo", 0, 1).
		String("empresa", 1, 3).
		Key("seq", 10, 5).
		String("tipo_doc", 15, 1).
		String("nit", 16, 13).
		String("cuenta", 29, 13).
		Date("fecha", 42, 8).
		String("descripcion", 93, 40).
		String("dc", 143, 1)

	// --- Z06: Maestros ---
	r.tables["maestros"] = NewTable("maestros", r.path("Z06", false), 4096).
		String("tipo", 0, 1).
		Key("codigo", 2, 7).
		String("nombre", 31, 20).
		String("responsable", 70, 20).
		String("direccion", 90, 30)

	// --- Z03YYYY: Plan de Cuentas ---
	r.tables["plan_cuentas"] = NewTable("plan_cuentas", r.path("Z03", true), 1152).
		String("empresa", 0, 3).
		Key("cuenta", 3, 9).
		String("activa", 12, 1).
		String("auxiliar", 13, 1).
		String("naturaleza", 17, 8).
		String("nombre", 25, 70)

	// --- Z27YYYY: Activos Fijos ---
	r.tables["activos_fijos"] = NewTable("activos_fijos", r.path("Z27", true), 2048).
		String("empresa", 0, 5).
		Key("codigo", 5, 6).
		String("nombre", 11, 50).
		String("nit", 61, 13).
		Date("fecha", 122, 8)

	// --- Z11YYYY: Documentos ---
	r.tables["documentos"] = NewTable("documentos", r.path("Z11", true), 518).
		String("tipo", 0, 1).
		Key("codigo", 1, 3).
		String("seq", 10, 5).
		String("nit", 21, 13).
		String("cuenta", 29, 13).
		String("producto", 42, 7).
		String("bodega", 49, 3).
		String("cc", 52, 3).
		Date("fecha", 55, 8).
		String("descripcion", 93, 50).
		String("dc", 143, 1).
		String("referencia", 167, 7)

	// --- Z08YYYYA: Terceros Ampliados ---
	r.tables["terceros_ampliados"] = NewTable("terceros_ampliados", r.path("Z08", true)+"A", 1152).
		String("empresa", 0, 3).
		Key("nit", 5, 8).
		String("tipo_persona", 16, 2).
		String("nombre", 18, 60).
		String("rep_legal", 96, 60).
		String("direccion", 194, 56).
		String("email", 323, 70)

	// --- Z25YYYY: Saldos Terceros ---
	r.tables["saldos_terceros"] = NewTable("saldos_terceros", r.path("Z25", true), 512).
		String("empresa", 0, 3).
		Key("cuenta", 3, 9).
		String("nit", 12, 13).
		BCD("saldo_anterior", 140, 8, 2).
		BCD("debito", 148, 8, 2).
		BCD("credito", 156, 8, 2)

	// --- Z28YYYY: Saldos Consolidados ---
	r.tables["saldos_consolidados"] = NewTable("saldos_consolidados", r.path("Z28", true), 512).
		String("empresa", 0, 3).
		Key("cuenta", 3, 9).
		BCD("saldo_anterior", 38, 8, 2).
		BCD("debito", 46, 8, 2).
		BCD("credito", 54, 8, 2)

	// --- ZDANE: Códigos DANE ---
	r.tables["codigos_dane"] = NewTable("codigos_dane", r.path("ZDANE", false), 256).
		Key("codigo", 0, 5).
		String("nombre", 5, 40)

	// --- Z18YYYY: Historial ---
	r.tables["historial"] = NewTable("historial", r.path("Z18", true), 524).
		String("tipo", 0, 1).
		String("sub_tipo", 1, 2).
		Key("empresa", 3, 3).
		Date("fecha", 63, 8).
		String("nombre", 77, 40).
		String("nombre2", 161, 40)

	// --- ZICA: Actividades ICA ---
	r.tables["actividades_ica"] = NewTable("actividades_ica", r.path("ZICA", false), 256).
		Key("codigo", 0, 5).
		String("nombre", 5, 50).
		String("tarifa", 55, 6)

	// --- ZPILA: Conceptos PILA ---
	r.tables["conceptos_pila"] = NewTable("conceptos_pila", r.path("ZPILA", false), 230).
		Key("tipo", 0, 8).
		String("fondo", 8, 4).
		String("concepto", 12, 3).
		String("flags", 30, 2).
		String("tipo_base", 32, 4).
		String("base_calculo", 36, 4)

	// --- Z07YYYY: Libros Auxiliares ---
	r.tables["libros_auxiliares"] = NewTable("libros_auxiliares", r.path("Z07", true), 256).
		String("empresa", 7, 3).
		Key("cuenta", 10, 9).
		String("tipo_comp", 20, 1).
		String("cod_comp", 21, 3).
		Date("fecha_doc", 33, 8).
		String("nit", 41, 13).
		BCD("saldo", 112, 6, 2).
		BCD("debito", 118, 6, 2).
		BCD("credito", 124, 7, 2).
		Date("fecha_reg", 133, 8).
		String("num_ref", 144, 7).
		String("tipo_sec", 155, 1).
		String("cod_sec", 156, 3)

	// --- Z07T: Transacciones Detalle ---
	r.tables["transacciones_detalle"] = NewTable("transacciones_detalle", r.path("Z07T", false), 256).
		String("tipo", 0, 1).
		String("empresa", 1, 3).
		Key("seq", 4, 12).
		String("nit", 18, 14).
		String("empresa_cta", 32, 3).
		String("cuenta", 35, 9).
		String("tipo_sec", 44, 1).
		Date("fecha", 64, 8).
		Date("fecha_venc", 94, 8).
		String("dc", 102, 1).
		String("valor", 103, 13)

	// --- Z26YYYY: Periodos Contables ---
	r.tables["periodos_contables"] = NewTable("periodos_contables", r.path("Z26", true), 1544).
		Key("num_periodo", 40, 4).
		Date("fecha_inicio", 44, 8).
		Date("fecha_fin", 52, 8).
		BCD("saldo1", 60, 7, 2).
		BCD("saldo2", 67, 7, 2).
		BCD("saldo3", 74, 7, 2)

	// --- Z05YYYY: Condiciones de Pago ---
	r.tables["condiciones_pago"] = NewTable("condiciones_pago", r.path("Z05", true), 1023).
		String("tipo", 0, 1).
		String("empresa", 1, 3).
		String("flag", 9, 1).
		Key("seq", 10, 3).
		String("tipo_doc", 13, 1).
		Date("fecha", 14, 8).
		String("nit", 27, 13).
		String("tipo_sec", 131, 4).
		BCD("valor", 211, 7, 2).
		Date("fecha_reg", 224, 8)

	// --- Z16YYYY: Movimientos Inventario ---
	r.tables["movimientos_inventario"] = NewTable("movimientos_inventario", r.path("Z16", true), 320).
		String("empresa", 0, 5).
		Key("codigo", 5, 6).
		String("tipo", 11, 1).
		String("bodega", 12, 3).
		String("cantidad", 15, 10).
		Date("fecha", 25, 8)

	// --- Z23YYYY: Saldos Inventario ---
	r.tables["saldos_inventario"] = NewTable("saldos_inventario", r.path("Z23", true), 441).
		String("empresa", 0, 5).
		Key("codigo", 5, 6).
		String("bodega", 11, 3).
		String("cantidad", 14, 10)

	// --- Z279CP: Clasificación Cuentas ---
	r.tables["clasificacion_cuentas"] = NewTable("clasificacion_cuentas", r.path("Z279CP", false), 128).
		Key("codigo", 0, 9).
		String("nombre", 9, 40).
		String("tipo", 49, 1)

	// --- Z27YYYYA: Activos Fijos Detalle ---
	r.tables["activos_fijos_detalle"] = NewTable("activos_fijos_detalle", r.path("Z27", true)+"A", 1536).
		String("empresa", 0, 5).
		Key("codigo", 5, 6).
		String("nombre", 11, 50).
		String("nit", 61, 13).
		Date("fecha", 122, 8).
		BCD("valor_compra", 130, 8, 2).
		String("ubicacion", 586, 46).
		String("referencia", 736, 8)

	// --- Z11NYYYY: Audit Trail Terceros ---
	r.tables["audit_trail_terceros"] = NewTable("audit_trail_terceros", r.path("Z11N", true), 846).
		Date("fecha_cambio", 0, 8).
		Key("nit", 16, 13).
		Date("timestamp", 41, 8).
		String("usuario", 48, 5).
		Date("fecha_periodo", 56, 8).
		String("tipo_doc", 80, 2).
		String("nombre", 82, 40).
		String("nit_rep", 142, 16).
		String("nom_rep", 158, 40).
		String("direccion", 250, 40).
		String("email", 391, 47)
}
