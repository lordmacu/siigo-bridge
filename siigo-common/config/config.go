package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Auth            AuthConfig            `json:"auth"`
	Server          ServerConfig          `json:"server,omitempty"`
	Siigo           SiigoConfig           `json:"siigo"`
	Finearom        FinearomConfig        `json:"finearom"`
	Sync            SyncConfig            `json:"sync"`
	PublicAPI       PublicAPIConfig        `json:"public_api"`
	Telegram        TelegramConfig        `json:"telegram"`
	Webhooks        WebhookConfig         `json:"webhooks"`
	FieldMappings   map[string][]FieldMap `json:"field_mappings,omitempty"`
	SendEnabled       map[string]bool       `json:"send_enabled,omitempty"`
	DetectEnabled     map[string]bool       `json:"detect_enabled,omitempty"`
	GlobalSendEnabled bool                  `json:"global_send_enabled"`
	AllowEditDelete   bool                  `json:"allow_edit_delete"`
	Terminal          TerminalConfig        `json:"terminal,omitempty"`
	SetupComplete   bool                  `json:"setup_complete"`
}

type TerminalConfig struct {
	// PinHash is the bcrypt hash of the extra access PIN required to open the web terminal.
	// Empty = no PIN, admin/root JWT is enough. Set via POST /api/terminal/set-pin.
	PinHash string `json:"pin_hash,omitempty"`
}

type ServerConfig struct {
	Port string `json:"port,omitempty"` // default "3210"
}

type WebhookConfig struct {
	Enabled bool          `json:"enabled"`
	Hooks   []WebhookDef  `json:"hooks,omitempty"`
}

type WebhookDef struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"` // HMAC-SHA256 signing key
	Events []string `json:"events"`           // e.g. ["sync_complete","sync_error","send_complete","record_change"]
	Active bool     `json:"active"`
}

type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
	ChatID   int64  `json:"chat_id"`
	ExecPin  string `json:"exec_pin"`
	// Notification toggles (per type)
	NotifyServerStart  *bool `json:"notify_server_start,omitempty"`
	NotifySyncComplete *bool `json:"notify_sync_complete,omitempty"`
	NotifySyncErrors   *bool `json:"notify_sync_errors,omitempty"`
	NotifyLoginFailed  *bool `json:"notify_login_failed,omitempty"`
	NotifyChanges      *bool `json:"notify_changes,omitempty"`
	NotifyDBCleared    *bool `json:"notify_db_cleared,omitempty"`
	NotifyMaxRetries   *bool `json:"notify_max_retries,omitempty"`
}

// IsNotifyEnabled checks if a notification type is enabled.
// Server start defaults to true; all others default to false.
func (t *TelegramConfig) IsNotifyEnabled(notifType string) bool {
	switch notifType {
	case "server_start":
		if t.NotifyServerStart == nil {
			return true // default enabled
		}
		return *t.NotifyServerStart
	case "sync_complete":
		if t.NotifySyncComplete == nil {
			return false
		}
		return *t.NotifySyncComplete
	case "sync_errors":
		if t.NotifySyncErrors == nil {
			return false
		}
		return *t.NotifySyncErrors
	case "login_failed":
		if t.NotifyLoginFailed == nil {
			return false
		}
		return *t.NotifyLoginFailed
	case "changes":
		if t.NotifyChanges == nil {
			return false
		}
		return *t.NotifyChanges
	case "db_cleared":
		if t.NotifyDBCleared == nil {
			return false
		}
		return *t.NotifyDBCleared
	case "max_retries":
		if t.NotifyMaxRetries == nil {
			return false
		}
		return *t.NotifyMaxRetries
	default:
		return false
	}
}

type PublicAPIConfig struct {
	Enabled     bool   `json:"enabled"`
	JwtRequired bool   `json:"jwt_required"`
	ApiKey      string `json:"api_key"`
	JwtSecret   string `json:"jwt_secret"`
}

// FieldMap defines how a single field is sent to the API
type FieldMap struct {
	Source  string `json:"source"`  // key in the Data map (from DB)
	ApiKey  string `json:"api_key"` // key to use when sending to API
	Label   string `json:"label"`   // display name for UI
	Enabled bool   `json:"enabled"` // whether to include in API payload
}

type SiigoConfig struct {
	DataPath string `json:"data_path"`
}

type FinearomConfig struct {
	BaseURL  string `json:"base_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SyncConfig struct {
	IntervalSeconds         int      `json:"interval_seconds"`
	SendIntervalSeconds     int      `json:"send_interval_seconds"`
	BatchSize               int      `json:"batch_size"`
	BatchDelayMs            int      `json:"batch_delay_ms"`
	MaxRetries              int      `json:"max_retries"`
	RetryDelaySeconds       int      `json:"retry_delay_seconds"`
	CircuitBreakerThreshold int      `json:"circuit_breaker_threshold,omitempty"` // consecutive failures before auto-pause (default 4)
	Files                   []string `json:"files"`
	StatePath               string   `json:"state_path"`
}

// GetCircuitBreakerThreshold returns the configured threshold or default of 4
func (s *SyncConfig) GetCircuitBreakerThreshold() int {
	if s.CircuitBreakerThreshold > 0 {
		return s.CircuitBreakerThreshold
	}
	return 4
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.EnsureFieldMappings()
	return &cfg, nil
}

func Default() *Config {
	return &Config{
		Auth: AuthConfig{
			Username: "admin",
			Password: "change-me",
		},
		Siigo: SiigoConfig{
			DataPath: `C:\SIIWI02`,
		},
		Finearom: FinearomConfig{
			BaseURL:  "",
			Email:    "",
			Password: "",
		},
		Sync: SyncConfig{
			IntervalSeconds:     60,
			SendIntervalSeconds: 30,
			BatchSize:           50,
			BatchDelayMs:        500,
			MaxRetries:          3,
			RetryDelaySeconds:   30,
			Files:             []string{"Z17", "Z04", "Z49", "Z092024"},
			StatePath:         "sync_state.json",
		},
		PublicAPI: PublicAPIConfig{
			Enabled:     true,
			JwtRequired: true,
			ApiKey:      "change-me-to-a-secure-key",
			JwtSecret:   "change-me-to-a-random-secret",
		},
		Telegram: TelegramConfig{
			Enabled:  false,
			BotToken: "",
			ChatID:   0,
			ExecPin:  "2337",
		},
		FieldMappings: DefaultFieldMappings(),
		SendEnabled:   DefaultSendEnabled(),
		DetectEnabled: DefaultDetectEnabled(),
	}
}

// allSyncTables is the canonical list of all tables that support detect and send.
var allSyncTables = []string{
	"clients", "products", "cartera",
	"documentos",
	"condiciones_pago", "codigos_dane",
	"formulas",
	"vendedores_areas",
	"notas_documentos", "facturas_electronicas", "detalle_movimientos",
	"cartera_cxc", "ventas_productos",
}

// AllSyncTables returns the canonical list of all syncable tables.
func AllSyncTables() []string {
	return allSyncTables
}

// DefaultSendEnabled returns all modules disabled by default.
// The user must explicitly enable sending from the web UI or bot.
func DefaultSendEnabled() map[string]bool {
	m := make(map[string]bool, len(allSyncTables))
	for _, t := range allSyncTables {
		m[t] = false
	}
	return m
}

// DefaultDetectEnabled returns all modules enabled by default.
// Detection (ISAM → SQLite) runs for all tables unless explicitly disabled.
func DefaultDetectEnabled() map[string]bool {
	m := make(map[string]bool, len(allSyncTables))
	for _, t := range allSyncTables {
		m[t] = true
	}
	return m
}

// IsSendEnabled checks if sending is enabled for a given table.
// Both the global send toggle AND the per-table toggle must be enabled.
func (c *Config) IsSendEnabled(table string) bool {
	if !c.GlobalSendEnabled {
		return false
	}
	if c.SendEnabled == nil {
		return false
	}
	enabled, ok := c.SendEnabled[table]
	if !ok {
		return false
	}
	return enabled
}

// IsDetectEnabled checks if detection (ISAM → SQLite) is enabled for a given table.
// Defaults to true if not configured.
func (c *Config) IsDetectEnabled(table string) bool {
	if c.DetectEnabled == nil {
		return true // default: detect everything
	}
	enabled, ok := c.DetectEnabled[table]
	if !ok {
		return true // default: enabled
	}
	return enabled
}

// columnLabels provides human-readable labels for common SQLite column names.
var columnLabels = map[string]string{
	"nit": "NIT", "nombre": "Nombre", "empresa": "Empresa", "tipo_persona": "Tipo Persona",
	"rep_legal": "Rep. Legal", "direccion": "Direccion", "email": "Email",
	"codigo": "Codigo", "nombre_corto": "Nombre Corto", "grupo": "Grupo", "referencia": "Referencia",
	"tipo_comprobante": "Tipo Comprobante", "numero_doc": "Numero Documento", "nit_tercero": "NIT Tercero",
	"tipo_registro": "Tipo Registro", "secuencia": "Secuencia", "tipo_doc": "Tipo Documento",
	"cuenta_contable": "Cuenta Contable", "fecha": "Fecha", "descripcion": "Descripcion",
	"tipo_mov": "Tipo Movimiento", "tipo_movimiento": "Tipo Movimiento",
	"naturaleza": "Naturaleza", "activa": "Activa", "auxiliar": "Auxiliar",
	"nit_responsable": "NIT Responsable", "fecha_adquisicion": "Fecha Adquisicion",
	"saldo_anterior": "Saldo Anterior", "debito": "Debito", "credito": "Credito", "saldo_final": "Saldo Final",
	"codigo_comp": "Codigo Comprobante", "bodega": "Bodega", "centro_costo": "Centro Costo",
	"producto_ref": "Producto Ref", "representante_legal": "Rep. Legal",
	"fecha_documento": "Fecha Documento", "fecha_vencimiento": "Fecha Vencimiento", "valor": "Valor",
	"numero_periodo": "Numero Periodo", "fecha_inicio": "Fecha Inicio", "fecha_fin": "Fecha Fin",
	"saldo1": "Saldo 1", "saldo2": "Saldo 2", "saldo3": "Saldo 3",
	"tipo": "Tipo", "fecha_registro": "Fecha Registro", "tipo_secundario": "Tipo Secundario",
	"saldo": "Saldo", "tarifa": "Tarifa", "fondo": "Fondo", "concepto": "Concepto",
	"flags": "Flags", "tipo_base": "Tipo Base", "base_calculo": "Base Calculo",
	"valor_compra": "Valor Compra", "fecha_cambio": "Fecha Cambio", "timestamp": "Timestamp",
	"usuario": "Usuario", "fecha_periodo": "Fecha Periodo", "nombre_representante": "Nombre Rep.",
	"nit_representante": "NIT Rep.", "codigo_grupo": "Codigo Grupo", "codigo_detalle": "Codigo Detalle",
	"codigo_producto": "Codigo Producto", "cantidad": "Cantidad",
	"saldo_inicial": "Saldo Inicial", "entradas": "Entradas", "salidas": "Salidas",
	"tipo_reg": "Tipo Registro", "sub_tipo": "Sub Tipo", "nombre_origen": "Nombre Origen",
	"nombre_destin": "Nombre Destino", "responsable": "Responsable",
	"grupo_producto": "Grupo Producto", "codigo_ingrediente": "Codigo Ingrediente",
	"grupo_ingrediente": "Grupo Ingrediente", "porcentaje": "Porcentaje",
	"hora": "Hora", "seq": "Secuencia", "usuario_crea": "Usuario Crea",
	"usuario_modifica": "Usuario Modifica", "fecha_modifica": "Fecha Modifica",
	"modulo_origen": "Modulo Origen", "campo_modificado": "Campo Modificado",
	"ciudad": "Ciudad", "record_key": "Clave Registro", "code": "Codigo",
}

// tableColumns defines the SQLite columns (excluding internal ones) for each sync table.
// Used to auto-generate field mappings. Key column is listed first.
var tableColumns = map[string][]string{
	"clients":                 {"nit", "nombre", "empresa", "tipo_persona", "rep_legal", "direccion", "email"},
	"products":                {"code", "nombre", "nombre_corto", "grupo", "referencia", "empresa"},
	"cartera":                 {"record_key", "tipo_registro", "empresa", "secuencia", "tipo_doc", "nit_tercero", "cuenta_contable", "fecha", "descripcion", "tipo_mov"},
	"documentos":              {"record_key", "tipo_comprobante", "codigo_comp", "secuencia", "nit_tercero", "cuenta_contable", "producto_ref", "bodega", "centro_costo", "fecha", "descripcion", "tipo_mov", "referencia"},
	"condiciones_pago":        {"record_key", "tipo", "empresa", "secuencia", "tipo_doc", "fecha", "nit", "tipo_secundario", "valor", "fecha_registro"},
	"codigos_dane":            {"codigo", "nombre"},
	"formulas":                {"record_key", "empresa", "grupo_producto", "codigo_producto", "grupo_ingrediente", "codigo_ingrediente", "porcentaje"},
	"vendedores_areas":        {"record_key", "tipo", "codigo", "nombre", "nombre_corto", "ciudad", "nit", "direccion", "email"},
}

// DefaultFieldMappings returns the default field mapping for all modules.
// All fields are enabled by default — the user disables what they don't need.
func DefaultFieldMappings() map[string][]FieldMap {
	result := make(map[string][]FieldMap, len(tableColumns))
	for table, cols := range tableColumns {
		fields := make([]FieldMap, 0, len(cols))
		for _, col := range cols {
			label := columnLabels[col]
			if label == "" {
				label = col // fallback to column name
			}
			fields = append(fields, FieldMap{
				Source:  col,
				ApiKey:  col,
				Label:   label,
				Enabled: true,
			})
		}
		result[table] = fields
	}
	return result
}

// ApplyFieldMapping filters a data map through the field mapping for a given table.
// Returns a new map with only enabled fields, renamed to their api_key.
func (c *Config) ApplyFieldMapping(table string, data map[string]interface{}) map[string]interface{} {
	mappings, ok := c.FieldMappings[table]
	if !ok || len(mappings) == 0 {
		return data // no mapping configured, send everything
	}

	result := make(map[string]interface{})
	for _, fm := range mappings {
		if !fm.Enabled {
			continue
		}
		if val, exists := data[fm.Source]; exists {
			result[fm.ApiKey] = val
		}
	}
	return result
}

// EnsureFieldMappings initializes field mappings if not set.
// Also ensures any new tables added since the config was created get their default mappings.
func (c *Config) EnsureFieldMappings() {
	defaults := DefaultFieldMappings()
	validTables := make(map[string]bool, len(allSyncTables))
	for _, t := range allSyncTables {
		validTables[t] = true
	}
	if c.FieldMappings == nil {
		c.FieldMappings = defaults
	} else {
		// Add mappings for any tables that don't exist yet in config
		for table, fields := range defaults {
			if _, ok := c.FieldMappings[table]; !ok {
				c.FieldMappings[table] = fields
			}
		}
		// Remove mappings for tables no longer active
		for table := range c.FieldMappings {
			if !validTables[table] {
				delete(c.FieldMappings, table)
			}
		}
	}
	if c.SendEnabled == nil {
		c.SendEnabled = DefaultSendEnabled()
	}
	// Ensure all tables exist in SendEnabled (for configs created before expansion)
	for _, t := range allSyncTables {
		if _, ok := c.SendEnabled[t]; !ok {
			c.SendEnabled[t] = false
		}
	}
	// Remove stale tables from SendEnabled
	for t := range c.SendEnabled {
		if !validTables[t] {
			delete(c.SendEnabled, t)
		}
	}
	if c.DetectEnabled == nil {
		c.DetectEnabled = DefaultDetectEnabled()
	}
	// Ensure all tables exist in DetectEnabled
	for _, t := range allSyncTables {
		if _, ok := c.DetectEnabled[t]; !ok {
			c.DetectEnabled[t] = true
		}
	}
	// Remove stale tables from DetectEnabled
	for t := range c.DetectEnabled {
		if !validTables[t] {
			delete(c.DetectEnabled, t)
		}
	}
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
