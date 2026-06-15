package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLaunchArgs_defaultPrivacy(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "standard" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "standard")
	}
	if cfg.integrationName != "pi" {
		t.Errorf("integrationName = %q, want %q", cfg.integrationName, "pi")
	}
}

func TestParseLaunchArgs_strictFlag(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "-p", "strict"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "strict" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "strict")
	}
	if cfg.integrationName != "pi" {
		t.Errorf("integrationName = %q, want %q", cfg.integrationName, "pi")
	}
}

func TestParseLaunchArgs_strictLongFlag(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"opencode", "--privacy", "strict"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "strict" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "strict")
	}
	if cfg.integrationName != "opencode" {
		t.Errorf("integrationName = %q, want %q", cfg.integrationName, "opencode")
	}
}

func TestParseLaunchArgs_standardExplicit(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"claude", "-p", "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "standard" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "standard")
	}
}

func TestParseLaunchArgs_invalidPrivacy(t *testing.T) {
	_, err := parseLaunchArgs([]string{"pi", "-p", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid privacy mode, got nil")
	}
}

func TestParseLaunchArgs_missingPrivacyValue(t *testing.T) {
	_, err := parseLaunchArgs([]string{"pi", "-p"})
	if err == nil {
		t.Fatal("expected error for missing privacy value, got nil")
	}
}

func TestParseLaunchArgs_withModel(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "-m", "gpt-4", "-p", "strict"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "strict" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "strict")
	}
	if cfg.model != "gpt-4" {
		t.Errorf("model = %q, want %q", cfg.model, "gpt-4")
	}
}

func TestParseLaunchArgs_integrationNameOnly(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"codex"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.integrationName != "codex" {
		t.Errorf("integrationName = %q, want %q", cfg.integrationName, "codex")
	}
	if cfg.privacyMode != "standard" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "standard")
	}
}

func TestParseLaunchArgs_missingIntegration(t *testing.T) {
	_, err := parseLaunchArgs([]string{})
	if err == nil {
		t.Fatal("expected error for missing integration, got nil")
	}
}

func TestParseLaunchArgs_withExtraArgs(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"opencode", "-p", "strict", "--", "--sandbox", "--flag"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.privacyMode != "strict" {
		t.Errorf("privacyMode = %q, want %q", cfg.privacyMode, "strict")
	}
	if len(cfg.extraArgs) != 2 || cfg.extraArgs[0] != "--sandbox" || cfg.extraArgs[1] != "--flag" {
		t.Errorf("extraArgs = %v, want [--sandbox --flag]", cfg.extraArgs)
	}
}

func TestParseLaunchArgs_debugUpstream(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "--debug-upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.debugUpstream {
		t.Error("debugUpstream = false, want true")
	}
}

func TestParseLaunchArgs_traceDir(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "--trace-dir", "/tmp/traces"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.traceDir != "/tmp/traces" {
		t.Errorf("traceDir = %q, want %q", cfg.traceDir, "/tmp/traces")
	}
}

func TestParseLaunchArgs_configOnly(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "--config-only"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.configOnly {
		t.Error("configOnly = false, want true")
	}
}

func TestParseLaunchArgs_apiKey(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"pi", "-k", "custom-key"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.proxyKey != "custom-key" {
		t.Errorf("proxyKey = %q, want %q", cfg.proxyKey, "custom-key")
	}
}

func TestParseLaunchArgs_provider(t *testing.T) {
	cfg, err := parseLaunchArgs([]string{"codex", "--provider", "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.provider != "openai" {
		t.Errorf("provider = %q, want %q", cfg.provider, "openai")
	}
	if cfg.integrationName != "codex" {
		t.Errorf("integrationName = %q, want %q", cfg.integrationName, "codex")
	}
}

func TestParseLaunchArgs_providerMissingValue(t *testing.T) {
	_, err := parseLaunchArgs([]string{"pi", "--provider"})
	if err == nil {
		t.Fatal("expected error for missing --provider value, got nil")
	}
}

func TestBuildCodexCommandArgs_ConfiguresProxyProvider(t *testing.T) {
	args := buildCodexCommandArgs("gpt-5", "sk-proxy-dev", "http://127.0.0.1:8000", "/tmp/catalog.json", []string{"exec", "--skip-git-repo-check"})
	joined := strings.Join(args, " ")

	required := []string{
		"--profile proxy-privacy-launch",
		"-c model_provider=proxy-privacy",
		"-c model_providers.proxy-privacy.name=Proxy Privacy",
		"-c model_providers.proxy-privacy.base_url=http://127.0.0.1:8000/v1/",
		"-c model_providers.proxy-privacy.env_key=OPENAI_API_KEY",
		"-c model_providers.proxy-privacy.requires_openai_auth=true",
		"-c model_providers.proxy-privacy.wire_api=responses",
		"-c model_catalog_json=/tmp/catalog.json",
		"-m gpt-5",
		"exec --skip-git-repo-check",
	}
	for _, want := range required {
		if !strings.Contains(joined, want) {
			t.Fatalf("joined args missing %q: %s", want, joined)
		}
	}
}

func TestBuildCodexCommandArgs_UsesProxyProviderWithoutHardcodedModel(t *testing.T) {
	args := buildCodexCommandArgs("", "sk-proxy-dev", "http://127.0.0.1:8000", "/tmp/catalog.json", []string{"exec"})
	joined := strings.Join(args, " ")

	required := []string{
		"--profile proxy-privacy-launch",
		"-c model_provider=proxy-privacy",
		"-c model_providers.proxy-privacy.name=Proxy Privacy",
		"-c model_providers.proxy-privacy.base_url=http://127.0.0.1:8000/v1/",
		"-c model_catalog_json=/tmp/catalog.json",
		"-c model_providers.proxy-privacy.wire_api=responses",
		"exec",
	}
	for _, want := range required {
		if !strings.Contains(joined, want) {
			t.Fatalf("joined args missing %q: %s", want, joined)
		}
	}

	if strings.Contains(joined, "-m ") {
		t.Fatalf("joined args unexpectedly hardcode a model: %s", joined)
	}
}

func TestWriteCodexModelCatalog_WritesListVisibleModels(t *testing.T) {
	path, cleanup, err := writeCodexModelCatalog([]string{"model-a", "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var catalog struct {
		Models []struct {
			Slug           string `json:"slug"`
			DisplayName    string `json:"display_name"`
			Visibility     string `json:"visibility"`
			SupportedInAPI bool   `json:"supported_in_api"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}

	if len(catalog.Models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(catalog.Models))
	}
	if catalog.Models[0].Slug != "model-a" || catalog.Models[0].DisplayName != "model-a" {
		t.Fatalf("first model = %+v", catalog.Models[0])
	}
	if catalog.Models[0].Visibility != "list" || !catalog.Models[0].SupportedInAPI {
		t.Fatalf("first model visibility/api flags = %+v", catalog.Models[0])
	}
	if catalog.Models[1].Slug != "model-b" || catalog.Models[1].DisplayName != "model-b" {
		t.Fatalf("second model = %+v", catalog.Models[1])
	}
}

func TestWriteClaudeSettings_UsesAllowlistAndEnforcement(t *testing.T) {
	path, cleanup, err := writeClaudeSettings([]string{"model-z"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var settings struct {
		AvailableModels        []string `json:"availableModels"`
		EnforceAvailableModels bool     `json:"enforceAvailableModels"`
		ModelOverrides         map[string]string `json:"modelOverrides"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	if !settings.EnforceAvailableModels {
		t.Fatal("enforceAvailableModels = false, want true")
	}

	want := []string{"sonnet"}
	if len(settings.AvailableModels) != len(want) {
		t.Fatalf("len(availableModels) = %d, want %d (%v)", len(settings.AvailableModels), len(want), settings.AvailableModels)
	}
	for i := range want {
		if settings.AvailableModels[i] != want[i] {
			t.Fatalf("availableModels[%d] = %q, want %q (all=%v)", i, settings.AvailableModels[i], want[i], settings.AvailableModels)
		}
	}
	if len(settings.ModelOverrides) != 1 || settings.ModelOverrides["sonnet"] != "model-z" {
		t.Fatalf("modelOverrides = %v", settings.ModelOverrides)
	}
}

func TestOrderClaudeModelIDs_PutsPreferredFirstWithoutDuplicates(t *testing.T) {
	got := orderClaudeModelIDs([]string{"model-a", "model-b", "model-c", "model-a"}, "model-b")
	want := []string{"model-b", "model-a", "model-c"}

	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestMapClaudeModelSlots_AssignsPickerAliases(t *testing.T) {
	got := mapClaudeModelSlots([]string{"model-1"})
	want := []claudeModelSlot{
		{Alias: "sonnet", ID: "model-1"},
	}

	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %+v, want %+v (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestSelectedClaudeModel(t *testing.T) {
	if got := selectedClaudeModel(""); got != "" {
		t.Fatalf("selectedClaudeModel(\"\") = %q, want empty", got)
	}
	if got := selectedClaudeModel("model-a"); got != "model-a" {
		t.Fatalf("selectedClaudeModel(\"model-a\") = %q, want %q", got, "model-a")
	}
}

func TestFetchProxyModels_UsesAuthAndReturnsAllProxyModels(t *testing.T) {
	var authHeader string
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/models")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"model-a"},{"id":"model-b"},{"id":"model-c"}]}`)),
		}, nil
	})}
	defer func() {
		http.DefaultClient = originalClient
	}()

	models, err := fetchProxyModels("http://proxy.test", "proxy-key")
	if err != nil {
		t.Fatal(err)
	}

	if authHeader != "Bearer proxy-key" {
		t.Fatalf("Authorization = %q, want %q", authHeader, "Bearer proxy-key")
	}

	want := []string{"model-a", "model-b", "model-c"}
	if len(models) != len(want) {
		t.Fatalf("len(models) = %d, want %d (%v)", len(models), len(want), models)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("models[%d] = %q, want %q (all=%v)", i, models[i], want[i], models)
		}
	}
}

func TestFetchProxyModels_RetriesUntilSuccess(t *testing.T) {
	attempts := 0
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("try again")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"model-final"}]}`)),
		}, nil
	})}
	defer func() {
		http.DefaultClient = originalClient
	}()

	models, err := fetchProxyModels("http://proxy.test", "proxy-key")
	if err != nil {
		t.Fatal(err)
	}

	if attempts != 3 {
		t.Fatalf("attempts = %d, want %d", attempts, 3)
	}
	if len(models) != 1 || models[0] != "model-final" {
		t.Fatalf("models = %v, want [model-final]", models)
	}
}

func TestWritePiProxyProvider_UsesAuthAndPreservesExistingProviders(t *testing.T) {
	tmpDir := t.TempDir()
	originalFn := piModelsPath
	piModelsPath = func() string {
		return filepath.Join(tmpDir, "models.json")
	}
	defer func() {
		piModelsPath = originalFn
	}()

	tmpPath := piModelsPath()
	if err := os.WriteFile(tmpPath, []byte(`{"providers":{"existing":{"baseUrl":"http://keep.me"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	var authHeader string
	originalClient := piHTTPClient
	piHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		authHeader = req.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"model-a"},{"id":"model-b"}]}`)),
		}, nil
	})}
	defer func() {
		piHTTPClient = originalClient
	}()

	proxyURL := "http://proxy.test"
	restore, err := writePiProxyProvider(proxyURL, "proxy-key", "model-c")
	if err != nil {
		t.Fatal(err)
	}
	defer restore()

	if authHeader != "Bearer proxy-key" {
		t.Fatalf("Authorization = %q, want %q", authHeader, "Bearer proxy-key")
	}

	data, err := os.ReadFile(piModelsPath())
	if err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		Providers map[string]map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	if _, ok := cfg.Providers["existing"]; !ok {
		t.Fatal("existing provider should be preserved")
	}

	proxyProvider, ok := cfg.Providers["proxy-privacy"]
	if !ok {
		t.Fatal("proxy-privacy provider missing")
	}
	if proxyProvider["baseUrl"] != proxyURL+"/v1" {
		t.Fatalf("baseUrl = %v, want %s/v1", proxyProvider["baseUrl"], proxyURL)
	}

	models, ok := proxyProvider["models"].([]any)
	if !ok {
		t.Fatalf("models type = %T, want []any", proxyProvider["models"])
	}
	var ids []string
	for _, item := range models {
		entry := item.(map[string]any)
		ids = append(ids, entry["id"].(string))
	}

	for _, want := range []string{"model-a", "model-b", "model-c"} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("models missing %q: %v", want, ids)
		}
	}
}
