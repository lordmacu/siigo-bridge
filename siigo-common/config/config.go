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
	SendEnabled     map[string]bool       `json:"send_enabled,omitempty"`
	AllowEditDelete bool                  `json:"allow_edit_delete"`
	SetupComplete   bool                  `json:"setup_complete"`
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
			DataPath: `C:\DEMOS01\`,
		},
		Finearom: FinearomConfig{
			BaseURL:  "https://ordenes.finearom.co/api",
			Email:    "siigo-sync@finearom.com",
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
	}
}

// DefaultSendEnabled returns all modules disabled by default.
// The user must explicitly enable sending from the web UI or bot.
func DefaultSendEnabled() map[string]bool {
	return map[string]bool{
		"clients":   false,
		"products":  false,
		"movements": false,
		"cartera":   false,
	}
}

// IsSendEnabled checks if sending is enabled for a given table
func (c *Config) IsSendEnabled(table string) bool {
	if c.SendEnabled == nil {
		return false // if not configured, default to disabled
	}
	enabled, ok := c.SendEnabled[table]
	if !ok {
		return false
	}
	return enabled
}

// DefaultFieldMappings returns the default field mapping for all modules
func DefaultFieldMappings() map[string][]FieldMap {
	return map[string][]FieldMap{
		"clients": {
			{Source: "nit", ApiKey: "nit", Label: "NIT", Enabled: true},
			{Source: "client_name", ApiKey: "client_name", Label: "Nombre Cliente", Enabled: true},
			{Source: "business_name", ApiKey: "business_name", Label: "Razon Social", Enabled: true},
			{Source: "taxpayer_type", ApiKey: "taxpayer_type", Label: "Tipo Documento", Enabled: true},
			{Source: "tipo_clave", ApiKey: "tipo_clave", Label: "Tipo Clave", Enabled: false},
			{Source: "siigo_empresa", ApiKey: "siigo_empresa", Label: "Empresa Siigo", Enabled: false},
			{Source: "siigo_codigo", ApiKey: "siigo_codigo", Label: "Codigo Siigo", Enabled: false},
			{Source: "fecha_creacion", ApiKey: "fecha_creacion", Label: "Fecha Creacion", Enabled: false},
		},
		"products": {
			{Source: "code", ApiKey: "code", Label: "Codigo", Enabled: true},
			{Source: "product_name", ApiKey: "product_name", Label: "Nombre Producto", Enabled: true},
			{Source: "grupo", ApiKey: "grupo", Label: "Grupo", Enabled: true},
			{Source: "referencia", ApiKey: "referencia", Label: "Referencia", Enabled: true},
		},
		"movements": {
			{Source: "tipo_comprobante", ApiKey: "tipo_comprobante", Label: "Tipo Comprobante", Enabled: true},
			{Source: "numero_doc", ApiKey: "numero_doc", Label: "Numero Documento", Enabled: true},
			{Source: "fecha", ApiKey: "fecha", Label: "Fecha", Enabled: true},
			{Source: "nit_tercero", ApiKey: "nit_tercero", Label: "NIT Tercero", Enabled: true},
			{Source: "cuenta_contable", ApiKey: "cuenta_contable", Label: "Cuenta Contable", Enabled: true},
			{Source: "descripcion", ApiKey: "descripcion", Label: "Descripcion", Enabled: true},
			{Source: "valor", ApiKey: "valor", Label: "Valor", Enabled: true},
			{Source: "tipo_mov", ApiKey: "tipo_mov", Label: "Tipo Movimiento", Enabled: true},
		},
		"cartera": {
			{Source: "nit", ApiKey: "nit", Label: "NIT", Enabled: true},
			{Source: "cuenta_contable", ApiKey: "cuenta_contable", Label: "Cuenta Contable", Enabled: true},
			{Source: "fecha", ApiKey: "fecha", Label: "Fecha", Enabled: true},
			{Source: "tipo_movimiento", ApiKey: "tipo_movimiento", Label: "Tipo Movimiento", Enabled: true},
			{Source: "descripcion", ApiKey: "descripcion", Label: "Descripcion", Enabled: true},
			{Source: "tipo_registro", ApiKey: "tipo_registro", Label: "Tipo Registro", Enabled: true},
		},
	}
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

// EnsureFieldMappings initializes field mappings if not set
func (c *Config) EnsureFieldMappings() {
	if c.FieldMappings == nil {
		c.FieldMappings = DefaultFieldMappings()
	}
	if c.SendEnabled == nil {
		c.SendEnabled = DefaultSendEnabled()
	}
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
