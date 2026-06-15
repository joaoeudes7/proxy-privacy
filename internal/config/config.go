package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var writeMu sync.Mutex

type PrivacyMode string

const (
	PrivacyStandard PrivacyMode = "standard"
	PrivacyStrict   PrivacyMode = "strict"
)

type AppConfig struct {
	DefaultModel  string      `json:"default_model"`
	PrivacyMode   PrivacyMode `json:"privacy_mode"`
	RedactPII     bool        `json:"redact_pii"`
	RedactSecrets bool        `json:"redact_secrets"`
	AllowedModels []string    `json:"allowed_models,omitempty"`
}

type ProviderConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	DefaultModel string `json:"default_model,omitempty"`
	Default      bool   `json:"default,omitempty"`
}

type EnvConfig struct {
	APIKey          string
	ProxyAPIKey     string
	UpstreamBaseURL string
	DebugUpstream   bool
	TraceDir        string
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		DefaultModel: "",
		PrivacyMode:  PrivacyStandard,
		RedactPII:    false,
		RedactSecrets: true,
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".proxy-privacy", "config.json"), nil
}

func LoadAppConfig() (AppConfig, error) {
	cfg := DefaultAppConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func SaveAppConfig(cfg AppConfig) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	return os.WriteFile(path, data, 0644)
}

func ConfigsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".proxy-privacy", "configs.json"), nil
}

func LoadConfigs() ([]ProviderConfig, error) {
	path, err := ConfigsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading configs: %w", err)
	}
	var configs []ProviderConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("invalid configs.json: %w", err)
	}
	return configs, nil
}

func SelectProvider(configs []ProviderConfig, id string) (ProviderConfig, bool) {
	if len(configs) == 0 {
		return ProviderConfig{}, false
	}
	if id == "" {
		for _, p := range configs {
			if p.Default {
				return p, true
			}
		}
		return configs[0], true
	}
	for _, p := range configs {
		if p.ID == id || p.Name == id {
			return p, true
		}
	}
	return ProviderConfig{}, false
}

func LoadEnvConfig() EnvConfig {
	return EnvConfig{
		APIKey:          os.Getenv("OPENAI_API_KEY"),
		ProxyAPIKey:     getEnvDefault("PROXY_API_KEY", "sk-proxy-dev"),
		UpstreamBaseURL: os.Getenv("UPSTREAM_BASE_URL"),
		DebugUpstream:   getEnvBool("PROXY_DEBUG_UPSTREAM"),
		TraceDir:        os.Getenv("PROXY_TRACE_DIR"),
	}
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}
