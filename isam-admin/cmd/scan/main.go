package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"isam-admin/pkg/isam"

	"golang.org/x/text/encoding/charmap"
)

func printable(data []byte, max int) string {
	if len(data) < max {
		max = len(data)
	}
	buf := make([]byte, max)
	for i := 0; i < max; i++ {
		if data[i] >= 32 && data[i] < 127 {
			buf[i] = data[i]
		} else {
			buf[i] = '.'
		}
	}
	return string(buf)
}

type FileResult struct {
	Name     string
	RecSize  int
	Records  int
	Category string
	Desc     string
	Samples  []string
}

func classify(name string) (string, string) {
	upper := strings.ToUpper(name)

	switch {
	case upper == "Z17":
		return "TERCEROS", "Maestro de terceros (clientes/proveedores/empleados)"
	case upper == "Z17B" || upper == "Z17EST":
		return "TERCEROS_AUX", "Terceros auxiliar/estado"
	case strings.HasPrefix(upper, "Z03") && strings.HasSuffix(upper, "A"):
		return "PLAN_CUENTAS_AUX", "Plan de cuentas auxiliar"
	case strings.HasPrefix(upper, "Z03") && len(upper) > 3:
		return "PLAN_CUENTAS", "Plan de cuentas contable"
	case upper == "Z04AD":
		return "INVENTARIO_CONFIG", "Config inventario"
	case strings.HasPrefix(upper, "Z04") && strings.HasSuffix(upper, "A"):
		return "INVENTARIO_DET", "Inventario detalle/auxiliar"
	case strings.HasPrefix(upper, "Z04"):
		return "INVENTARIO", "Productos/inventario"
	case strings.HasPrefix(upper, "Z05"):
		return "COND_PAGO", "Condiciones de pago"
	case upper == "Z06" || upper == "Z06A" || strings.HasPrefix(upper, "Z0620"):
		return "MAESTROS", "Maestros generales (comprobantes, ciudades, etc)"
	case upper == "Z06MCCO" || upper == "Z06MCON":
		return "MAESTROS_CONFIG", "Config maestros"
	case upper == "Z07T":
		return "TRANS_DETALLE", "Transacciones detalle"
	case upper == "Z07S":
		return "LIBROS_RESUMEN", "Resumen libros auxiliares"
	case upper == "Z07C":
		return "LIBROS_CONFIG", "Config libros"
	case strings.HasPrefix(upper, "Z07PE"):
		return "PERIODOS_LIBROS", "Periodos contables libros"
	case strings.HasPrefix(upper, "Z07") && strings.Contains(upper, "HOY"):
		return "LIBROS_AUX_BACKUP", "Backup libros auxiliares"
	case strings.HasPrefix(upper, "Z07"):
		return "LIBROS_AUX", "Libros auxiliares contables"
	case upper == "Z088E" || upper == "Z088F":
		return "TERCEROS_CONF", "Config terceros especiales"
	case upper == "Z08DAT":
		return "TERCEROS_DATOS", "Datos adicionales terceros"
	case strings.HasPrefix(upper, "Z08") && strings.HasSuffix(upper, "A"):
		return "TERCEROS_AMP_AUX", "Terceros ampliados auxiliar"
	case strings.HasPrefix(upper, "Z08"):
		return "TERCEROS_AMP", "Terceros ampliados (datos adicionales)"
	case strings.HasPrefix(upper, "Z09CL"):
		return "CARTERA_CLASIF", "Clasificacion cartera"
	case strings.HasPrefix(upper, "Z09DLA"):
		return "CARTERA_DLA", "Cartera detalle liquidacion"
	case upper == "Z09B2011":
		return "CARTERA_BACKUP", "Backup cartera"
	case upper == "Z09H":
		return "CARTERA_HIST", "Historial cartera"
	case strings.HasPrefix(upper, "Z09") && strings.HasSuffix(upper, "A"):
		return "CARTERA_AUX", "Cartera auxiliar"
	case strings.HasPrefix(upper, "Z09"):
		return "CARTERA", "Cartera (cuentas por cobrar/pagar)"
	case strings.HasPrefix(upper, "Z10"):
		return "MOV_INVENTARIO_10", "Movimientos inventario tipo 10"
	case upper == "Z11IN2011":
		return "DOCS_INV_NOTA", "Notas documentos inventario"
	case strings.HasPrefix(upper, "Z11I"):
		return "DOCS_INVENTARIO", "Documentos de inventario"
	case strings.HasPrefix(upper, "Z11N"):
		return "AUDIT_TRAIL", "Audit trail terceros"
	case upper == "Z11K2011":
		return "DOCS_CONFIG", "Config documentos"
	case upper == "Z11L":
		return "DOCS_LOG", "Log documentos"
	case upper == "Z11P":
		return "DOCS_PARAM", "Parametros documentos"
	case upper == "Z11U":
		return "DOCS_USUARIOS", "Usuarios documentos"
	case strings.HasPrefix(upper, "Z11"):
		return "DOCUMENTOS", "Documentos contables (comprobantes)"
	case upper == "Z12" || upper == "Z120" || upper == "Z120SIIGO" || upper == "Z121" || upper == "Z12101" || upper == "Z122" || upper == "Z12201" || upper == "Z123":
		return "CONFIG_SISTEMA", "Configuracion del sistema / formatos"
	case strings.HasPrefix(upper, "Z14"):
		return "MOV_TIPO14", "Movimientos tipo 14"
	case strings.HasPrefix(upper, "Z15"):
		return "COSTOS_INV", "Costos de inventario"
	case strings.HasPrefix(upper, "Z16") && strings.HasSuffix(upper, "A"):
		return "MOV_INV_DET_AUX", "Mov inventario detalle auxiliar"
	case strings.HasPrefix(upper, "Z16"):
		return "MOV_INV_DETALLE", "Movimientos inventario detalle"
	case strings.HasPrefix(upper, "Z18"):
		return "HISTORIAL", "Historial de documentos"
	case upper == "Z19" || upper == "Z19A":
		return "PERIODOS_USUARIO", "Periodos/usuarios"
	case strings.HasPrefix(upper, "Z20"):
		return "NOMINA", "Nomina"
	case strings.HasPrefix(upper, "Z21"):
		return "NOMINA_DET", "Nomina detalle"
	case strings.HasPrefix(upper, "Z22"):
		return "NOMINA_PROV", "Provisiones nomina"
	case strings.HasPrefix(upper, "Z23"):
		return "SALDOS_INV", "Saldos de inventario"
	case upper == "Z24" || upper == "Z24T" || upper == "Z24C" || upper == "Z24N" || upper == "Z244T":
		return "PRESUPUESTO", "Presupuesto"
	case strings.HasPrefix(upper, "Z25") && strings.Contains(upper, "HOY"):
		return "SALDOS_TERC_BK", "Backup saldos terceros"
	case strings.HasPrefix(upper, "Z25"):
		return "SALDOS_TERCEROS", "Saldos por tercero (BCD)"
	case strings.HasPrefix(upper, "Z26") && strings.HasSuffix(upper, "A"):
		return "PERIODOS_AUX", "Periodos contables auxiliar"
	case strings.HasPrefix(upper, "Z26"):
		return "PERIODOS", "Periodos contables"
	case strings.HasPrefix(upper, "Z279CP"):
		return "CLASIF_CUENTAS", "Clasificacion cuentas"
	case strings.HasPrefix(upper, "Z279CT"):
		return "CLASIF_CUENTAS_T", "Clasificacion cuentas tipo"
	case upper == "Z27M":
		return "ACTIVOS_MAESTRO", "Maestro activos fijos"
	case strings.HasPrefix(upper, "Z27") && strings.HasSuffix(upper, "A"):
		return "ACTIVOS_FIJOS_DET", "Activos fijos detalle"
	case strings.HasPrefix(upper, "Z27"):
		return "ACTIVOS_FIJOS", "Activos fijos"
	case strings.HasPrefix(upper, "Z28"):
		return "SALDOS_CONSOL", "Saldos consolidados (BCD)"
	case upper == "Z34":
		return "RETENCION", "Retencion en la fuente"
	case strings.HasPrefix(upper, "Z49A"):
		return "MOVIMIENTOS_AUX", "Movimientos auxiliar"
	case strings.HasPrefix(upper, "Z49"):
		return "MOVIMIENTOS", "Movimientos (indices documentos)"
	case upper == "Z502012" || upper == "Z51":
		return "PUNTO_VENTA", "Punto de venta"
	case upper == "Z522012" || upper == "Z532012":
		return "PV_DETALLE", "Punto de venta detalle"
	case strings.HasPrefix(upper, "Z54"):
		return "TEMPORAL", "Archivos temporales"
	case strings.HasPrefix(upper, "Z55"):
		return "CENTROS_COSTO", "Centros de costo"
	case upper == "Z56" || upper == "Z57" || upper == "Z58":
		return "CONFIG_ADICIONAL", "Config adicional sistema"
	case strings.HasPrefix(upper, "Z59"):
		return "ANTICIPOS", "Anticipos"
	case upper == "Z600NPS":
		return "PLANEACION", "Planeacion Siigo"
	case upper == "Z70" || upper == "Z73":
		return "CONFIG_70", "Config especial"
	case strings.HasPrefix(upper, "Z80"):
		return "MEDIOS_MAG", "Medios magneticos (informacion exogena)"
	case upper == "Z88":
		return "CONFIG_88", "Config especial"
	case strings.HasPrefix(upper, "Z90"):
		return "CONFIG_MODULOS", "Configuracion de modulos"
	case upper == "Z91PRO":
		return "MENU_PROGRAMAS", "Menu de programas del sistema"
	case strings.HasPrefix(upper, "Z92"):
		return "CONFIG_REPORTES", "Config reportes"
	case upper == "Z95":
		return "CONFIG_95", "Config sistema"
	case upper == "Z001" || upper == "Z003":
		return "EMPRESA", "Datos de la empresa"
	case upper == "Z019CM":
		return "LICENCIAS", "Licencias/estaciones"
	case upper == "ZDANE":
		return "DANE", "Codigos DANE municipios"
	case upper == "ZICA":
		return "ICA", "Actividades ICA"
	case upper == "ZPILA":
		return "PILA", "Conceptos PILA"
	case upper == "ZPRGN" || upper == "AZPRGN":
		return "PROGRAMACION", "Programacion/config usuarios"
	case strings.HasPrefix(upper, "CHGFM"):
		return "CHANGELOG_FM", "Changelog formatos"
	case strings.HasPrefix(upper, "CHGTAR"):
		return "CHANGELOG_TAR", "Changelog tarifas"
	case upper == "INF":
		return "INFORMES", "Definicion de informes"
	case strings.HasPrefix(upper, "T941"):
		return "TEMPORALES", "Archivos temporales de procesamiento"
	case strings.HasPrefix(upper, "W08"):
		return "WORK_TERCEROS", "Working terceros ampliados"
	case strings.HasPrefix(upper, "W27"):
		return "WORK_ACTIVOS", "Working activos fijos"
	case upper == "TEMDLA" || upper == "TF09" || upper == "TMOV" || upper == "TZ09PE":
		return "TEMPORALES", "Archivos temporales"
	case strings.HasPrefix(upper, "ZF07") || strings.HasPrefix(upper, "ZF09"):
		return "FISCAL", "Datos fiscales"
	case upper == "ZFLUJ":
		return "FLUJO_CAJA", "Flujo de caja"
	case upper == "ZH02":
		return "HISTORIAL_02", "Historial tipo 02"
	case upper == "ZLOG":
		return "LOG_SISTEMA", "Log del sistema"
	case upper == "ZM06" || upper == "ZM27":
		return "MAESTROS_MIGR", "Migracion maestros"
	case upper == "ZPAIS":
		return "PAISES", "Tabla de paises"
	case strings.HasPrefix(upper, "ZQUE"):
		return "COLAS", "Colas de procesamiento"
	case upper == "ZSUBCTA":
		return "SUBCUENTAS", "Subcuentas contables"
	case strings.HasPrefix(upper, "ZW0"):
		return "CONFIG_WEB", "Config web/workflow"
	case upper == "ZCTA000":
		return "CUENTAS_BASE", "Cuentas base"
	case upper == "ZCUBOS":
		return "CUBOS", "Cubos analiticos"
	case upper == "ZCTRFECHA":
		return "CONTROL_FECHA", "Control de fechas"
	case upper == "ZIVA":
		return "IVA", "Tarifas IVA"
	case upper == "ZINSPYM":
		return "INSPECCION", "Inspeccion Pyme"
	case upper == "ZAIU":
		return "AIU", "AIU (Admon, Imprevistos, Utilidad)"
	case upper == "Z9001ES" || upper == "Z9001RH" || upper == "Z9001RHA":
		return "CONFIG_MODULOS", "Config modulos especiales"
	case strings.HasPrefix(upper, "C1") || upper == "C05" || upper == "P35":
		return "CONFIG_COBOL", "Config COBOL interna"
	case upper == "Z06" || strings.HasPrefix(upper, "Z06"):
		return "MAESTROS", "Maestros generales"
	}
	return "???_REVISAR", "Necesita revision manual"
}

func main() {
	dir := `C:\Archivos Siigo`
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	var results []FileResult

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		lower := strings.ToLower(ext)
		if lower == ".idx" || lower == ".prn" || lower == ".cfg" || lower == ".dis" || lower == ".txt" || lower == ".gnt" || lower == ".rs" {
			continue
		}

		fullPath := filepath.Join(dir, name)
		info, _ := e.Info()
		if info == nil || info.Size() <= 128 {
			continue
		}

		fi, hdr, err := isam.ReadIsamFile(fullPath)
		if err != nil || len(fi.Records) == 0 {
			continue
		}

		var samples []string
		for i := 0; i < 3 && i < len(fi.Records); i++ {
			raw := fi.Records[i].Data
			decoded, _ := charmap.Windows1252.NewDecoder().Bytes(raw)
			samples = append(samples, printable(decoded, 150))
		}

		cat, desc := classify(name)
		results = append(results, FileResult{
			Name:     name,
			RecSize:  int(hdr.MaxRecordLen),
			Records:  len(fi.Records),
			Category: cat,
			Desc:     desc,
			Samples:  samples,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return results[i].Category < results[j].Category
		}
		return results[i].Records > results[j].Records
	})

	currentCat := ""
	totalFiles := 0
	totalRecords := 0
	for _, r := range results {
		if r.Category != currentCat {
			fmt.Printf("\n=== %s ===\n", r.Category)
			currentCat = r.Category
		}
		fmt.Printf("  %-25s %6d recs  %5d recsize  %s\n", r.Name, r.Records, r.RecSize, r.Desc)
		if len(r.Samples) > 0 {
			fmt.Printf("    Sample: %.130s\n", r.Samples[0])
		}
		totalFiles++
		totalRecords += r.Records
	}
	fmt.Printf("\n\nTOTAL: %d archivos ISAM, %d registros totales\n", totalFiles, totalRecords)
}
