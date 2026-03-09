package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

var validTables = map[string]bool{
	"clients": true, "products": true, "movements": true, "cartera": true,
	"plan_cuentas": true, "activos_fijos": true, "saldos_terceros": true, "saldos_consolidados": true,
	"documentos": true, "terceros_ampliados": true,
	"transacciones_detalle": true, "periodos_contables": true, "condiciones_pago": true,
	"libros_auxiliares": true, "codigos_dane": true, "actividades_ica": true,
	"conceptos_pila": true, "activos_fijos_detalle": true, "audit_trail_terceros": true,
	"clasificacion_cuentas": true,
	"sync_history": true, "logs": true,
}

func isValidTable(table string) bool {
	return validTables[table]
}

// ==================== TYPED RECORDS (mirror Siigo tables) ====================

type ClientRecord struct {
	ID           int64  `json:"id"`
	Nit          string `json:"nit"`
	Nombre       string `json:"nombre"`
	TipoPersona  string `json:"tipo_persona"`
	Empresa      string `json:"empresa"`
	Direccion    string `json:"direccion"`
	Email        string `json:"email"`
	RepLegal     string `json:"rep_legal"`
	Hash         string `json:"hash"`
	SyncStatus   string `json:"sync_status"`
	SyncAction   string `json:"sync_action"`
	SyncError    string `json:"sync_error"`
	UpdatedAt    string `json:"updated_at"`
	SyncedAt     string `json:"synced_at"`
}

type ProductRecord struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Nombre      string `json:"nombre"`
	NombreCorto string `json:"nombre_corto"`
	Grupo       string `json:"grupo"`
	Referencia  string `json:"referencia"`
	Empresa     string `json:"empresa"`
	Hash        string `json:"hash"`
	SyncStatus  string `json:"sync_status"`
	SyncAction  string `json:"sync_action"`
	SyncError   string `json:"sync_error"`
	UpdatedAt   string `json:"updated_at"`
	SyncedAt    string `json:"synced_at"`
}

type MovementRecord struct {
	ID              int64  `json:"id"`
	RecordKey       string `json:"record_key"`
	TipoComprobante string `json:"tipo_comprobante"`
	Empresa         string `json:"empresa"`
	NumeroDoc       string `json:"numero_doc"`
	Fecha           string `json:"fecha"`
	NitTercero      string `json:"nit_tercero"`
	CuentaContable  string `json:"cuenta_contable"`
	Descripcion     string `json:"descripcion"`
	Valor           string `json:"valor"`
	TipoMov         string `json:"tipo_mov"`
	Hash            string `json:"hash"`
	SyncStatus      string `json:"sync_status"`
	SyncAction      string `json:"sync_action"`
	SyncError       string `json:"sync_error"`
	UpdatedAt       string `json:"updated_at"`
	SyncedAt        string `json:"synced_at"`
}

type CarteraRecord struct {
	ID             int64  `json:"id"`
	RecordKey      string `json:"record_key"`
	TipoRegistro   string `json:"tipo_registro"`
	Empresa        string `json:"empresa"`
	Secuencia      string `json:"secuencia"`
	TipoDoc        string `json:"tipo_doc"`
	NitTercero     string `json:"nit_tercero"`
	CuentaContable string `json:"cuenta_contable"`
	Fecha          string `json:"fecha"`
	Descripcion    string `json:"descripcion"`
	TipoMov        string `json:"tipo_mov"`
	Hash           string `json:"hash"`
	SyncStatus     string `json:"sync_status"`
	SyncAction     string `json:"sync_action"`
	SyncError      string `json:"sync_error"`
	UpdatedAt      string `json:"updated_at"`
	SyncedAt       string `json:"synced_at"`
}

type PlanCuentasRecord struct {
	ID             int64  `json:"id"`
	CodigoCuenta   string `json:"codigo_cuenta"`
	Nombre         string `json:"nombre"`
	Empresa        string `json:"empresa"`
	Activa         bool   `json:"activa"`
	Auxiliar       bool   `json:"auxiliar"`
	Naturaleza     string `json:"naturaleza"`
	Hash           string `json:"hash"`
	SyncStatus     string `json:"sync_status"`
	SyncAction     string `json:"sync_action"`
	SyncError      string `json:"sync_error"`
	UpdatedAt      string `json:"updated_at"`
	SyncedAt       string `json:"synced_at"`
}

type ActivoFijoRecord struct {
	ID               int64  `json:"id"`
	Codigo           string `json:"codigo"`
	Nombre           string `json:"nombre"`
	Empresa          string `json:"empresa"`
	NitResponsable   string `json:"nit_responsable"`
	FechaAdquisicion string `json:"fecha_adquisicion"`
	Hash             string `json:"hash"`
	SyncStatus       string `json:"sync_status"`
	SyncAction       string `json:"sync_action"`
	SyncError        string `json:"sync_error"`
	UpdatedAt        string `json:"updated_at"`
	SyncedAt         string `json:"synced_at"`
}

type SaldoTerceroRecord struct {
	ID             int64   `json:"id"`
	RecordKey      string  `json:"record_key"`
	CuentaContable string  `json:"cuenta_contable"`
	NitTercero     string  `json:"nit_tercero"`
	Empresa        string  `json:"empresa"`
	SaldoAnterior  float64 `json:"saldo_anterior"`
	Debito         float64 `json:"debito"`
	Credito        float64 `json:"credito"`
	SaldoFinal     float64 `json:"saldo_final"`
	Hash           string  `json:"hash"`
	SyncStatus     string  `json:"sync_status"`
	SyncAction     string  `json:"sync_action"`
	SyncError      string  `json:"sync_error"`
	UpdatedAt      string  `json:"updated_at"`
	SyncedAt       string  `json:"synced_at"`
}

type SaldoConsolidadoRecord struct {
	ID             int64   `json:"id"`
	CuentaContable string  `json:"cuenta_contable"`
	Empresa        string  `json:"empresa"`
	SaldoAnterior  float64 `json:"saldo_anterior"`
	Debito         float64 `json:"debito"`
	Credito        float64 `json:"credito"`
	SaldoFinal     float64 `json:"saldo_final"`
	Hash           string  `json:"hash"`
	SyncStatus     string  `json:"sync_status"`
	SyncAction     string  `json:"sync_action"`
	SyncError      string  `json:"sync_error"`
	UpdatedAt      string  `json:"updated_at"`
	SyncedAt       string  `json:"synced_at"`
}

type DocumentoRecord struct {
	ID              int64  `json:"id"`
	RecordKey       string `json:"record_key"`
	TipoComprobante string `json:"tipo_comprobante"`
	CodigoComp      string `json:"codigo_comp"`
	Secuencia       string `json:"secuencia"`
	NitTercero      string `json:"nit_tercero"`
	CuentaContable  string `json:"cuenta_contable"`
	ProductoRef     string `json:"producto_ref"`
	Bodega          string `json:"bodega"`
	CentroCosto     string `json:"centro_costo"`
	Fecha           string `json:"fecha"`
	Descripcion     string `json:"descripcion"`
	TipoMov         string `json:"tipo_mov"`
	Referencia      string `json:"referencia"`
	Hash            string `json:"hash"`
	SyncStatus      string `json:"sync_status"`
	SyncAction      string `json:"sync_action"`
	SyncError       string `json:"sync_error"`
	UpdatedAt       string `json:"updated_at"`
	SyncedAt        string `json:"synced_at"`
}

type TerceroAmpliadoRecord struct {
	ID                 int64  `json:"id"`
	Nit                string `json:"nit"`
	Nombre             string `json:"nombre"`
	Empresa            string `json:"empresa"`
	TipoPersona        string `json:"tipo_persona"`
	RepresentanteLegal string `json:"representante_legal"`
	Direccion          string `json:"direccion"`
	Email              string `json:"email"`
	Hash               string `json:"hash"`
	SyncStatus         string `json:"sync_status"`
	SyncAction         string `json:"sync_action"`
	SyncError          string `json:"sync_error"`
	UpdatedAt          string `json:"updated_at"`
	SyncedAt           string `json:"synced_at"`
}

// ==================== NEW TABLE RECORDS ====================

type TransaccionDetalleRecord struct {
	ID                int64   `json:"id"`
	RecordKey         string  `json:"record_key"`
	TipoComprobante   string  `json:"tipo_comprobante"`
	Empresa           string  `json:"empresa"`
	Secuencia         string  `json:"secuencia"`
	NitTercero        string  `json:"nit_tercero"`
	CuentaContable    string  `json:"cuenta_contable"`
	FechaDocumento    string  `json:"fecha_documento"`
	FechaVencimiento  string  `json:"fecha_vencimiento"`
	TipoMovimiento    string  `json:"tipo_movimiento"`
	Valor             float64 `json:"valor"`
	Referencia        string  `json:"referencia"`
	Hash              string  `json:"hash"`
	SyncStatus        string  `json:"sync_status"`
	SyncAction        string  `json:"sync_action"`
	SyncError         string  `json:"sync_error"`
	UpdatedAt         string  `json:"updated_at"`
	SyncedAt          string  `json:"synced_at"`
}

type PeriodoContableRecord struct {
	ID            int64   `json:"id"`
	RecordKey     string  `json:"record_key"`
	Empresa       string  `json:"empresa"`
	NumeroPeriodo string  `json:"numero_periodo"`
	FechaInicio   string  `json:"fecha_inicio"`
	FechaFin      string  `json:"fecha_fin"`
	Estado        string  `json:"estado"`
	Saldo1        float64 `json:"saldo1"`
	Saldo2        float64 `json:"saldo2"`
	Saldo3        float64 `json:"saldo3"`
	Hash          string  `json:"hash"`
	SyncStatus    string  `json:"sync_status"`
	SyncAction    string  `json:"sync_action"`
	SyncError     string  `json:"sync_error"`
	UpdatedAt     string  `json:"updated_at"`
	SyncedAt      string  `json:"synced_at"`
}

type CondicionPagoRecord struct {
	ID             int64   `json:"id"`
	RecordKey      string  `json:"record_key"`
	Tipo           string  `json:"tipo"`
	Empresa        string  `json:"empresa"`
	Secuencia      string  `json:"secuencia"`
	TipoDoc        string  `json:"tipo_doc"`
	Fecha          string  `json:"fecha"`
	NIT            string  `json:"nit"`
	TipoSecundario string  `json:"tipo_secundario"`
	Valor          float64 `json:"valor"`
	FechaRegistro  string  `json:"fecha_registro"`
	Hash           string  `json:"hash"`
	SyncStatus     string  `json:"sync_status"`
	SyncAction     string  `json:"sync_action"`
	SyncError      string  `json:"sync_error"`
	UpdatedAt      string  `json:"updated_at"`
	SyncedAt       string  `json:"synced_at"`
}

type LibroAuxiliarRecord struct {
	ID                int64   `json:"id"`
	RecordKey         string  `json:"record_key"`
	Empresa           string  `json:"empresa"`
	CuentaContable    string  `json:"cuenta_contable"`
	TipoComprobante   string  `json:"tipo_comprobante"`
	CodigoComprobante string  `json:"codigo_comprobante"`
	FechaDocumento    string  `json:"fecha_documento"`
	NitTercero        string  `json:"nit_tercero"`
	Saldo             float64 `json:"saldo"`
	Debito            float64 `json:"debito"`
	Credito           float64 `json:"credito"`
	Hash              string  `json:"hash"`
	SyncStatus        string  `json:"sync_status"`
	SyncAction        string  `json:"sync_action"`
	SyncError         string  `json:"sync_error"`
	UpdatedAt         string  `json:"updated_at"`
	SyncedAt          string  `json:"synced_at"`
}

type CodigoDaneRecord struct {
	ID         int64  `json:"id"`
	Codigo     string `json:"codigo"`
	Nombre     string `json:"nombre"`
	Hash       string `json:"hash"`
	SyncStatus string `json:"sync_status"`
	SyncAction string `json:"sync_action"`
	SyncError  string `json:"sync_error"`
	UpdatedAt  string `json:"updated_at"`
	SyncedAt   string `json:"synced_at"`
}

type ActividadICARecord struct {
	ID         int64  `json:"id"`
	Codigo     string `json:"codigo"`
	Nombre     string `json:"nombre"`
	Tarifa     string `json:"tarifa"`
	Hash       string `json:"hash"`
	SyncStatus string `json:"sync_status"`
	SyncAction string `json:"sync_action"`
	SyncError  string `json:"sync_error"`
	UpdatedAt  string `json:"updated_at"`
	SyncedAt   string `json:"synced_at"`
}

type ConceptoPILARecord struct {
	ID          int64  `json:"id"`
	RecordKey   string `json:"record_key"`
	Tipo        string `json:"tipo"`
	Fondo       string `json:"fondo"`
	Concepto    string `json:"concepto"`
	Flags       string `json:"flags"`
	TipoBase    string `json:"tipo_base"`
	BaseCalculo string `json:"base_calculo"`
	Hash        string `json:"hash"`
	SyncStatus  string `json:"sync_status"`
	SyncAction  string `json:"sync_action"`
	SyncError   string `json:"sync_error"`
	UpdatedAt   string `json:"updated_at"`
	SyncedAt    string `json:"synced_at"`
}

type ActivoFijoDetalleRecord struct {
	ID             int64   `json:"id"`
	RecordKey      string  `json:"record_key"`
	Grupo          string  `json:"grupo"`
	Secuencia      string  `json:"secuencia"`
	Nombre         string  `json:"nombre"`
	NitResponsable string  `json:"nit_responsable"`
	Codigo         string  `json:"codigo"`
	Fecha          string  `json:"fecha"`
	ValorCompra    float64 `json:"valor_compra"`
	Hash           string  `json:"hash"`
	SyncStatus     string  `json:"sync_status"`
	SyncAction     string  `json:"sync_action"`
	SyncError      string  `json:"sync_error"`
	UpdatedAt      string  `json:"updated_at"`
	SyncedAt       string  `json:"synced_at"`
}

type AuditTrailTerceroRecord struct {
	ID                  int64  `json:"id"`
	RecordKey           string `json:"record_key"`
	FechaCambio         string `json:"fecha_cambio"`
	NitTercero          string `json:"nit_tercero"`
	Timestamp           string `json:"timestamp"`
	Usuario             string `json:"usuario"`
	FechaPeriodo        string `json:"fecha_periodo"`
	TipoDoc             string `json:"tipo_doc"`
	Nombre              string `json:"nombre"`
	NitRepresentante    string `json:"nit_representante"`
	NombreRepresentante string `json:"nombre_representante"`
	Hash                string `json:"hash"`
	SyncStatus          string `json:"sync_status"`
	SyncAction          string `json:"sync_action"`
	SyncError           string `json:"sync_error"`
	UpdatedAt           string `json:"updated_at"`
	SyncedAt            string `json:"synced_at"`
}

type ClasificacionCuentaRecord struct {
	ID            int64  `json:"id"`
	CodigoCuenta  string `json:"codigo_cuenta"`
	CodigoGrupo   string `json:"codigo_grupo"`
	CodigoDetalle string `json:"codigo_detalle"`
	Descripcion   string `json:"descripcion"`
	Hash          string `json:"hash"`
	SyncStatus    string `json:"sync_status"`
	SyncAction    string `json:"sync_action"`
	SyncError     string `json:"sync_error"`
	UpdatedAt     string `json:"updated_at"`
	SyncedAt      string `json:"synced_at"`
}

// PendingRecord is a generic record ready to send to the API
type PendingRecord struct {
	ID         int64
	Key        string
	SyncAction string
	Data       map[string]interface{}
}

// SyncRecord is used for sync_history display
type SyncRecord struct {
	ID         int64  `json:"id"`
	Table      string `json:"table"`
	Key        string `json:"key"`
	Data       string `json:"data"`
	Hash       string `json:"hash"`
	SyncStatus string `json:"sync_status"`
	SyncError  string `json:"sync_error"`
	SyncAction string `json:"sync_action"`
	UpdatedAt  string `json:"updated_at"`
	SyncedAt   string `json:"synced_at"`
}

type LogEntry struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) migrate() error {
	queries := []string{
		// Clients table (from Z08A - Terceros Ampliados)
		`CREATE TABLE IF NOT EXISTS clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nit TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			tipo_clave TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			codigo TEXT DEFAULT '',
			fecha_creacion TEXT DEFAULT '',
			tipo_cta_pref TEXT DEFAULT '',
			tipo_persona TEXT DEFAULT '',
			direccion TEXT DEFAULT '',
			email TEXT DEFAULT '',
			rep_legal TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_nit ON clients(nit)`,

		// Products table (from Z04YYYY - real inventory)
		`CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			nombre_corto TEXT DEFAULT '',
			grupo TEXT DEFAULT '',
			referencia TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_products_status ON products(sync_status)`,

		// Movements table (from Z49)
		`CREATE TABLE IF NOT EXISTS movements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_comprobante TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			numero_doc TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			valor TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_movements_status ON movements(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_movements_nit ON movements(nit_tercero)`,

		// Cartera table (from Z09YYYY)
		`CREATE TABLE IF NOT EXISTS cartera (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_registro TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cartera_status ON cartera(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_cartera_nit ON cartera(nit_tercero)`,

		// Sync history log
		`CREATE TABLE IF NOT EXISTS sync_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			record_key TEXT NOT NULL,
			action TEXT NOT NULL,
			data TEXT,
			status TEXT DEFAULT 'sent',
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_table ON sync_history(table_name)`,
		`CREATE INDEX IF NOT EXISTS idx_history_status ON sync_history(status)`,

		// App logs
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT DEFAULT 'info',
			source TEXT,
			message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_source ON logs(source)`,

		// Drop old generic table if it exists (migration from previous schema)
		`DROP TABLE IF EXISTS siigo_records`,

		// Add retry_count column to all tables (migration)
		`ALTER TABLE clients ADD COLUMN retry_count INTEGER DEFAULT 0`,
		`ALTER TABLE products ADD COLUMN retry_count INTEGER DEFAULT 0`,
		`ALTER TABLE movements ADD COLUMN retry_count INTEGER DEFAULT 0`,
		`ALTER TABLE cartera ADD COLUMN retry_count INTEGER DEFAULT 0`,

		// Migration: clients from Z17 to Z08A (add ampliado fields)
		`ALTER TABLE clients ADD COLUMN tipo_persona TEXT DEFAULT ''`,
		`ALTER TABLE clients ADD COLUMN direccion TEXT DEFAULT ''`,
		`ALTER TABLE clients ADD COLUMN email TEXT DEFAULT ''`,
		`ALTER TABLE clients ADD COLUMN rep_legal TEXT DEFAULT ''`,

		// Migration: products table from Z06CP (comprobantes) to Z04 (real inventory)
		`ALTER TABLE products ADD COLUMN nombre_corto TEXT DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN referencia TEXT DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN empresa TEXT DEFAULT ''`,

		// App users table (for multi-user with roles/permissions)
		`CREATE TABLE IF NOT EXISTS app_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			permissions TEXT DEFAULT '[]',
			active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Audit trail
		`CREATE TABLE IF NOT EXISTS audit_trail (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			action TEXT NOT NULL,
			table_name TEXT DEFAULT '',
			record_id TEXT DEFAULT '',
			details TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_trail(created_at)`,

		// Change history (field-level diffs during sync)
		`CREATE TABLE IF NOT EXISTS change_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			record_key TEXT NOT NULL,
			changes TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_changes_key ON change_history(table_name, record_key)`,

		// Plan de Cuentas table (from Z03YYYY)
		`CREATE TABLE IF NOT EXISTS plan_cuentas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			codigo_cuenta TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			empresa TEXT DEFAULT '',
			activa INTEGER DEFAULT 1,
			auxiliar INTEGER DEFAULT 0,
			naturaleza TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plan_cuentas_status ON plan_cuentas(sync_status)`,

		// Activos Fijos table (from Z27YYYY)
		`CREATE TABLE IF NOT EXISTS activos_fijos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			codigo TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			empresa TEXT DEFAULT '',
			nit_responsable TEXT DEFAULT '',
			fecha_adquisicion TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activos_fijos_status ON activos_fijos(sync_status)`,

		// Saldos por Tercero table (from Z25YYYY)
		`CREATE TABLE IF NOT EXISTS saldos_terceros (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			cuenta_contable TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			saldo_anterior REAL DEFAULT 0,
			debito REAL DEFAULT 0,
			credito REAL DEFAULT 0,
			saldo_final REAL DEFAULT 0,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_saldos_terceros_status ON saldos_terceros(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_saldos_terceros_nit ON saldos_terceros(nit_tercero)`,
		`CREATE INDEX IF NOT EXISTS idx_saldos_terceros_cuenta ON saldos_terceros(cuenta_contable)`,

		// Saldos Consolidados table (from Z28YYYY)
		`CREATE TABLE IF NOT EXISTS saldos_consolidados (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cuenta_contable TEXT NOT NULL UNIQUE,
			empresa TEXT DEFAULT '',
			saldo_anterior REAL DEFAULT 0,
			debito REAL DEFAULT 0,
			credito REAL DEFAULT 0,
			saldo_final REAL DEFAULT 0,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_saldos_consolidados_status ON saldos_consolidados(sync_status)`,

		// Documentos table (from Z11YYYY - invoice/voucher detail lines)
		`CREATE TABLE IF NOT EXISTS documentos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_comprobante TEXT DEFAULT '',
			codigo_comp TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			producto_ref TEXT DEFAULT '',
			bodega TEXT DEFAULT '',
			centro_costo TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			tipo_mov TEXT DEFAULT '',
			referencia TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_documentos_status ON documentos(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_documentos_nit ON documentos(nit_tercero)`,
		`CREATE INDEX IF NOT EXISTS idx_documentos_cuenta ON documentos(cuenta_contable)`,

		// Terceros Ampliados table (from Z08YYYYA - extended third-party data)
		`CREATE TABLE IF NOT EXISTS terceros_ampliados (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nit TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			empresa TEXT DEFAULT '',
			tipo_persona TEXT DEFAULT '',
			representante_legal TEXT DEFAULT '',
			direccion TEXT DEFAULT '',
			email TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_terceros_ampliados_status ON terceros_ampliados(sync_status)`,

		// Transacciones Detalle table (from Z07T - transaction detail lines)
		`CREATE TABLE IF NOT EXISTS transacciones_detalle (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo_comprobante TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			fecha_documento TEXT DEFAULT '',
			fecha_vencimiento TEXT DEFAULT '',
			tipo_movimiento TEXT DEFAULT '',
			valor REAL DEFAULT 0,
			referencia TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transacciones_detalle_status ON transacciones_detalle(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_transacciones_detalle_nit ON transacciones_detalle(nit_tercero)`,

		// Periodos Contables table (from Z26YYYY)
		`CREATE TABLE IF NOT EXISTS periodos_contables (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			empresa TEXT DEFAULT '',
			numero_periodo TEXT DEFAULT '',
			fecha_inicio TEXT DEFAULT '',
			fecha_fin TEXT DEFAULT '',
			estado TEXT DEFAULT '',
			saldo1 REAL DEFAULT 0,
			saldo2 REAL DEFAULT 0,
			saldo3 REAL DEFAULT 0,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_periodos_contables_status ON periodos_contables(sync_status)`,

		// Condiciones de Pago table (from Z05YYYY)
		`CREATE TABLE IF NOT EXISTS condiciones_pago (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo TEXT DEFAULT '',
			empresa TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			nit TEXT DEFAULT '',
			tipo_secundario TEXT DEFAULT '',
			valor REAL DEFAULT 0,
			fecha_registro TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_condiciones_pago_status ON condiciones_pago(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_condiciones_pago_nit ON condiciones_pago(nit)`,

		// Libros Auxiliares table (from Z07YYYY)
		`CREATE TABLE IF NOT EXISTS libros_auxiliares (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			empresa TEXT DEFAULT '',
			cuenta_contable TEXT DEFAULT '',
			tipo_comprobante TEXT DEFAULT '',
			codigo_comprobante TEXT DEFAULT '',
			fecha_documento TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			saldo REAL DEFAULT 0,
			debito REAL DEFAULT 0,
			credito REAL DEFAULT 0,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_libros_auxiliares_status ON libros_auxiliares(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_libros_auxiliares_cuenta ON libros_auxiliares(cuenta_contable)`,
		`CREATE INDEX IF NOT EXISTS idx_libros_auxiliares_nit ON libros_auxiliares(nit_tercero)`,

		// Codigos DANE table (from ZDANE)
		`CREATE TABLE IF NOT EXISTS codigos_dane (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			codigo TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_codigos_dane_status ON codigos_dane(sync_status)`,

		// Actividades ICA table (from ZICA)
		`CREATE TABLE IF NOT EXISTS actividades_ica (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			codigo TEXT NOT NULL UNIQUE,
			nombre TEXT NOT NULL DEFAULT '',
			tarifa TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_actividades_ica_status ON actividades_ica(sync_status)`,

		// Conceptos PILA table (from ZPILA)
		`CREATE TABLE IF NOT EXISTS conceptos_pila (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			tipo TEXT DEFAULT '',
			fondo TEXT DEFAULT '',
			concepto TEXT DEFAULT '',
			flags TEXT DEFAULT '',
			tipo_base TEXT DEFAULT '',
			base_calculo TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conceptos_pila_status ON conceptos_pila(sync_status)`,

		// Activos Fijos Detalle table (from Z27YYYY - detailed records)
		`CREATE TABLE IF NOT EXISTS activos_fijos_detalle (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			grupo TEXT DEFAULT '',
			secuencia TEXT DEFAULT '',
			nombre TEXT NOT NULL DEFAULT '',
			nit_responsable TEXT DEFAULT '',
			codigo TEXT DEFAULT '',
			fecha TEXT DEFAULT '',
			valor_compra REAL DEFAULT 0,
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activos_fijos_detalle_status ON activos_fijos_detalle(sync_status)`,

		// Audit Trail Terceros table (from Z11NYYYY)
		`CREATE TABLE IF NOT EXISTS audit_trail_terceros (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_key TEXT NOT NULL UNIQUE,
			fecha_cambio TEXT DEFAULT '',
			nit_tercero TEXT DEFAULT '',
			timestamp TEXT DEFAULT '',
			usuario TEXT DEFAULT '',
			fecha_periodo TEXT DEFAULT '',
			tipo_doc TEXT DEFAULT '',
			nombre TEXT DEFAULT '',
			nit_representante TEXT DEFAULT '',
			nombre_representante TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_trail_terceros_status ON audit_trail_terceros(sync_status)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_trail_terceros_nit ON audit_trail_terceros(nit_tercero)`,

		// Clasificacion Cuentas table (from Z279CPYY)
		`CREATE TABLE IF NOT EXISTS clasificacion_cuentas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			codigo_cuenta TEXT NOT NULL UNIQUE,
			codigo_grupo TEXT DEFAULT '',
			codigo_detalle TEXT DEFAULT '',
			descripcion TEXT DEFAULT '',
			hash TEXT NOT NULL,
			sync_status TEXT DEFAULT 'pending',
			sync_action TEXT DEFAULT 'add',
			sync_error TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_clasificacion_cuentas_status ON clasificacion_cuentas(sync_status)`,

		// Missing relationship indexes on ORIGINAL tables
		// movements → plan_cuentas, fecha
		`CREATE INDEX IF NOT EXISTS idx_movements_cuenta ON movements(cuenta_contable)`,
		`CREATE INDEX IF NOT EXISTS idx_movements_fecha ON movements(fecha)`,
		// cartera → plan_cuentas, fecha
		`CREATE INDEX IF NOT EXISTS idx_cartera_cuenta ON cartera(cuenta_contable)`,
		`CREATE INDEX IF NOT EXISTS idx_cartera_fecha ON cartera(fecha)`,
		// activos_fijos → clients via nit_responsable
		`CREATE INDEX IF NOT EXISTS idx_activos_fijos_nit ON activos_fijos(nit_responsable)`,
		// products → grupo lookup
		`CREATE INDEX IF NOT EXISTS idx_products_grupo ON products(grupo)`,
		// documentos → fecha, producto
		`CREATE INDEX IF NOT EXISTS idx_documentos_fecha ON documentos(fecha)`,
		`CREATE INDEX IF NOT EXISTS idx_documentos_producto ON documentos(producto_ref)`,

		// Cross-table relationship indexes on NEW tables
		// Transacciones → cuentas, terceros
		`CREATE INDEX IF NOT EXISTS idx_transacciones_detalle_cuenta ON transacciones_detalle(cuenta_contable)`,
		`CREATE INDEX IF NOT EXISTS idx_transacciones_detalle_fecha ON transacciones_detalle(fecha_documento)`,
		`CREATE INDEX IF NOT EXISTS idx_transacciones_detalle_tipo ON transacciones_detalle(tipo_comprobante)`,
		// Libros auxiliares → cuentas, terceros, comprobantes
		`CREATE INDEX IF NOT EXISTS idx_libros_auxiliares_tipo ON libros_auxiliares(tipo_comprobante)`,
		`CREATE INDEX IF NOT EXISTS idx_libros_auxiliares_fecha ON libros_auxiliares(fecha_documento)`,
		// Condiciones pago → terceros
		`CREATE INDEX IF NOT EXISTS idx_condiciones_pago_fecha ON condiciones_pago(fecha)`,
		// Activos fijos detalle → terceros
		`CREATE INDEX IF NOT EXISTS idx_activos_fijos_detalle_nit ON activos_fijos_detalle(nit_responsable)`,
		`CREATE INDEX IF NOT EXISTS idx_activos_fijos_detalle_grupo ON activos_fijos_detalle(grupo)`,
		// Audit trail → terceros, usuario
		`CREATE INDEX IF NOT EXISTS idx_audit_trail_terceros_usuario ON audit_trail_terceros(usuario)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_trail_terceros_fecha ON audit_trail_terceros(fecha_cambio)`,
		// Clasificacion → join con plan_cuentas
		`CREATE INDEX IF NOT EXISTS idx_clasificacion_cuentas_grupo ON clasificacion_cuentas(codigo_grupo)`,
		// Periodos → empresa
		`CREATE INDEX IF NOT EXISTS idx_periodos_contables_empresa ON periodos_contables(empresa)`,

		// Sync stats (for dashboard charts)
		`CREATE TABLE IF NOT EXISTS sync_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			total INTEGER DEFAULT 0,
			pending INTEGER DEFAULT 0,
			synced INTEGER DEFAULT 0,
			errors INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_syncstats_created ON sync_stats(created_at)`,

		// User preferences (dashboard layout, etc.)
		`CREATE TABLE IF NOT EXISTS user_preferences (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			pref_key TEXT NOT NULL,
			pref_value TEXT DEFAULT '{}',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(username, pref_key)
		)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			// Ignore ALTER TABLE errors (column may already exist)
			if strings.Contains(q, "ALTER TABLE") {
				continue
			}
			return err
		}
	}
	return nil
}

// ==================== CLIENTS ====================

func (db *DB) GetAllClientHashes() map[string]string {
	return db.getAllHashes("clients", "nit")
}

func (db *DB) UpsertClient(nit, nombre, tipoPersona, empresa, direccion, email, repLegal, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM clients WHERE nit=?`, nit).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO clients (nit, nombre, tipo_persona, empresa, direccion, email, rep_legal, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			nit, nombre, tipoPersona, empresa, direccion, email, repLegal, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		// Track field changes
		db.trackChanges("clients", nit, map[string]string{
			"nombre": nombre, "tipo_persona": tipoPersona,
			"empresa": empresa, "direccion": direccion, "email": email,
			"rep_legal": repLegal,
		})
		db.conn.Exec(
			`UPDATE clients SET nombre=?, tipo_persona=?, empresa=?, direccion=?, email=?, rep_legal=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE nit=?`,
			nombre, tipoPersona, empresa, direccion, email, repLegal, hash, now, nit,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedClients(currentKeys map[string]bool) int {
	return db.markDeleted("clients", "nit", currentKeys)
}

func (db *DB) GetPendingClients() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, nit, nombre, tipo_persona, empresa, direccion, email, rep_legal, sync_action
		 FROM clients WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var nit, nombre, tipoPersona, empresa, direccion, email, repLegal, action string
		if err := rows.Scan(&id, &nit, &nombre, &tipoPersona, &empresa, &direccion, &email, &repLegal, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: nit, SyncAction: action,
			Data: map[string]interface{}{
				"nit":           nit,
				"client_name":   nombre,
				"business_name": nombre,
				"tipo_persona":  tipoPersona,
				"siigo_empresa": empresa,
				"direccion":     direccion,
				"email":         email,
				"rep_legal":     repLegal,
			},
		})
	}
	return records
}

// ==================== PRODUCTS ====================

func (db *DB) GetAllProductHashes() map[string]string {
	return db.getAllHashes("products", "code")
}

func (db *DB) UpsertProduct(code, nombre, nombreCorto, grupo, referencia, empresa, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM products WHERE code=?`, code).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO products (code, nombre, nombre_corto, grupo, referencia, empresa, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			code, nombre, nombreCorto, grupo, referencia, empresa, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.trackChanges("products", code, map[string]string{
			"nombre": nombre, "nombre_corto": nombreCorto,
			"grupo": grupo, "referencia": referencia, "empresa": empresa,
		})
		db.conn.Exec(
			`UPDATE products SET nombre=?, nombre_corto=?, grupo=?, referencia=?, empresa=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE code=?`,
			nombre, nombreCorto, grupo, referencia, empresa, hash, now, code,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedProducts(currentKeys map[string]bool) int {
	return db.markDeleted("products", "code", currentKeys)
}

func (db *DB) GetPendingProducts() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, code, nombre, COALESCE(nombre_corto,''), grupo, COALESCE(referencia,''), sync_action
		 FROM products WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var code, nombre, nombreCorto, grupo, referencia, action string
		if err := rows.Scan(&id, &code, &nombre, &nombreCorto, &grupo, &referencia, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: code, SyncAction: action,
			Data: map[string]interface{}{
				"code":         code,
				"product_name": nombre,
				"grupo":        grupo,
				"referencia":   referencia,
			},
		})
	}
	return records
}

// ==================== MOVEMENTS ====================

func (db *DB) GetAllMovementHashes() map[string]string {
	return db.getAllHashes("movements", "record_key")
}

func (db *DB) UpsertMovement(key, tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM movements WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO movements (record_key, tipo_comprobante, empresa, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.trackChanges("movements", key, map[string]string{
			"tipo_comprobante": tipoComprobante, "empresa": empresa, "numero_doc": numeroDoc,
			"fecha": fecha, "nit_tercero": nitTercero, "cuenta_contable": cuentaContable,
			"descripcion": descripcion, "valor": valor, "tipo_mov": tipoMov,
		})
		db.conn.Exec(
			`UPDATE movements SET tipo_comprobante=?, empresa=?, numero_doc=?, fecha=?, nit_tercero=?, cuenta_contable=?, descripcion=?, valor=?, tipo_mov=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoComprobante, empresa, numeroDoc, fecha, nitTercero, cuentaContable, descripcion, valor, tipoMov, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedMovements(currentKeys map[string]bool) int {
	return db.markDeleted("movements", "record_key", currentKeys)
}

func (db *DB) GetPendingMovements() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, record_key, tipo_comprobante, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, sync_action
		 FROM movements WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var key, tipoComp, numDoc, fecha, nit, cuenta, desc, valor, tipoMov, action string
		if err := rows.Scan(&id, &key, &tipoComp, &numDoc, &fecha, &nit, &cuenta, &desc, &valor, &tipoMov, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: key, SyncAction: action,
			Data: map[string]interface{}{
				"tipo_comprobante": tipoComp,
				"numero_doc":       numDoc,
				"fecha":            fecha,
				"nit_tercero":      nit,
				"cuenta_contable":  cuenta,
				"descripcion":      desc,
				"valor":            valor,
				"tipo_mov":         tipoMov,
			},
		})
	}
	return records
}

// ==================== CARTERA ====================

func (db *DB) GetAllCarteraHashes() map[string]string {
	return db.getAllHashes("cartera", "record_key")
}

func (db *DB) UpsertCartera(key, tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM cartera WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO cartera (record_key, tipo_registro, empresa, secuencia, tipo_doc, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.trackChanges("cartera", key, map[string]string{
			"tipo_registro": tipoRegistro, "empresa": empresa, "secuencia": secuencia,
			"tipo_doc": tipoDoc, "nit_tercero": nitTercero, "cuenta_contable": cuentaContable,
			"fecha": fecha, "descripcion": descripcion, "tipo_mov": tipoMov,
		})
		db.conn.Exec(
			`UPDATE cartera SET tipo_registro=?, empresa=?, secuencia=?, tipo_doc=?, nit_tercero=?, cuenta_contable=?, fecha=?, descripcion=?, tipo_mov=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoRegistro, empresa, secuencia, tipoDoc, nitTercero, cuentaContable, fecha, descripcion, tipoMov, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedCartera(currentKeys map[string]bool) int {
	return db.markDeleted("cartera", "record_key", currentKeys)
}

func (db *DB) GetPendingCartera() []PendingRecord {
	rows, err := db.conn.Query(
		`SELECT id, record_key, tipo_registro, empresa, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, sync_action
		 FROM cartera WHERE sync_status='pending' ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var id int64
		var key, tipoReg, empresa, nit, cuenta, fecha, desc, tipoMov, action string
		if err := rows.Scan(&id, &key, &tipoReg, &empresa, &nit, &cuenta, &fecha, &desc, &tipoMov, &action); err != nil {
			continue
		}
		records = append(records, PendingRecord{
			ID: id, Key: key, SyncAction: action,
			Data: map[string]interface{}{
				"nit":             nit,
				"cuenta_contable": cuenta,
				"fecha":           fecha,
				"tipo_movimiento": tipoMov,
				"descripcion":     desc,
				"tipo_registro":   tipoReg,
			},
		})
	}
	return records
}

// ==================== PLAN DE CUENTAS (Z03) ====================

func (db *DB) UpsertPlanCuenta(codigoCuenta, nombre, empresa, naturaleza, hash string, activa, auxiliar bool) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM plan_cuentas WHERE codigo_cuenta=?`, codigoCuenta).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	activaInt := 0
	if activa {
		activaInt = 1
	}
	auxiliarInt := 0
	if auxiliar {
		auxiliarInt = 1
	}

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO plan_cuentas (codigo_cuenta, nombre, empresa, activa, auxiliar, naturaleza, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			codigoCuenta, nombre, empresa, activaInt, auxiliarInt, naturaleza, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE plan_cuentas SET nombre=?, empresa=?, activa=?, auxiliar=?, naturaleza=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE codigo_cuenta=?`,
			nombre, empresa, activaInt, auxiliarInt, naturaleza, hash, now, codigoCuenta,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedPlanCuentas(currentKeys map[string]bool) int {
	return db.markDeleted("plan_cuentas", "codigo_cuenta", currentKeys)
}

func (db *DB) GetPlanCuentas(limit, offset int, search string) ([]PlanCuentasRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(codigo_cuenta) LIKE ? OR LOWER(nombre) LIKE ?)"
		args = append(args, like, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM plan_cuentas WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, codigo_cuenta, nombre, COALESCE(empresa,''), activa, auxiliar, COALESCE(naturaleza,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM plan_cuentas WHERE "+where+" ORDER BY codigo_cuenta LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []PlanCuentasRecord
	for rows.Next() {
		var r PlanCuentasRecord
		var activa, auxiliar int
		if err := rows.Scan(&r.ID, &r.CodigoCuenta, &r.Nombre, &r.Empresa, &activa, &auxiliar, &r.Naturaleza, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		r.Activa = activa == 1
		r.Auxiliar = auxiliar == 1
		records = append(records, r)
	}
	return records, total
}

// ==================== ACTIVOS FIJOS (Z27) ====================

func (db *DB) UpsertActivoFijo(codigo, nombre, empresa, nitResponsable, fechaAdquisicion, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM activos_fijos WHERE codigo=?`, codigo).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO activos_fijos (codigo, nombre, empresa, nit_responsable, fecha_adquisicion, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			codigo, nombre, empresa, nitResponsable, fechaAdquisicion, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE activos_fijos SET nombre=?, empresa=?, nit_responsable=?, fecha_adquisicion=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE codigo=?`,
			nombre, empresa, nitResponsable, fechaAdquisicion, hash, now, codigo,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedActivosFijos(currentKeys map[string]bool) int {
	return db.markDeleted("activos_fijos", "codigo", currentKeys)
}

func (db *DB) GetActivosFijos(limit, offset int, search string) ([]ActivoFijoRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(codigo) LIKE ? OR LOWER(nombre) LIKE ? OR LOWER(nit_responsable) LIKE ?)"
		args = append(args, like, like, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM activos_fijos WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, codigo, nombre, COALESCE(empresa,''), COALESCE(nit_responsable,''), COALESCE(fecha_adquisicion,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM activos_fijos WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []ActivoFijoRecord
	for rows.Next() {
		var r ActivoFijoRecord
		if err := rows.Scan(&r.ID, &r.Codigo, &r.Nombre, &r.Empresa, &r.NitResponsable, &r.FechaAdquisicion, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== SALDOS TERCEROS (Z25) ====================

func (db *DB) UpsertSaldoTercero(key, cuentaContable, nitTercero, empresa, hash string, saldoAnterior, debito, credito, saldoFinal float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM saldos_terceros WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO saldos_terceros (record_key, cuenta_contable, nit_tercero, empresa, saldo_anterior, debito, credito, saldo_final, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, cuentaContable, nitTercero, empresa, saldoAnterior, debito, credito, saldoFinal, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE saldos_terceros SET cuenta_contable=?, nit_tercero=?, empresa=?, saldo_anterior=?, debito=?, credito=?, saldo_final=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			cuentaContable, nitTercero, empresa, saldoAnterior, debito, credito, saldoFinal, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedSaldosTerceros(currentKeys map[string]bool) int {
	return db.markDeleted("saldos_terceros", "record_key", currentKeys)
}

func (db *DB) GetSaldosTerceros(limit, offset int, search string) ([]SaldoTerceroRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(cuenta_contable) LIKE ? OR LOWER(nit_tercero) LIKE ?)"
		args = append(args, like, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM saldos_terceros WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, cuenta_contable, COALESCE(nit_tercero,''), COALESCE(empresa,''), saldo_anterior, debito, credito, saldo_final, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM saldos_terceros WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []SaldoTerceroRecord
	for rows.Next() {
		var r SaldoTerceroRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.CuentaContable, &r.NitTercero, &r.Empresa, &r.SaldoAnterior, &r.Debito, &r.Credito, &r.SaldoFinal, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== SALDOS CONSOLIDADOS (Z28) ====================

func (db *DB) UpsertSaldoConsolidado(cuentaContable, empresa, hash string, saldoAnterior, debito, credito, saldoFinal float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM saldos_consolidados WHERE cuenta_contable=?`, cuentaContable).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO saldos_consolidados (cuenta_contable, empresa, saldo_anterior, debito, credito, saldo_final, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			cuentaContable, empresa, saldoAnterior, debito, credito, saldoFinal, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE saldos_consolidados SET empresa=?, saldo_anterior=?, debito=?, credito=?, saldo_final=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE cuenta_contable=?`,
			empresa, saldoAnterior, debito, credito, saldoFinal, hash, now, cuentaContable,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedSaldosConsolidados(currentKeys map[string]bool) int {
	return db.markDeleted("saldos_consolidados", "cuenta_contable", currentKeys)
}

func (db *DB) GetSaldosConsolidados(limit, offset int, search string) ([]SaldoConsolidadoRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "LOWER(cuenta_contable) LIKE ?"
		args = append(args, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM saldos_consolidados WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, cuenta_contable, COALESCE(empresa,''), saldo_anterior, debito, credito, saldo_final, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM saldos_consolidados WHERE "+where+" ORDER BY cuenta_contable LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []SaldoConsolidadoRecord
	for rows.Next() {
		var r SaldoConsolidadoRecord
		if err := rows.Scan(&r.ID, &r.CuentaContable, &r.Empresa, &r.SaldoAnterior, &r.Debito, &r.Credito, &r.SaldoFinal, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== DOCUMENTOS (Z11) ====================

func (db *DB) UpsertDocumento(key, tipoComp, codigoComp, secuencia, nitTercero, cuentaContable, productoRef, bodega, centroCosto, fecha, descripcion, tipoMov, referencia, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM documentos WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO documentos (record_key, tipo_comprobante, codigo_comp, secuencia, nit_tercero, cuenta_contable, producto_ref, bodega, centro_costo, fecha, descripcion, tipo_mov, referencia, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoComp, codigoComp, secuencia, nitTercero, cuentaContable, productoRef, bodega, centroCosto, fecha, descripcion, tipoMov, referencia, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE documentos SET tipo_comprobante=?, codigo_comp=?, secuencia=?, nit_tercero=?, cuenta_contable=?, producto_ref=?, bodega=?, centro_costo=?, fecha=?, descripcion=?, tipo_mov=?, referencia=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoComp, codigoComp, secuencia, nitTercero, cuentaContable, productoRef, bodega, centroCosto, fecha, descripcion, tipoMov, referencia, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedDocumentos(currentKeys map[string]bool) int {
	return db.markDeleted("documentos", "record_key", currentKeys)
}

func (db *DB) GetDocumentos(limit, offset int, search string) ([]DocumentoRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(cuenta_contable) LIKE ? OR LOWER(producto_ref) LIKE ?)"
		args = append(args, like, like, like, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM documentos WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, COALESCE(tipo_comprobante,''), COALESCE(codigo_comp,''), COALESCE(secuencia,''), COALESCE(nit_tercero,''), COALESCE(cuenta_contable,''), COALESCE(producto_ref,''), COALESCE(bodega,''), COALESCE(centro_costo,''), COALESCE(fecha,''), COALESCE(descripcion,''), COALESCE(tipo_mov,''), COALESCE(referencia,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM documentos WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []DocumentoRecord
	for rows.Next() {
		var r DocumentoRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.TipoComprobante, &r.CodigoComp, &r.Secuencia, &r.NitTercero, &r.CuentaContable, &r.ProductoRef, &r.Bodega, &r.CentroCosto, &r.Fecha, &r.Descripcion, &r.TipoMov, &r.Referencia, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== TERCEROS AMPLIADOS (Z08A) ====================

func (db *DB) UpsertTerceroAmpliado(nit, nombre, empresa, tipoPersona, repLegal, direccion, email, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM terceros_ampliados WHERE nit=?`, nit).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO terceros_ampliados (nit, nombre, empresa, tipo_persona, representante_legal, direccion, email, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			nit, nombre, empresa, tipoPersona, repLegal, direccion, email, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE terceros_ampliados SET nombre=?, empresa=?, tipo_persona=?, representante_legal=?, direccion=?, email=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE nit=?`,
			nombre, empresa, tipoPersona, repLegal, direccion, email, hash, now, nit,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedTercerosAmpliados(currentKeys map[string]bool) int {
	return db.markDeleted("terceros_ampliados", "nit", currentKeys)
}

func (db *DB) GetTercerosAmpliados(limit, offset int, search string) ([]TerceroAmpliadoRecord, int) {
	where := "1=1"
	args := []interface{}{}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit) LIKE ? OR LOWER(nombre) LIKE ? OR LOWER(COALESCE(email,'')) LIKE ? OR LOWER(COALESCE(direccion,'')) LIKE ?)"
		args = append(args, like, like, like, like)
	}
	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM terceros_ampliados WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, nit, nombre, COALESCE(empresa,''), COALESCE(tipo_persona,''), COALESCE(representante_legal,''), COALESCE(direccion,''), COALESCE(email,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM terceros_ampliados WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []TerceroAmpliadoRecord
	for rows.Next() {
		var r TerceroAmpliadoRecord
		if err := rows.Scan(&r.ID, &r.Nit, &r.Nombre, &r.Empresa, &r.TipoPersona, &r.RepresentanteLegal, &r.Direccion, &r.Email, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== TRANSACCIONES DETALLE (Z07T) ====================

func (db *DB) UpsertTransaccionDetalle(key, tipoComp, empresa, secuencia, nitTercero, cuentaContable, fechaDoc, fechaVenc, tipoMov, referencia, hash string, valor float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM transacciones_detalle WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO transacciones_detalle (record_key, tipo_comprobante, empresa, secuencia, nit_tercero, cuenta_contable, fecha_documento, fecha_vencimiento, tipo_movimiento, valor, referencia, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipoComp, empresa, secuencia, nitTercero, cuentaContable, fechaDoc, fechaVenc, tipoMov, valor, referencia, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE transacciones_detalle SET tipo_comprobante=?, empresa=?, secuencia=?, nit_tercero=?, cuenta_contable=?, fecha_documento=?, fecha_vencimiento=?, tipo_movimiento=?, valor=?, referencia=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipoComp, empresa, secuencia, nitTercero, cuentaContable, fechaDoc, fechaVenc, tipoMov, valor, referencia, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedTransaccionesDetalle(currentKeys map[string]bool) int {
	return db.markDeleted("transacciones_detalle", "record_key", currentKeys)
}

// ==================== PERIODOS CONTABLES (Z26) ====================

func (db *DB) UpsertPeriodoContable(key, empresa, numeroPeriodo, fechaInicio, fechaFin, estado, hash string, saldo1, saldo2, saldo3 float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM periodos_contables WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO periodos_contables (record_key, empresa, numero_periodo, fecha_inicio, fecha_fin, estado, saldo1, saldo2, saldo3, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, empresa, numeroPeriodo, fechaInicio, fechaFin, estado, saldo1, saldo2, saldo3, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE periodos_contables SET empresa=?, numero_periodo=?, fecha_inicio=?, fecha_fin=?, estado=?, saldo1=?, saldo2=?, saldo3=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			empresa, numeroPeriodo, fechaInicio, fechaFin, estado, saldo1, saldo2, saldo3, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedPeriodosContables(currentKeys map[string]bool) int {
	return db.markDeleted("periodos_contables", "record_key", currentKeys)
}

// ==================== CONDICIONES DE PAGO (Z05) ====================

func (db *DB) UpsertCondicionPago(key, tipo, empresa, secuencia, tipoDoc, fecha, nit, tipoSecundario, fechaRegistro, hash string, valor float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM condiciones_pago WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO condiciones_pago (record_key, tipo, empresa, secuencia, tipo_doc, fecha, nit, tipo_secundario, valor, fecha_registro, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipo, empresa, secuencia, tipoDoc, fecha, nit, tipoSecundario, valor, fechaRegistro, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE condiciones_pago SET tipo=?, empresa=?, secuencia=?, tipo_doc=?, fecha=?, nit=?, tipo_secundario=?, valor=?, fecha_registro=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipo, empresa, secuencia, tipoDoc, fecha, nit, tipoSecundario, valor, fechaRegistro, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedCondicionesPago(currentKeys map[string]bool) int {
	return db.markDeleted("condiciones_pago", "record_key", currentKeys)
}

// ==================== LIBROS AUXILIARES (Z07) ====================

func (db *DB) UpsertLibroAuxiliar(key, empresa, cuentaContable, tipoComp, codigoComp, fechaDoc, nitTercero, hash string, saldo, debito, credito float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM libros_auxiliares WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO libros_auxiliares (record_key, empresa, cuenta_contable, tipo_comprobante, codigo_comprobante, fecha_documento, nit_tercero, saldo, debito, credito, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, empresa, cuentaContable, tipoComp, codigoComp, fechaDoc, nitTercero, saldo, debito, credito, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE libros_auxiliares SET empresa=?, cuenta_contable=?, tipo_comprobante=?, codigo_comprobante=?, fecha_documento=?, nit_tercero=?, saldo=?, debito=?, credito=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			empresa, cuentaContable, tipoComp, codigoComp, fechaDoc, nitTercero, saldo, debito, credito, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedLibrosAuxiliares(currentKeys map[string]bool) int {
	return db.markDeleted("libros_auxiliares", "record_key", currentKeys)
}

// ==================== CODIGOS DANE (ZDANE) ====================

func (db *DB) UpsertCodigoDane(codigo, nombre, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM codigos_dane WHERE codigo=?`, codigo).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO codigos_dane (codigo, nombre, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, 'pending', 'add', ?)`,
			codigo, nombre, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE codigos_dane SET nombre=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE codigo=?`,
			nombre, hash, now, codigo,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedCodigosDane(currentKeys map[string]bool) int {
	return db.markDeleted("codigos_dane", "codigo", currentKeys)
}

// ==================== ACTIVIDADES ICA (ZICA) ====================

func (db *DB) UpsertActividadICA(codigo, nombre, tarifa, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM actividades_ica WHERE codigo=?`, codigo).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO actividades_ica (codigo, nombre, tarifa, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, 'pending', 'add', ?)`,
			codigo, nombre, tarifa, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE actividades_ica SET nombre=?, tarifa=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE codigo=?`,
			nombre, tarifa, hash, now, codigo,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedActividadesICA(currentKeys map[string]bool) int {
	return db.markDeleted("actividades_ica", "codigo", currentKeys)
}

// ==================== CONCEPTOS PILA (ZPILA) ====================

func (db *DB) UpsertConceptoPILA(key, tipo, fondo, concepto, flags, tipoBase, baseCalculo, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM conceptos_pila WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO conceptos_pila (record_key, tipo, fondo, concepto, flags, tipo_base, base_calculo, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, tipo, fondo, concepto, flags, tipoBase, baseCalculo, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE conceptos_pila SET tipo=?, fondo=?, concepto=?, flags=?, tipo_base=?, base_calculo=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			tipo, fondo, concepto, flags, tipoBase, baseCalculo, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedConceptosPILA(currentKeys map[string]bool) int {
	return db.markDeleted("conceptos_pila", "record_key", currentKeys)
}

// ==================== ACTIVOS FIJOS DETALLE (Z27 detail) ====================

func (db *DB) UpsertActivoFijoDetalle(key, grupo, secuencia, nombre, nitResponsable, codigo, fecha, hash string, valorCompra float64) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM activos_fijos_detalle WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO activos_fijos_detalle (record_key, grupo, secuencia, nombre, nit_responsable, codigo, fecha, valor_compra, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, grupo, secuencia, nombre, nitResponsable, codigo, fecha, valorCompra, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE activos_fijos_detalle SET grupo=?, secuencia=?, nombre=?, nit_responsable=?, codigo=?, fecha=?, valor_compra=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			grupo, secuencia, nombre, nitResponsable, codigo, fecha, valorCompra, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedActivosFijosDetalle(currentKeys map[string]bool) int {
	return db.markDeleted("activos_fijos_detalle", "record_key", currentKeys)
}

// ==================== AUDIT TRAIL TERCEROS (Z11N) ====================

func (db *DB) UpsertAuditTrailTercero(key, fechaCambio, nitTercero, timestamp, usuario, fechaPeriodo, tipoDoc, nombre, nitRepresentante, nombreRepresentante, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM audit_trail_terceros WHERE record_key=?`, key).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO audit_trail_terceros (record_key, fecha_cambio, nit_tercero, timestamp, usuario, fecha_periodo, tipo_doc, nombre, nit_representante, nombre_representante, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			key, fechaCambio, nitTercero, timestamp, usuario, fechaPeriodo, tipoDoc, nombre, nitRepresentante, nombreRepresentante, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE audit_trail_terceros SET fecha_cambio=?, nit_tercero=?, timestamp=?, usuario=?, fecha_periodo=?, tipo_doc=?, nombre=?, nit_representante=?, nombre_representante=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE record_key=?`,
			fechaCambio, nitTercero, timestamp, usuario, fechaPeriodo, tipoDoc, nombre, nitRepresentante, nombreRepresentante, hash, now, key,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedAuditTrailTerceros(currentKeys map[string]bool) int {
	return db.markDeleted("audit_trail_terceros", "record_key", currentKeys)
}

// ==================== CLASIFICACION CUENTAS (Z279CP) ====================

func (db *DB) UpsertClasificacionCuenta(codigoCuenta, codigoGrupo, codigoDetalle, descripcion, hash string) string {
	var existingHash string
	err := db.conn.QueryRow(`SELECT hash FROM clasificacion_cuentas WHERE codigo_cuenta=?`, codigoCuenta).Scan(&existingHash)
	now := time.Now().Format(time.RFC3339)

	if err == sql.ErrNoRows {
		db.conn.Exec(
			`INSERT INTO clasificacion_cuentas (codigo_cuenta, codigo_grupo, codigo_detalle, descripcion, hash, sync_status, sync_action, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', 'add', ?)`,
			codigoCuenta, codigoGrupo, codigoDetalle, descripcion, hash, now,
		)
		return "add"
	}

	if existingHash != hash {
		db.conn.Exec(
			`UPDATE clasificacion_cuentas SET codigo_grupo=?, codigo_detalle=?, descripcion=?, hash=?, sync_status='pending', sync_action='edit', updated_at=?
			 WHERE codigo_cuenta=?`,
			codigoGrupo, codigoDetalle, descripcion, hash, now, codigoCuenta,
		)
		return "edit"
	}
	return ""
}

func (db *DB) MarkDeletedClasificacionCuentas(currentKeys map[string]bool) int {
	return db.markDeleted("clasificacion_cuentas", "codigo_cuenta", currentKeys)
}

// ==================== GENERIC SYNC HELPERS ====================

func (db *DB) getAllHashes(table, keyCol string) map[string]string {
	hashes := make(map[string]string)
	rows, err := db.conn.Query(
		fmt.Sprintf(`SELECT %s, hash FROM %s`, keyCol, table),
	)
	if err != nil {
		return hashes
	}
	defer rows.Close()

	for rows.Next() {
		var key, hash string
		if err := rows.Scan(&key, &hash); err == nil {
			hashes[key] = hash
		}
	}
	return hashes
}

func (db *DB) markDeleted(table, keyCol string, currentKeys map[string]bool) int {
	rows, err := db.conn.Query(
		fmt.Sprintf(`SELECT %s FROM %s WHERE sync_action != 'delete'`, keyCol, table),
	)
	if err != nil {
		return 0
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil {
			if !currentKeys[key] {
				toDelete = append(toDelete, key)
			}
		}
	}

	now := time.Now().Format(time.RFC3339)
	for _, key := range toDelete {
		db.conn.Exec(
			fmt.Sprintf(`UPDATE %s SET sync_status='pending', sync_action='delete', updated_at=? WHERE %s=?`, table, keyCol),
			now, key,
		)
	}
	return len(toDelete)
}

// MarkSynced marks a record as successfully synced
func (db *DB) MarkSynced(table string, id int64) {
	now := time.Now().Format(time.RFC3339)
	db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='synced', sync_error='', synced_at=? WHERE id=?`, table),
		now, id,
	)
}

// MarkSyncError marks a record as failed to sync and increments retry_count
func (db *DB) MarkSyncError(table string, id int64, errMsg string) {
	db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='error', sync_error=?, retry_count=COALESCE(retry_count,0)+1 WHERE id=?`, table),
		errMsg, id,
	)
}

// RemoveDeletedSynced removes records that were deleted and successfully synced
func (db *DB) RemoveDeletedSynced(table string) {
	db.conn.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE sync_action='delete' AND sync_status='synced'`, table),
	)
}

// RetryErrors resets error records to pending so they get retried (manual retry resets retry_count)
func (db *DB) RetryErrors(table string) int {
	result, err := db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='pending', retry_count=0 WHERE sync_status='error'`, table),
	)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// GetRetryableErrorCount returns the number of error records eligible for automatic retry
func (db *DB) GetRetryableErrorCount(table string, maxRetries int) int {
	var count int
	db.conn.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='error' AND COALESCE(retry_count,0) < ?`, table),
		maxRetries,
	).Scan(&count)
	return count
}

// GetMaxRetryCount returns the highest retry_count among retryable error records
func (db *DB) GetMaxRetryCount(table string, maxRetries int) int {
	var maxCount int
	db.conn.QueryRow(
		fmt.Sprintf(`SELECT COALESCE(MAX(COALESCE(retry_count,0)),0) FROM %s WHERE sync_status='error' AND COALESCE(retry_count,0) < ?`, table),
		maxRetries,
	).Scan(&maxCount)
	return maxCount
}

// RequeueRetryableErrors moves error records that haven't exceeded max retries back to pending
func (db *DB) RequeueRetryableErrors(table string, maxRetries int) int {
	result, err := db.conn.Exec(
		fmt.Sprintf(`UPDATE %s SET sync_status='pending' WHERE sync_status='error' AND COALESCE(retry_count,0) < ?`, table),
		maxRetries,
	)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// ==================== ERROR SUMMARY ====================

type ErrorSummary struct {
	Table      string `json:"table"`
	Error      string `json:"error"`
	Count      int    `json:"count"`
	MaxRetries int    `json:"max_retries"`
	LastSeen   string `json:"last_seen"`
}

func (db *DB) GetErrorSummary() []ErrorSummary {
	tables := []string{"clients", "products", "movements", "cartera"}
	results := make([]ErrorSummary, 0)

	for _, t := range tables {
		rows, err := db.conn.Query(
			fmt.Sprintf(`SELECT sync_error, COUNT(*) as cnt, MAX(COALESCE(retry_count,0)) as max_rc, MAX(updated_at) as last_seen
				FROM %s WHERE sync_status='error' AND sync_error != ''
				GROUP BY sync_error ORDER BY cnt DESC`, t),
		)
		if err != nil {
			continue
		}
		for rows.Next() {
			var e ErrorSummary
			if err := rows.Scan(&e.Error, &e.Count, &e.MaxRetries, &e.LastSeen); err == nil {
				e.Table = t
				results = append(results, e)
			}
		}
		rows.Close()
	}
	return results
}

// ==================== CSV EXPORT ====================

func (db *DB) ExportSyncHistoryCSV(table string) ([]byte, error) {
	where := "1=1"
	args := []interface{}{}
	if table != "" {
		where = "table_name=?"
		args = append(args, table)
	}

	rows, err := db.conn.Query(
		"SELECT table_name, record_key, action, COALESCE(data,''), status, COALESCE(error,''), created_at FROM sync_history WHERE "+where+" ORDER BY id DESC",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buf strings.Builder
	buf.WriteString("tabla,key,accion,data,estado,error,fecha\n")
	for rows.Next() {
		var tbl, key, action, data, status, errMsg, created string
		if err := rows.Scan(&tbl, &key, &action, &data, &status, &errMsg, &created); err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("%s,%s,%s,\"%s\",%s,\"%s\",%s\n",
			csvEscape(tbl), csvEscape(key), csvEscape(action), csvEscape(data), csvEscape(status), csvEscape(errMsg), csvEscape(created)))
	}
	return []byte(buf.String()), nil
}

func (db *DB) ExportLogsCSV() ([]byte, error) {
	rows, err := db.conn.Query(`SELECT level, source, message, created_at FROM logs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buf strings.Builder
	buf.WriteString("nivel,fuente,mensaje,fecha\n")
	for rows.Next() {
		var level, source, msg, created string
		if err := rows.Scan(&level, &source, &msg, &created); err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("%s,%s,\"%s\",%s\n",
			csvEscape(level), csvEscape(source), csvEscape(msg), csvEscape(created)))
	}
	return []byte(buf.String()), nil
}

func csvEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\"", "\"\""), "\n", " ")
}

// ==================== SYNC HISTORY ====================

func (db *DB) AddSyncHistory(tableName, key, action, data, status, errMsg string) {
	db.conn.Exec(
		`INSERT INTO sync_history (table_name, record_key, action, data, status, error)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tableName, key, action, data, status, errMsg,
	)
}

func (db *DB) GetSyncHistory(tableName string, limit, offset int) ([]SyncRecord, int, error) {
	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM sync_history WHERE table_name=?`, tableName).Scan(&total)

	rows, err := db.conn.Query(
		`SELECT id, table_name, record_key, COALESCE(data,''), '', status, COALESCE(error,''), action, created_at, ''
		 FROM sync_history WHERE table_name=? ORDER BY id DESC LIMIT ? OFFSET ?`,
		tableName, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		if err := rows.Scan(&r.ID, &r.Table, &r.Key, &r.Data, &r.Hash, &r.SyncStatus, &r.SyncError, &r.SyncAction, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total, nil
}

func (db *DB) SearchSyncHistory(tableName, search, dateFrom, dateTo, status string, limit, offset int) ([]SyncRecord, int, error) {
	where := "table_name=?"
	args := []interface{}{tableName}

	if search != "" {
		like := "%" + search + "%"
		where += " AND (record_key LIKE ? OR data LIKE ? OR action LIKE ?)"
		args = append(args, like, like, like)
	}
	if dateFrom != "" {
		where += " AND created_at >= ?"
		args = append(args, dateFrom+"T00:00:00")
	}
	if dateTo != "" {
		where += " AND created_at <= ?"
		args = append(args, dateTo+"T23:59:59")
	}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	db.conn.QueryRow("SELECT COUNT(*) FROM sync_history WHERE "+where, countArgs...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, table_name, record_key, COALESCE(data,''), '', status, COALESCE(error,''), action, created_at, '' FROM sync_history WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		if err := rows.Scan(&r.ID, &r.Table, &r.Key, &r.Data, &r.Hash, &r.SyncStatus, &r.SyncError, &r.SyncAction, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total, nil
}

// ==================== STATS ====================

func (db *DB) GetStats() map[string]interface{} {
	stats := map[string]interface{}{}

	tables := []string{"clients", "products", "movements", "cartera", "plan_cuentas", "activos_fijos", "saldos_terceros", "saldos_consolidados", "documentos", "terceros_ampliados", "transacciones_detalle", "periodos_contables", "condiciones_pago", "libros_auxiliares", "codigos_dane", "actividades_ica", "conceptos_pila", "activos_fijos_detalle", "audit_trail_terceros", "clasificacion_cuentas"}
	for _, t := range tables {
		var total, synced, pending, errors int
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, t)).Scan(&total)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='synced'`, t)).Scan(&synced)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='pending'`, t)).Scan(&pending)
		db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE sync_status='error'`, t)).Scan(&errors)
		stats[t+"_total"] = total
		stats[t+"_synced"] = synced
		stats[t+"_pending"] = pending
		stats[t+"_errors"] = errors
	}

	return stats
}

// ==================== PUBLIC API QUERIES ====================

// APIQueryParams holds the parameters for public API queries
type APIQueryParams struct {
	Search     string
	SyncStatus string
	Since      string // ISO date, records updated after this
	Limit      int
	Offset     int
}

// APIQueryResult holds a paginated result for the public API
type APIQueryResult struct {
	Data  []map[string]interface{} `json:"data"`
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Limit int                      `json:"limit"`
}

func (db *DB) APIGetRecords(table string, params APIQueryParams) APIQueryResult {
	if !isValidTable(table) {
		return APIQueryResult{Data: make([]map[string]interface{}, 0)}
	}
	keyCol := db.keyColForTable(table)
	if keyCol == "" {
		return APIQueryResult{Data: make([]map[string]interface{}, 0)}
	}

	where := "1=1"
	args := []interface{}{}

	if params.Search != "" {
		like := "%" + strings.ToLower(params.Search) + "%"
		switch table {
		case "clients":
			where += " AND (LOWER(nit) LIKE ? OR LOWER(nombre) LIKE ?)"
			args = append(args, like, like)
		case "products":
			where += " AND (LOWER(code) LIKE ? OR LOWER(nombre) LIKE ?)"
			args = append(args, like, like)
		case "movements":
			where += " AND (LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(numero_doc) LIKE ?)"
			args = append(args, like, like, like)
		case "cartera":
			where += " AND (LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ?)"
			args = append(args, like, like)
		}
	}
	if params.SyncStatus != "" {
		where += " AND sync_status=?"
		args = append(args, params.SyncStatus)
	}
	if params.Since != "" {
		where += " AND updated_at >= ?"
		args = append(args, params.Since)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where), countArgs...).Scan(&total)

	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 500 {
		params.Limit = 500
	}

	queryArgs := append(args, params.Limit, params.Offset)
	rows, err := db.conn.Query(
		fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY id LIMIT ? OFFSET ?", table, where),
		queryArgs...,
	)
	if err != nil {
		return APIQueryResult{Data: make([]map[string]interface{}, 0), Total: total, Limit: params.Limit}
	}
	defer rows.Close()

	data := db.scanRowsToMaps(rows)
	page := 1
	if params.Offset > 0 && params.Limit > 0 {
		page = (params.Offset / params.Limit) + 1
	}
	return APIQueryResult{Data: data, Total: total, Page: page, Limit: params.Limit}
}

func (db *DB) APIGetRecord(table, key string) map[string]interface{} {
	if !isValidTable(table) {
		return nil
	}
	keyCol := db.keyColForTable(table)
	if keyCol == "" {
		return nil
	}
	rows, err := db.conn.Query(
		fmt.Sprintf("SELECT * FROM %s WHERE %s=? LIMIT 1", table, keyCol), key,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	results := db.scanRowsToMaps(rows)
	if len(results) == 0 {
		return nil
	}
	return results[0]
}

// ODataParams holds OData query options
type ODataParams struct {
	Top     int
	Skip    int
	Filter  string // raw $filter string
	OrderBy string // raw $orderby string
	Select  string // raw $select string
	Count   bool   // $count=true
}

// ODataResult holds the OData response
type ODataResult struct {
	Value    []map[string]interface{} `json:"value"`
	Count    *int                     `json:"@odata.count,omitempty"`
	NextLink string                   `json:"@odata.nextLink,omitempty"`
}

// ODataGetRecords queries a table with OData conventions
func (db *DB) ODataGetRecords(table string, params ODataParams) ODataResult {
	if !isValidTable(table) || table == "sync_history" || table == "logs" {
		return ODataResult{Value: make([]map[string]interface{}, 0)}
	}

	where, args := db.odataParseFilter(table, params.Filter)

	// Count
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where), countArgs...).Scan(&total)

	// Defaults
	top := params.Top
	if top <= 0 {
		top = 100
	}
	if top > 5000 {
		top = 5000
	}
	skip := params.Skip
	if skip < 0 {
		skip = 0
	}

	// OrderBy
	orderBy := db.odataParseOrderBy(table, params.OrderBy)

	// Select columns
	selectCols := db.odataParseSelect(table, params.Select)

	queryArgs := append(args, top, skip)
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT ? OFFSET ?",
		selectCols, table, where, orderBy)
	rows, err := db.conn.Query(q, queryArgs...)
	if err != nil {
		return ODataResult{Value: make([]map[string]interface{}, 0)}
	}
	defer rows.Close()

	data := db.scanRowsToMaps(rows)

	result := ODataResult{Value: data}
	if params.Count {
		result.Count = &total
	}

	return result
}

// ODataGetCount returns count only
func (db *DB) ODataGetCount(table string, filter string) int {
	if !isValidTable(table) || table == "sync_history" || table == "logs" {
		return 0
	}
	where, args := db.odataParseFilter(table, filter)
	var total int
	db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where), args...).Scan(&total)
	return total
}

// GetTableColumns returns column names for a table
func (db *DB) GetTableColumns(table string) []string {
	if !isValidTable(table) {
		return nil
	}
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt interface{}
		var pk int
		rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		cols = append(cols, name)
	}
	return cols
}

// odataValidColumn checks if a column name is valid for a table (prevent SQL injection)
func (db *DB) odataValidColumn(table, col string) bool {
	cols := db.GetTableColumns(table)
	for _, c := range cols {
		if c == col {
			return true
		}
	}
	return false
}

// odataParseFilter parses a basic OData $filter string into SQL WHERE + args
// Supports: eq, ne, gt, ge, lt, le, contains(), startswith()
func (db *DB) odataParseFilter(table, filter string) (string, []interface{}) {
	if filter == "" {
		return "1=1", nil
	}

	var conditions []string
	var args []interface{}

	// Split by ' and '
	parts := odataSplitAnd(filter)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// contains(field,'value')
		if strings.HasPrefix(part, "contains(") {
			field, val, ok := odataParseFunc(part, "contains")
			if ok && db.odataValidColumn(table, field) {
				conditions = append(conditions, fmt.Sprintf("LOWER(%s) LIKE ?", field))
				args = append(args, "%"+strings.ToLower(val)+"%")
			}
			continue
		}

		// startswith(field,'value')
		if strings.HasPrefix(part, "startswith(") {
			field, val, ok := odataParseFunc(part, "startswith")
			if ok && db.odataValidColumn(table, field) {
				conditions = append(conditions, fmt.Sprintf("LOWER(%s) LIKE ?", field))
				args = append(args, strings.ToLower(val)+"%")
			}
			continue
		}

		// field op value
		field, op, val, ok := odataParseComparison(part)
		if !ok || !db.odataValidColumn(table, field) {
			continue
		}

		sqlOp := ""
		switch op {
		case "eq":
			sqlOp = "="
		case "ne":
			sqlOp = "!="
		case "gt":
			sqlOp = ">"
		case "ge":
			sqlOp = ">="
		case "lt":
			sqlOp = "<"
		case "le":
			sqlOp = "<="
		default:
			continue
		}

		conditions = append(conditions, fmt.Sprintf("%s %s ?", field, sqlOp))
		args = append(args, val)
	}

	if len(conditions) == 0 {
		return "1=1", nil
	}
	return strings.Join(conditions, " AND "), args
}

func odataSplitAnd(filter string) []string {
	// Split by " and " (case insensitive), but not inside function calls
	var parts []string
	depth := 0
	start := 0
	lower := strings.ToLower(filter)
	for i := 0; i < len(filter); i++ {
		if filter[i] == '(' {
			depth++
		} else if filter[i] == ')' {
			depth--
		} else if depth == 0 && i+5 <= len(lower) && lower[i:i+5] == " and " {
			parts = append(parts, filter[start:i])
			start = i + 5
			i += 4
		}
	}
	parts = append(parts, filter[start:])
	return parts
}

func odataParseFunc(expr, funcName string) (field, value string, ok bool) {
	// contains(field,'value') or contains(field,'value')
	inner := strings.TrimPrefix(expr, funcName+"(")
	inner = strings.TrimSuffix(inner, ")")
	comma := strings.Index(inner, ",")
	if comma < 0 {
		return "", "", false
	}
	field = strings.TrimSpace(inner[:comma])
	value = strings.TrimSpace(inner[comma+1:])
	value = strings.Trim(value, "'\"")
	return field, value, field != ""
}

func odataParseComparison(expr string) (field, op, value string, ok bool) {
	// field eq 'value' or field gt 123
	ops := []string{" eq ", " ne ", " gt ", " ge ", " lt ", " le "}
	for _, o := range ops {
		idx := strings.Index(strings.ToLower(expr), o)
		if idx >= 0 {
			field = strings.TrimSpace(expr[:idx])
			op = strings.TrimSpace(o)
			value = strings.TrimSpace(expr[idx+len(o):])
			value = strings.Trim(value, "'\"")
			return field, op, value, field != ""
		}
	}
	return "", "", "", false
}

func (db *DB) odataParseOrderBy(table, orderBy string) string {
	if orderBy == "" {
		return "id"
	}
	var parts []string
	for _, seg := range strings.Split(orderBy, ",") {
		seg = strings.TrimSpace(seg)
		tokens := strings.Fields(seg)
		if len(tokens) == 0 {
			continue
		}
		col := tokens[0]
		if !db.odataValidColumn(table, col) {
			continue
		}
		dir := "ASC"
		if len(tokens) > 1 && strings.ToLower(tokens[1]) == "desc" {
			dir = "DESC"
		}
		parts = append(parts, col+" "+dir)
	}
	if len(parts) == 0 {
		return "id"
	}
	return strings.Join(parts, ", ")
}

func (db *DB) odataParseSelect(table, sel string) string {
	if sel == "" {
		return "*"
	}
	var cols []string
	for _, c := range strings.Split(sel, ",") {
		c = strings.TrimSpace(c)
		if db.odataValidColumn(table, c) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return "*"
	}
	// Always include id
	hasID := false
	for _, c := range cols {
		if c == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		cols = append([]string{"id"}, cols...)
	}
	return strings.Join(cols, ", ")
}

func (db *DB) keyColForTable(table string) string {
	switch table {
	case "clients":
		return "nit"
	case "products":
		return "code"
	case "movements":
		return "record_key"
	case "cartera":
		return "record_key"
	case "plan_cuentas":
		return "codigo_cuenta"
	case "activos_fijos":
		return "codigo"
	case "saldos_terceros":
		return "record_key"
	case "saldos_consolidados":
		return "cuenta_contable"
	case "documentos":
		return "record_key"
	case "terceros_ampliados":
		return "nit"
	case "transacciones_detalle":
		return "record_key"
	case "periodos_contables":
		return "record_key"
	case "condiciones_pago":
		return "record_key"
	case "libros_auxiliares":
		return "record_key"
	case "codigos_dane":
		return "codigo"
	case "actividades_ica":
		return "codigo"
	case "conceptos_pila":
		return "record_key"
	case "activos_fijos_detalle":
		return "record_key"
	case "audit_trail_terceros":
		return "record_key"
	case "clasificacion_cuentas":
		return "codigo_cuenta"
	}
	return ""
}

func (db *DB) scanRowsToMaps(rows *sql.Rows) []map[string]interface{} {
	cols, err := rows.Columns()
	if err != nil {
		return nil
	}
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = v
			}
		}
		result = append(result, row)
	}
	return result
}

// ==================== QUERY TABLES (for UI display) ====================

func (db *DB) GetClients(limit, offset int, search string) ([]ClientRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit) LIKE ? OR LOWER(nombre) LIKE ? OR LOWER(COALESCE(email,'')) LIKE ?)"
		args = append(args, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM clients WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, nit, nombre, COALESCE(tipo_persona,''), empresa, COALESCE(direccion,''), COALESCE(email,''), COALESCE(rep_legal,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM clients WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []ClientRecord
	for rows.Next() {
		var r ClientRecord
		if err := rows.Scan(&r.ID, &r.Nit, &r.Nombre, &r.TipoPersona, &r.Empresa, &r.Direccion, &r.Email, &r.RepLegal, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			log.Printf("scan client error: %v", err)
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetProducts(limit, offset int, search string) ([]ProductRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nombre) LIKE ? OR LOWER(code) LIKE ? OR LOWER(grupo) LIKE ? OR LOWER(COALESCE(referencia,'')) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM products WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, code, nombre, COALESCE(nombre_corto,''), grupo, COALESCE(referencia,''), COALESCE(empresa,''), hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM products WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []ProductRecord
	for rows.Next() {
		var r ProductRecord
		if err := rows.Scan(&r.ID, &r.Code, &r.Nombre, &r.NombreCorto, &r.Grupo, &r.Referencia, &r.Empresa, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetMovements(limit, offset int, search string) ([]MovementRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(tipo_comprobante) LIKE ? OR LOWER(numero_doc) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM movements WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, tipo_comprobante, empresa, numero_doc, fecha, nit_tercero, cuenta_contable, descripcion, valor, tipo_mov, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM movements WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []MovementRecord
	for rows.Next() {
		var r MovementRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.TipoComprobante, &r.Empresa, &r.NumeroDoc, &r.Fecha, &r.NitTercero, &r.CuentaContable, &r.Descripcion, &r.Valor, &r.TipoMov, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

func (db *DB) GetCarteraRecords(limit, offset int, search string) ([]CarteraRecord, int) {
	where := "1=1"
	args := []interface{}{}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = "(LOWER(nit_tercero) LIKE ? OR LOWER(descripcion) LIKE ? OR LOWER(cuenta_contable) LIKE ? OR LOWER(tipo_registro) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM cartera WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, record_key, tipo_registro, empresa, secuencia, tipo_doc, nit_tercero, cuenta_contable, fecha, descripcion, tipo_mov, hash, sync_status, COALESCE(sync_action,''), COALESCE(sync_error,''), updated_at, COALESCE(synced_at,'') FROM cartera WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var records []CarteraRecord
	for rows.Next() {
		var r CarteraRecord
		if err := rows.Scan(&r.ID, &r.RecordKey, &r.TipoRegistro, &r.Empresa, &r.Secuencia, &r.TipoDoc, &r.NitTercero, &r.CuentaContable, &r.Fecha, &r.Descripcion, &r.TipoMov, &r.Hash, &r.SyncStatus, &r.SyncAction, &r.SyncError, &r.UpdatedAt, &r.SyncedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, total
}

// ==================== LOGS ====================

func (db *DB) AddLog(level, source, message string) {
	db.conn.Exec(
		`INSERT INTO logs (level, source, message) VALUES (?, ?, ?)`,
		level, source, message,
	)
}

func (db *DB) GetLogs(limit, offset int) ([]LogEntry, int, error) {
	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM logs`).Scan(&total)

	rows, err := db.conn.Query(
		`SELECT id, level, source, message, created_at FROM logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Level, &l.Source, &l.Message, &l.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// GetLogsFiltered returns logs with optional filtering by level, source, and message search
func (db *DB) GetLogsFiltered(limit, offset int, level, source, search string) ([]LogEntry, int, error) {
	where := ""
	args := []interface{}{}

	conditions := []string{}
	if level != "" {
		conditions = append(conditions, "level = ?")
		args = append(args, level)
	}
	if source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, source)
	}
	if search != "" {
		conditions = append(conditions, "message LIKE ?")
		args = append(args, "%"+search+"%")
	}
	if len(conditions) > 0 {
		where = " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			where += " AND " + conditions[i]
		}
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM logs"+where, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := db.conn.Query(
		"SELECT id, level, source, message, created_at FROM logs"+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Level, &l.Source, &l.Message, &l.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// ==================== CLEANUP ====================

func (db *DB) ClearLogs() error {
	_, err := db.conn.Exec(`DELETE FROM logs`)
	return err
}

func (db *DB) ClearAll() error {
	tables := []string{
		"clients", "products", "movements", "cartera",
		"plan_cuentas", "activos_fijos", "saldos_terceros", "saldos_consolidados",
		"documentos", "terceros_ampliados", "transacciones_detalle",
		"periodos_contables", "condiciones_pago", "libros_auxiliares",
		"codigos_dane", "actividades_ica", "conceptos_pila",
		"activos_fijos_detalle", "audit_trail_terceros", "clasificacion_cuentas",
		"sync_history",
	}
	for _, t := range tables {
		db.conn.Exec(`DELETE FROM ` + t)
	}
	return nil
}

func (db *DB) Close() {
	db.conn.Close()
}

// QueryCount returns the row count for a table (used by setup wizard)
func (db *DB) QueryCount(table string, count *int) {
	if !isValidTable(table) {
		*count = 0
		return
	}
	db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(count)
}

// VacuumInto creates a clean backup copy of the database
func (db *DB) VacuumInto(path string) error {
	_, err := db.conn.Exec(fmt.Sprintf("VACUUM INTO '%s'", path))
	return err
}

// BulkDelete deletes records by IDs from a table
func (db *DB) BulkDelete(table string, ids []int64) int {
	if !isValidTable(table) || len(ids) == 0 {
		return 0
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", table, strings.Join(placeholders, ","))
	result, err := db.conn.Exec(q, args...)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// BulkUpdateStatus updates sync_status for records by IDs
func (db *DB) BulkUpdateStatus(table string, ids []int64, status string) int {
	if !isValidTable(table) || len(ids) == 0 {
		return 0
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, status)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf("UPDATE %s SET sync_status=?, sync_error=NULL, updated_at=CURRENT_TIMESTAMP WHERE id IN (%s)", table, strings.Join(placeholders, ","))
	result, err := db.conn.Exec(q, args...)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// QueryReadOnly executes a read-only SQL query with a row limit.
// Returns columns, rows as maps, and total count.
func (db *DB) QueryReadOnly(query string, limit, offset int) ([]string, []map[string]interface{}, int, error) {
	trimmed := strings.TrimSpace(query)
	isPragma := strings.HasPrefix(strings.ToUpper(trimmed), "PRAGMA")

	// Strip any existing LIMIT/OFFSET from the user query to avoid conflicts
	baseQuery := stripLimitOffset(trimmed)

	// Count total rows from the base query (without LIMIT)
	var total int
	if isPragma {
		total = -1
	} else {
		countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", baseQuery)
		if err := db.conn.QueryRow(countSQL).Scan(&total); err != nil {
			total = -1
		}
	}

	// Execute — PRAGMA doesn't support LIMIT/OFFSET
	var pagedSQL string
	if isPragma {
		pagedSQL = baseQuery
	} else {
		pagedSQL = fmt.Sprintf("%s LIMIT %d OFFSET %d", baseQuery, limit, offset)
	}
	rows, err := db.conn.Query(pagedSQL)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, 0, err
	}

	data := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = v
			}
		}
		data = append(data, row)
	}

	return cols, data, total, nil
}

// stripLimitOffset removes trailing LIMIT and OFFSET clauses from a SQL query
func stripLimitOffset(query string) string {
	re := regexp.MustCompile(`(?i)\s+LIMIT\s+\d+(\s+OFFSET\s+\d+)?\s*$`)
	return re.ReplaceAllString(strings.TrimSpace(query), "")
}

// UpdateRecord updates editable fields of a record by ID.
// fields is a map of column->value to update.
func (db *DB) UpdateRecord(table string, id int64, fields map[string]interface{}) error {
	allowed := map[string]bool{"clients": true, "products": true, "movements": true, "cartera": true}
	if !allowed[table] {
		return fmt.Errorf("tabla no valida: %s", table)
	}

	if len(fields) == 0 {
		return fmt.Errorf("no hay campos para actualizar")
	}

	setClauses := []string{}
	args := []interface{}{}
	for col, val := range fields {
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}
	// Mark as pending so it gets re-synced
	setClauses = append(setClauses, "sync_status = 'pending'", "sync_action = 'edit'", "updated_at = datetime('now')")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(setClauses, ", "))
	_, err := db.conn.Exec(query, args...)
	return err
}

// DeleteRecord deletes a record by ID from the given table.
func (db *DB) DeleteRecord(table string, id int64) error {
	allowed := map[string]bool{"clients": true, "products": true, "movements": true, "cartera": true}
	if !allowed[table] {
		return fmt.Errorf("tabla no valida: %s", table)
	}
	_, err := db.conn.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
	return err
}

// GetRecordByID returns a single record as a map by its ID.
func (db *DB) GetRecordByID(table string, id int64) (map[string]interface{}, error) {
	allowed := map[string]bool{"clients": true, "products": true, "movements": true, "cartera": true}
	if !allowed[table] {
		return nil, fmt.Errorf("tabla no valida: %s", table)
	}
	rows, err := db.conn.Query(fmt.Sprintf("SELECT * FROM %s WHERE id = ?", table), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	if !rows.Next() {
		return nil, fmt.Errorf("registro no encontrado")
	}
	values := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := make(map[string]interface{})
	for i, col := range cols {
		if b, ok := values[i].([]byte); ok {
			row[col] = string(b)
		} else {
			row[col] = values[i]
		}
	}
	return row, nil
}

// ==================== APP USERS ====================

type AppUser struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	Password    string   `json:"-"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Active      bool     `json:"active"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func (db *DB) CreateAppUser(username, password, role string, permissions []string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	permsJSON, _ := json.Marshal(permissions)
	_, err = db.conn.Exec(
		`INSERT INTO app_users (username, password, role, permissions) VALUES (?, ?, ?, ?)`,
		username, string(hashed), role, string(permsJSON),
	)
	return err
}

// CheckAppUserPassword compares a plaintext password against the stored hash.
// Also handles legacy plaintext passwords by rehashing them on match.
func (db *DB) CheckAppUserPassword(user *AppUser, password string) bool {
	if strings.HasPrefix(user.Password, "$2a$") || strings.HasPrefix(user.Password, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil
	}
	// Legacy plaintext comparison + auto-migrate to bcrypt
	if user.Password == password {
		if hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err == nil {
			db.conn.Exec(`UPDATE app_users SET password=? WHERE id=?`, string(hashed), user.ID)
		}
		return true
	}
	return false
}

func (db *DB) GetAppUser(username string) (*AppUser, error) {
	var u AppUser
	var permsJSON string
	var active int
	err := db.conn.QueryRow(
		`SELECT id, username, password, role, permissions, active, created_at, updated_at FROM app_users WHERE username=?`,
		username,
	).Scan(&u.ID, &u.Username, &u.Password, &u.Role, &permsJSON, &active, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.Active = active == 1
	json.Unmarshal([]byte(permsJSON), &u.Permissions)
	if u.Permissions == nil {
		u.Permissions = []string{}
	}
	return &u, nil
}

func (db *DB) GetAppUserByID(id int64) (*AppUser, error) {
	var u AppUser
	var permsJSON string
	var active int
	err := db.conn.QueryRow(
		`SELECT id, username, password, role, permissions, active, created_at, updated_at FROM app_users WHERE id=?`,
		id,
	).Scan(&u.ID, &u.Username, &u.Password, &u.Role, &permsJSON, &active, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.Active = active == 1
	json.Unmarshal([]byte(permsJSON), &u.Permissions)
	if u.Permissions == nil {
		u.Permissions = []string{}
	}
	return &u, nil
}

func (db *DB) ListAppUsers() ([]AppUser, error) {
	rows, err := db.conn.Query(`SELECT id, username, role, permissions, active, created_at, updated_at FROM app_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AppUser
	for rows.Next() {
		var u AppUser
		var permsJSON string
		var active int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &permsJSON, &active, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		u.Active = active == 1
		json.Unmarshal([]byte(permsJSON), &u.Permissions)
		if u.Permissions == nil {
			u.Permissions = []string{}
		}
		users = append(users, u)
	}
	if users == nil {
		users = []AppUser{}
	}
	return users, nil
}

func (db *DB) UpdateAppUser(id int64, role string, permissions []string, active bool) error {
	permsJSON, _ := json.Marshal(permissions)
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := db.conn.Exec(
		`UPDATE app_users SET role=?, permissions=?, active=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		role, string(permsJSON), activeInt, id,
	)
	return err
}

func (db *DB) UpdateAppUserPassword(id int64, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(`UPDATE app_users SET password=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, string(hashed), id)
	return err
}

func (db *DB) DeleteAppUser(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM app_users WHERE id=?`, id)
	return err
}

// trackChanges reads the current record and compares with new values, storing diffs
func (db *DB) trackChanges(table, key string, newValues map[string]string) {
	keyCol := db.keyColForTable(table)
	if keyCol == "" {
		return
	}
	// Get current record
	current := db.APIGetRecord(table, key)
	if current == nil {
		return
	}

	changes := make(map[string]interface{})
	for field, newVal := range newValues {
		oldVal := ""
		if v, ok := current[field]; ok && v != nil {
			oldVal = fmt.Sprintf("%v", v)
		}
		if oldVal != newVal {
			changes[field] = map[string]string{"old": oldVal, "new": newVal}
		}
	}
	if len(changes) > 0 {
		db.AddChangeHistory(table, key, changes)
	}
}

// ==================== AUDIT TRAIL ====================

type AuditEntry struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	TableName string `json:"table_name"`
	RecordID  string `json:"record_id"`
	Details   string `json:"details"`
	CreatedAt string `json:"created_at"`
}

func (db *DB) AddAudit(username, action, tableName, recordID, details string) {
	db.conn.Exec(
		`INSERT INTO audit_trail (username, action, table_name, record_id, details) VALUES (?, ?, ?, ?, ?)`,
		username, action, tableName, recordID, details,
	)
}

func (db *DB) GetAuditTrail(page int, limit int) ([]AuditEntry, int) {
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM audit_trail`).Scan(&total)

	rows, err := db.conn.Query(
		`SELECT id, username, action, table_name, record_id, details, created_at FROM audit_trail ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, total
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		rows.Scan(&e.ID, &e.Username, &e.Action, &e.TableName, &e.RecordID, &e.Details, &e.CreatedAt)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, total
}

// ==================== CHANGE HISTORY ====================

type ChangeEntry struct {
	ID        int64  `json:"id"`
	TableName string `json:"table_name"`
	RecordKey string `json:"record_key"`
	Changes   string `json:"changes"`
	CreatedAt string `json:"created_at"`
}

func (db *DB) AddChangeHistory(tableName, recordKey string, changes map[string]interface{}) {
	data, _ := json.Marshal(changes)
	db.conn.Exec(
		`INSERT INTO change_history (table_name, record_key, changes) VALUES (?, ?, ?)`,
		tableName, recordKey, string(data),
	)
}

func (db *DB) GetChangeHistory(tableName, recordKey string, limit int) []ChangeEntry {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.Query(
		`SELECT id, table_name, record_key, changes, created_at FROM change_history WHERE table_name=? AND record_key=? ORDER BY id DESC LIMIT ?`,
		tableName, recordKey, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []ChangeEntry
	for rows.Next() {
		var e ChangeEntry
		rows.Scan(&e.ID, &e.TableName, &e.RecordKey, &e.Changes, &e.CreatedAt)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []ChangeEntry{}
	}
	return entries
}

// ==================== SYNC STATS (for dashboard charts) ====================

type SyncStatEntry struct {
	ID        int64  `json:"id"`
	TableName string `json:"table_name"`
	Total     int    `json:"total"`
	Pending   int    `json:"pending"`
	Synced    int    `json:"synced"`
	Errors    int    `json:"errors"`
	CreatedAt string `json:"created_at"`
}

func (db *DB) RecordSyncStats(tableName string, total, pending, synced, errors int) {
	db.conn.Exec(
		`INSERT INTO sync_stats (table_name, total, pending, synced, errors) VALUES (?, ?, ?, ?, ?)`,
		tableName, total, pending, synced, errors,
	)
}

func (db *DB) GetSyncStats(hours int) []SyncStatEntry {
	if hours <= 0 {
		hours = 24
	}
	rows, err := db.conn.Query(
		`SELECT id, table_name, total, pending, synced, errors, created_at FROM sync_stats WHERE created_at >= datetime('now', ?) ORDER BY created_at`,
		fmt.Sprintf("-%d hours", hours),
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []SyncStatEntry
	for rows.Next() {
		var e SyncStatEntry
		rows.Scan(&e.ID, &e.TableName, &e.Total, &e.Pending, &e.Synced, &e.Errors, &e.CreatedAt)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []SyncStatEntry{}
	}
	return entries
}

// CleanOldSyncStats removes stats older than given hours
func (db *DB) CleanOldSyncStats(hours int) {
	db.conn.Exec(`DELETE FROM sync_stats WHERE created_at < datetime('now', ?)`,
		fmt.Sprintf("-%d hours", hours))
}

// ==================== USER PREFERENCES ====================

// GetUserPref retrieves a preference value for a user+key, returns empty string if not found
func (db *DB) GetUserPref(username, key string) string {
	var val string
	err := db.conn.QueryRow(
		`SELECT pref_value FROM user_preferences WHERE username=? AND pref_key=?`,
		username, key,
	).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

// SetUserPref saves a preference value for a user+key (upsert)
func (db *DB) SetUserPref(username, key, value string) error {
	_, err := db.conn.Exec(
		`INSERT INTO user_preferences (username, pref_key, pref_value, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(username, pref_key) DO UPDATE SET pref_value=excluded.pref_value, updated_at=CURRENT_TIMESTAMP`,
		username, key, value,
	)
	return err
}
