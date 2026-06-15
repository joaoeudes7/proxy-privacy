package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAppConfig(t *testing.T) {
	cfg := DefaultAppConfig()
	if cfg.DefaultModel != "" {
		t.Errorf("DefaultModel = %q, want empty", cfg.DefaultModel)
	}
	if cfg.PrivacyMode != PrivacyStandard {
		t.Errorf("PrivacyMode = %q, want %q", cfg.PrivacyMode, PrivacyStandard)
	}
	if cfg.RedactPII {
		t.Error("RedactPII = true, want false")
	}
	if !cfg.RedactSecrets {
		t.Error("RedactSecrets = false, want true")
	}
}

func TestLoadEnvConfig_defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PROXY_API_KEY", "")
	t.Setenv("UPSTREAM_BASE_URL", "")
	t.Setenv("PROXY_DEBUG_UPSTREAM", "")
	t.Setenv("PROXY_TRACE_DIR", "")

	env := LoadEnvConfig()
	if env.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", env.APIKey)
	}
	if env.ProxyAPIKey != "sk-proxy-dev" {
		t.Errorf("ProxyAPIKey = %q, want %q", env.ProxyAPIKey, "sk-proxy-dev")
	}
	if env.UpstreamBaseURL != "" {
		t.Errorf("UpstreamBaseURL = %q, want empty", env.UpstreamBaseURL)
	}
	if env.DebugUpstream {
		t.Error("DebugUpstream = true, want false")
	}
	if env.TraceDir != "" {
		t.Errorf("TraceDir = %q, want empty", env.TraceDir)
	}
}

func TestLoadEnvConfig_apiKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-123")

	env := LoadEnvConfig()
	if env.APIKey != "sk-openai-123" {
		t.Errorf("APIKey = %q, want %q", env.APIKey, "sk-openai-123")
	}
}

func TestLoadEnvConfig_proxyAPIKeyDefault(t *testing.T) {
	t.Setenv("PROXY_API_KEY", "custom-key")
	env := LoadEnvConfig()
	if env.ProxyAPIKey != "custom-key" {
		t.Errorf("ProxyAPIKey = %q, want %q", env.ProxyAPIKey, "custom-key")
	}
}

func TestLoadEnvConfig_upstreamBaseURL(t *testing.T) {
	t.Setenv("UPSTREAM_BASE_URL", "https://api.openai.com/v1")
	env := LoadEnvConfig()
	if env.UpstreamBaseURL != "https://api.openai.com/v1" {
		t.Errorf("UpstreamBaseURL = %q, want %q", env.UpstreamBaseURL, "https://api.openai.com/v1")
	}
}

func TestLoadEnvConfig_debugUpstream(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "True", "yes", "YES", "on", "ON"}
	for _, v := range truthy {
		t.Run("debug_"+v, func(t *testing.T) {
			t.Setenv("PROXY_DEBUG_UPSTREAM", v)
			env := LoadEnvConfig()
			if !env.DebugUpstream {
				t.Errorf("DebugUpstream(%q) = false, want true", v)
			}
		})
	}
}

func TestLoadEnvConfig_debugUpstream_false(t *testing.T) {
	falsy := []string{"", "0", "false", "FALSE", "off", "OFF", "no", "NO", "anything"}
	for _, v := range falsy {
		t.Run("debug_"+v, func(t *testing.T) {
			t.Setenv("PROXY_DEBUG_UPSTREAM", v)
			env := LoadEnvConfig()
			if env.DebugUpstream {
				t.Errorf("DebugUpstream(%q) = true, want false", v)
			}
		})
	}
}

func TestLoadEnvConfig_traceDir(t *testing.T) {
	t.Setenv("PROXY_TRACE_DIR", "/tmp/traces")
	env := LoadEnvConfig()
	if env.TraceDir != "/tmp/traces" {
		t.Errorf("TraceDir = %q, want %q", env.TraceDir, "/tmp/traces")
	}
}

func TestConfigPath(t *testing.T) {
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".proxy-privacy", "configs.json")
	if path != want {
		t.Errorf("ConfigPath() = %q, want %q", path, want)
	}
}

func TestLoadAppConfig_noFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Must return defaults
	if cfg.DefaultModel != "" {
		t.Errorf("DefaultModel = %q, want empty", cfg.DefaultModel)
	}
}

func TestLoadAppConfig_validJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	saved := AppConfig{
		DefaultModel:  "gpt-4",
		PrivacyMode:   PrivacyStrict,
		RedactPII:     false,
		AllowedModels: []string{"gpt-4", "claude-3"},
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4")
	}
	if cfg.PrivacyMode != PrivacyStrict {
		t.Errorf("PrivacyMode = %q, want %q", cfg.PrivacyMode, PrivacyStrict)
	}
	if cfg.RedactPII {
		t.Error("RedactPII = true, want false")
	}
	if len(cfg.AllowedModels) != 2 || cfg.AllowedModels[0] != "gpt-4" {
		t.Errorf("AllowedModels = %v, want %v", cfg.AllowedModels, []string{"gpt-4", "claude-3"})
	}
}

func TestLoadAppConfig_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "configs.json"), []byte("{bad json}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSaveAppConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := AppConfig{
		DefaultModel:  "custom-model",
		PrivacyMode:   PrivacyStrict,
		RedactPII:     false,
		AllowedModels: []string{"model-a"},
	}
	if err := SaveAppConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	configFile := filepath.Join(dir, ".proxy-privacy", "configs.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}

	var loaded ProxyConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.PrivacyMode != PrivacyStrict {
		t.Errorf("PrivacyMode = %q, want %q", loaded.PrivacyMode, PrivacyStrict)
	}
	if loaded.RedactPII {
		t.Error("RedactPII = true, want false")
	}
	if len(loaded.AllowedModels) != 1 || loaded.AllowedModels[0] != "model-a" {
		t.Errorf("AllowedModels = %v, want %v", loaded.AllowedModels, []string{"model-a"})
	}
}

func TestConfigsPath(t *testing.T) {
	path, err := ConfigsPath()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".proxy-privacy", "configs.json")
	if path != want {
		t.Errorf("ConfigsPath() = %q, want %q", path, want)
	}
}

func TestLoadConfigs_noFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configs, err := LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 {
		t.Errorf("configs = %v, want empty", configs)
	}
}

func TestLoadConfigs_validJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configs := []ProviderConfig{
		{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", APIKey: "sk-123", DefaultModel: "gpt-4o"},
		{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-or-456"},
	}
	data, _ := json.Marshal(configs)
	if err := os.WriteFile(filepath.Join(configDir, "configs.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len = %d, want 2", len(loaded))
	}
	if loaded[0].ID != "openai" || loaded[0].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("loaded[0] = %+v", loaded[0])
	}
	if loaded[1].ID != "openrouter" || loaded[1].APIKey != "sk-or-456" {
		t.Errorf("loaded[1] = %+v", loaded[1])
	}
}

func TestLoadConfigs_readsDefaultProviderFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configs := []ProviderConfig{
		{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", APIKey: "sk-123"},
		{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-or-456", Default: true},
	}
	data, _ := json.Marshal(configs)
	if err := os.WriteFile(filepath.Join(configDir, "configs.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded[1].Default {
		t.Fatalf("loaded default flag = false, want true: %+v", loaded[1])
	}
}

func TestSelectProvider_empty(t *testing.T) {
	_, ok := SelectProvider(nil, "")
	if ok {
		t.Error("SelectProvider(nil) = ok, want false")
	}
	_, ok = SelectProvider([]ProviderConfig{}, "any")
	if ok {
		t.Error("SelectProvider(empty) = ok, want false")
	}
}

func TestSelectProvider_first(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "a", Name: "Alpha"},
		{ID: "b", Name: "Beta"},
	}
	p, ok := SelectProvider(configs, "")
	if !ok {
		t.Fatal("SelectProvider(first) = not ok")
	}
	if p.ID != "a" {
		t.Errorf("ID = %q, want %q", p.ID, "a")
	}
}

func TestSelectProvider_defaultFlag(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "a", Name: "Alpha"},
		{ID: "b", Name: "Beta", Default: true},
	}
	p, ok := SelectProvider(configs, "")
	if !ok {
		t.Fatal("SelectProvider(default) = not ok")
	}
	if p.ID != "b" {
		t.Errorf("ID = %q, want %q", p.ID, "b")
	}
}

func TestSelectProvider_byID(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "a", Name: "Alpha"},
		{ID: "b", Name: "Beta"},
	}
	p, ok := SelectProvider(configs, "b")
	if !ok {
		t.Fatal("SelectProvider(id) = not ok")
	}
	if p.ID != "b" {
		t.Errorf("ID = %q, want %q", p.ID, "b")
	}
}

func TestSelectProvider_byName(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "a", Name: "Alpha"},
		{ID: "b", Name: "Beta"},
	}
	p, ok := SelectProvider(configs, "Alpha")
	if !ok {
		t.Fatal("SelectProvider(name) = not ok")
	}
	if p.ID != "a" {
		t.Errorf("ID = %q, want %q", p.ID, "a")
	}
}

func TestSelectProvider_notFound(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "a", Name: "Alpha"},
	}
	_, ok := SelectProvider(configs, "nonexistent")
	if ok {
		t.Error("SelectProvider(missing) = ok, want false")
	}
}

func TestSelectProvider_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "configs.json"), []byte("not json"), 0644)

	_, err := LoadConfigs()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadConfigs_emptyArray(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "configs.json"), []byte("[]"), 0644)

	configs, err := LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 {
		t.Errorf("len = %d, want 0", len(configs))
	}
}

func TestLoadConfigs_partialInvalidSkipsEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "configs.json"), []byte(`[{"id":"ok","name":"OK","base_url":"http://ok","api_key":"k"},{"bad}`), 0644)

	_, err := LoadConfigs()
	if err == nil {
		t.Fatal("expected error for partial invalid JSON, got nil")
	}
}

func TestSelectProvider_returnsFirstWhenEmptyID(t *testing.T) {
	configs := []ProviderConfig{
		{ID: "first", Name: "First Provider"},
		{ID: "second", Name: "Second Provider"},
	}
	p, ok := SelectProvider(configs, "")
	if !ok {
		t.Fatal("SelectProvider(first) = not ok")
	}
	if p.ID != "first" {
		t.Errorf("ID = %q, want %q", p.ID, "first")
	}
}

func TestProviderConfig_AllFieldsPreserved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".proxy-privacy")
	os.MkdirAll(configDir, 0755)
	cfg := []ProviderConfig{
		{
			ID:           "test-provider",
			Name:         "Test Provider",
			BaseURL:      "https://test.api.com/v1",
			APIKey:       "sk-test-123",
			DefaultModel: "test-model-v2",
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "configs.json"), data, 0644)

	loaded, err := LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len = %d, want 1", len(loaded))
	}
	p := loaded[0]
	if p.ID != "test-provider" || p.Name != "Test Provider" || p.BaseURL != "https://test.api.com/v1" || p.APIKey != "sk-test-123" || p.DefaultModel != "test-model-v2" {
		t.Errorf("provider = %+v", p)
	}
}

func TestSaveAppConfig_createsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Directory .proxy-privacy does not exist yet
	cfg := DefaultAppConfig()
	if err := SaveAppConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify directory and file were created
	configDir := filepath.Join(dir, ".proxy-privacy")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("SaveAppConfig did not create the config directory")
	}
	configFile := filepath.Join(configDir, "configs.json")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("SaveAppConfig did not create the config file")
	}
}
