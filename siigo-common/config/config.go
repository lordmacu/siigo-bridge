package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Siigo    SiigoConfig    `json:"siigo"`
	Finearom FinearomConfig `json:"finearom"`
	Sync     SyncConfig     `json:"sync"`
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
	IntervalSeconds int      `json:"interval_seconds"`
	Files           []string `json:"files"`
	StatePath       string   `json:"state_path"`
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
	return &cfg, nil
}

func Default() *Config {
	return &Config{
		Siigo: SiigoConfig{
			DataPath: `C:\DEMOS01\`,
		},
		Finearom: FinearomConfig{
			BaseURL:  "http://localhost:8000/api",
			Email:    "sync@finearom.com",
			Password: "password",
		},
		Sync: SyncConfig{
			IntervalSeconds: 60,
			Files:           []string{"Z17", "Z06CP", "Z49", "Z092024"},
			StatePath:       "sync_state.json",
		},
	}
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
