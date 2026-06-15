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
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	BaseURL       string                   `json:"base_url"`
	APIKey        string                   `json:"api_key"`
	DefaultModel  string                   `json:"default_model,omitempty"`
	Default       bool                     `json:"default,omitempty"`
	ProviderPrefs map[string]any           `json:"provider_prefs,omitempty"`
	Models        map[string]map[string]any `json:"models,omitempty"`
}

type EnvConfig struct {
	APIKey          string
	ProxyAPIKey     string
	UpstreamBaseURL string
	DebugUpstream   bool
	TraceDir        string
}

type ProxyConfig struct {
	DefaultModel  string           `json:"default_model,omitempty"`
	PrivacyMode   PrivacyMode      `json:"privacy_mode"`
	RedactPII     bool             `json:"redact_pii"`
	RedactSecrets bool             `json:"redact_secrets"`
	AllowedModels []string         `json:"allowed_models,omitempty"`
	Providers     []ProviderConfig `json:"providers"`
}

const DefaultClientName = "github.com/joaoeudes7/proxy-privacy"

func DefaultProxyConfig() *ProxyConfig {
	return &ProxyConfig{
		PrivacyMode:   PrivacyStandard,
		RedactSecrets: true,
	}
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		PrivacyMode:   PrivacyStandard,
		RedactSecrets: true,
	}
}

func (c *ProxyConfig) AppConfig() AppConfig {
	return AppConfig{
		DefaultModel:  c.DefaultModel,
		PrivacyMode:   c.PrivacyMode,
		RedactPII:     c.RedactPII,
		RedactSecrets: c.RedactSecrets,
		AllowedModels: c.AllowedModels,
	}
}

func (c *ProxyConfig) ApplyAppConfig(a AppConfig) {
	if a.DefaultModel != "" {
		c.DefaultModel = a.DefaultModel
	}
	if a.PrivacyMode != "" {
		c.PrivacyMode = a.PrivacyMode
	}
	c.RedactPII = a.RedactPII
	c.RedactSecrets = a.RedactSecrets
	if a.AllowedModels != nil {
		c.AllowedModels = a.AllowedModels
	}
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".proxy-privacy"), nil
}

func oldConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "configs.json"), nil
}

func ConfigsPath() (string, error) {
	return ConfigPath()
}

func LoadProxyConfig() (*ProxyConfig, error) {
	cfg := DefaultProxyConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("reading configs.json: %w", err)
		}
		return migrateOldConfigs(cfg)
	}

	var probe struct {
		Providers []ProviderConfig `json:"providers"`
	}
	if err := json.Unmarshal(data, &probe); err == nil && probe.Providers != nil {
		var unified ProxyConfig
		if err := json.Unmarshal(data, &unified); err == nil {
			if unified.PrivacyMode == "" {
				unified.PrivacyMode = PrivacyStandard
			}
			return &unified, nil
		}
	}

	var oldProviders []ProviderConfig
	if err := json.Unmarshal(data, &oldProviders); err == nil {
		cfg.Providers = oldProviders
		mergeOldAppConfig(cfg)
		return cfg, nil
	}

	return cfg, fmt.Errorf("invalid configs.json: unknown format")
}

func SaveProxyConfig(cfg *ProxyConfig) error {
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

func migrateOldConfigs(cfg *ProxyConfig) (*ProxyConfig, error) {
	oldPath, err := oldConfigPath()
	if err != nil {
		return cfg, nil
	}

	oldData, err := os.ReadFile(oldPath)
	if err == nil {
		var oldCfg AppConfig
		if uerr := json.Unmarshal(oldData, &oldCfg); uerr == nil {
			cfg.ApplyAppConfig(oldCfg)
		}
	}

	return cfg, nil
}

func mergeOldAppConfig(cfg *ProxyConfig) {
	oldPath, err := oldConfigPath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}
	var oldCfg AppConfig
	if err := json.Unmarshal(data, &oldCfg); err != nil {
		return
	}
	cfg.ApplyAppConfig(oldCfg)
}

func LoadAppConfig() (AppConfig, error) {
	cfg, err := LoadProxyConfig()
	if err != nil {
		return DefaultAppConfig(), err
	}
	return cfg.AppConfig(), nil
}

func SaveAppConfig(a AppConfig) error {
	cfg, err := LoadProxyConfig()
	if err != nil {
		cfg = DefaultProxyConfig()
	}
	cfg.ApplyAppConfig(a)
	return SaveProxyConfig(cfg)
}

func LoadConfigs() ([]ProviderConfig, error) {
	cfg, err := LoadProxyConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Providers, nil
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
