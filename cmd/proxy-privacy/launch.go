package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joaoeudes7/proxy-privacy/internal/config"
)

type Runner interface {
	Run(model string, args []string, proxyAddr, proxyKey string) error
	Name() string
}

func fetchProxyModels(proxyAddr, proxyKey string) ([]string, error) {
	return fetchProxyModelsWithClient(http.DefaultClient, proxyAddr, proxyKey)
}

func fetchProxyModelsWithClient(client *http.Client, proxyAddr, proxyKey string) ([]string, error) {
	modelsURL := openAIURL(proxyAddr) + "/models"

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+proxyKey)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			lastErr = fmt.Errorf("models endpoint returned %d", resp.StatusCode)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = err
			continue
		}

		ids := make([]string, 0, len(result.Data))
		for _, m := range result.Data {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		return ids, nil
	}
	return nil, lastErr
}

func ensureModelIDs(modelIDs []string, preferred ...string) []string {
	seen := make(map[string]struct{}, len(modelIDs)+len(preferred))
	merged := make([]string, 0, len(modelIDs)+len(preferred))

	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}

	for _, id := range modelIDs {
		add(id)
	}
	for _, id := range preferred {
		add(id)
	}

	return merged
}

func orderClaudeModelIDs(modelIDs []string, preferred string) []string {
	seen := make(map[string]struct{}, len(modelIDs)+1)
	ordered := make([]string, 0, len(modelIDs)+1)

	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	add(preferred)
	for _, id := range modelIDs {
		add(id)
	}

	return ordered
}

type claudeModelSlot struct {
	Alias string
	ID    string
}

func mapClaudeModelSlots(modelIDs []string) []claudeModelSlot {
	aliases := []string{"sonnet", "opus", "haiku", "fable"}
	slots := make([]claudeModelSlot, 0, len(aliases))
	for i, alias := range aliases {
		if i >= len(modelIDs) {
			break
		}
		if modelIDs[i] == "" {
			continue
		}
		slots = append(slots, claudeModelSlot{
			Alias: alias,
			ID:    modelIDs[i],
		})
	}
	return slots
}

func claudeSlotEnv(slots []claudeModelSlot) []string {
	env := make([]string, 0, len(slots)*3)
	for _, slot := range slots {
		upper := strings.ToUpper(slot.Alias)
		env = append(env,
			fmt.Sprintf("ANTHROPIC_DEFAULT_%s_MODEL=%s", upper, slot.ID),
			fmt.Sprintf("ANTHROPIC_DEFAULT_%s_MODEL_NAME=%s", upper, slot.ID),
			fmt.Sprintf("ANTHROPIC_DEFAULT_%s_MODEL_DESCRIPTION=%s", upper, "Proxy model via proxy-privacy"),
		)
	}
	return env
}

func selectedClaudeModel(model string) string {
	return model
}

type Integration struct {
	Name        string
	Aliases     []string
	Description string
	Runner      Runner
}

var integrations []*Integration

func registerIntegration(in *Integration) {
	integrations = append(integrations, in)
}

func lookupIntegration(name string) *Integration {
	name = strings.ToLower(name)
	for _, in := range integrations {
		if in.Name == name {
			return in
		}
		for _, alias := range in.Aliases {
			if alias == name {
				return in
			}
		}
	}
	return nil
}

func init() {
	registerIntegration(&Integration{
		Name: "claude", Aliases: []string{},
		Description: "Anthropic's coding agent (Claude Code)",
		Runner:      &ClaudeRunner{},
	})
	registerIntegration(&Integration{
		Name: "codex", Aliases: []string{},
		Description: "OpenAI's open-source coding agent",
		Runner:      &CodexRunner{},
	})
	registerIntegration(&Integration{
		Name: "kilo", Aliases: []string{"kilocode"},
		Description: "Open-source AI coding agent (fork of OpenCode by Kilo)",
		Runner:      &KiloRunner{},
	})
	registerIntegration(&Integration{
		Name: "opencode", Aliases: []string{},
		Description: "Anomaly's open-source coding agent",
		Runner:      &OpenCodeRunner{},
	})
	registerIntegration(&Integration{
		Name: "codex-app", Aliases: []string{"codex-desktop", "codex-gui"},
		Description: "Codex Desktop app by OpenAI",
		Runner:      &CodexAppRunner{},
	})
	registerIntegration(&Integration{
		Name: "cursor", Aliases: []string{},
		Description: "Cursor code editor",
		Runner:      &CursorRunner{},
	})
	registerIntegration(&Integration{
		Name: "cline", Aliases: []string{},
		Description: "Autonomous coding agent",
		Runner:      &ClineRunner{},
	})
	registerIntegration(&Integration{
		Name: "hermes", Aliases: []string{},
		Description: "Self-improving AI agent by Nous Research",
		Runner:      &HermesRunner{},
	})
	registerIntegration(&Integration{
		Name: "droid", Aliases: []string{},
		Description: "Factory's AI coding agent",
		Runner:      &DroidRunner{},
	})
	registerIntegration(&Integration{
		Name: "copilot", Aliases: []string{"copilot-cli"},
		Description: "GitHub's AI coding agent",
		Runner:      &CopilotRunner{},
	})
	registerIntegration(&Integration{
		Name: "pi", Aliases: []string{},
		Description: "Minimal AI agent toolkit",
		Runner:      &PiRunner{},
	})
	registerIntegration(&Integration{
		Name: "generic", Aliases: []string{"any", "default"},
		Description: "Any OpenAI-compatible agent (sets OPENAI_BASE_URL + OPENAI_API_KEY)",
		Runner:      &GenericRunner{},
	})
}

func printIntegrations() {
	fmt.Println("Supported integrations:")
	for _, in := range integrations {
		fmt.Printf("  %-12s  %s\n", in.Name, in.Description)
	}
}

func launchMain(args []string) {
	cfg, err := parseLaunchArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Println()
		printIntegrations()
		os.Exit(1)
	}

	if cfg.integrationName == "" {
		fmt.Println("Error: integration name required")
		fmt.Println()
		printIntegrations()
		os.Exit(1)
	}

	integration := lookupIntegration(cfg.integrationName)
	if integration == nil {
		fmt.Fprintf(os.Stderr, "Unknown integration: %s\n", cfg.integrationName)
		fmt.Println()
		printIntegrations()
		os.Exit(1)
	}

	proxyCfg, err := config.LoadProxyConfig()
	if err != nil {
		log.Printf("Warning: %v", err)
		proxyCfg = config.DefaultProxyConfig()
	}
	proxyCfg.PrivacyMode = config.PrivacyMode(cfg.privacyMode)
	proxyCfg.RedactPII = cfg.redactPII
	proxyCfg.RedactSecrets = cfg.redactSecrets
	appCfg := proxyCfg.AppConfig()
	envCfg := config.LoadEnvConfig()

	var launchSelected *config.ProviderConfig
	if provider, ok := config.SelectProvider(proxyCfg.Providers, cfg.provider); ok {
		launchSelected = &provider
		if envCfg.UpstreamBaseURL == "" {
			envCfg.UpstreamBaseURL = provider.BaseURL
		}
		if envCfg.APIKey == "" {
			envCfg.APIKey = provider.APIKey
		}
		if cfg.model != "" {
			appCfg.DefaultModel = cfg.model
			for i := range proxyCfg.Providers {
				if proxyCfg.Providers[i].ID == provider.ID {
					proxyCfg.Providers[i].DefaultModel = cfg.model
					break
				}
			}
		} else if provider.DefaultModel != "" {
			appCfg.DefaultModel = provider.DefaultModel
		}
		proxyCfg.DefaultModel = ""
		config.SaveProxyConfig(proxyCfg)
	} else if envCfg.UpstreamBaseURL == "" && envCfg.APIKey == "" {
		log.Fatal("No providers configured. Create ~/.proxy-privacy/configs.json or set UPSTREAM_BASE_URL and OPENAI_API_KEY env vars.")
	}

	if cfg.proxyKey != "" {
		envCfg.ProxyAPIKey = cfg.proxyKey
	}
	envCfg.DebugUpstream = envCfg.DebugUpstream || cfg.debugUpstream
	if cfg.traceDir != "" {
		envCfg.TraceDir = cfg.traceDir
	}
	if cfg.upstream != "" {
		envCfg.UpstreamBaseURL = cfg.upstream
	}
	if cfg.upstreamKey != "" {
		envCfg.APIKey = cfg.upstreamKey
	}
	if envCfg.UpstreamBaseURL == "" {
		log.Fatal("Upstream URL is required. Configure a provider in configs.json or set UPSTREAM_BASE_URL env var.")
	}
	if envCfg.APIKey == "" {
		log.Fatal("API key is required. Configure a provider in configs.json or set OPENAI_API_KEY env var.")
	}

	logResources, err := setupLogResources(envCfg.DebugUpstream, envCfg.TraceDir)
	if err != nil {
		log.Fatalf("Failed to set up logging: %v", err)
	}
	if logResources.cleanup != nil {
		defer logResources.cleanup()
	}

	s := newServer(appCfg, envCfg, launchSelected, logResources.traceLogger)
	if appCfg.DefaultModel == "" && s.Provider != nil {
		if id := fetchFirstModel(s.ProxyClient); id != "" {
			s.Cfg.DefaultModel = id
			appCfg.DefaultModel = id
			cfg.model = id
			s.Provider.DefaultModel = id
			proxyCfg, _ := config.LoadProxyConfig()
			if proxyCfg != nil {
				proxyCfg.DefaultModel = ""
				for i := range proxyCfg.Providers {
					if proxyCfg.Providers[i].ID == s.Provider.ID {
						proxyCfg.Providers[i].DefaultModel = id
						break
					}
				}
				config.SaveProxyConfig(proxyCfg)
			}
		}
	}
	srv, addr, err := listenAndServe(s, "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}

	proxyAddr := "http://" + addr
	fmt.Fprintf(os.Stderr, "  Proxy Privacy — %s mode — redact-pii=%v redact-secrets=%v — %s\n", appCfg.PrivacyMode, appCfg.RedactPII, appCfg.RedactSecrets, envCfg.UpstreamBaseURL)
	fmt.Fprintf(os.Stderr, "  Proxy running at %s\n", proxyAddr)
	if envCfg.DebugUpstream {
		logDir := envCfg.TraceDir
		if logDir == "" {
			if cwd, err := os.Getwd(); err == nil {
				logDir = cwd
			}
		}
		if logDir != "" {
			fmt.Fprintf(os.Stderr, "  Logs: %s/proxy-privacy.log\n", logDir)
			fmt.Fprintf(os.Stderr, "  Trace: %s/proxy-privacy.trace.log\n", logDir)
		}
	}

	if cfg.configOnly {
		fmt.Fprintf(os.Stderr, "  Configured %s with proxy at %s\n", integration.Name, proxyAddr)
		srv.Close()
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	errCh := make(chan error, 1)
	go func() {
		errCh <- integration.Runner.Run(cfg.model, cfg.extraArgs, proxyAddr, envCfg.ProxyAPIKey)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "\nShutting down...")
	}

	srv.Close()
}

type launchArgs struct {
	integrationName string
	provider        string
	privacyMode     string
	model           string
	proxyKey        string
	upstream        string
	upstreamKey     string
	sort            string
	providerOrder   string
	configOnly      bool
	debugUpstream   bool
	traceDir        string
	redactPII       bool
	redactSecrets   bool
	extraArgs       []string
}

func parseLaunchArgs(args []string) (*launchArgs, error) {
	cfg := &launchArgs{
		privacyMode:   "standard",
		redactSecrets: true,
	}

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			cfg.extraArgs = args[i+1:]
			break
		}

		if a == "--help" || a == "-h" {
			fmt.Println("Usage: proxy-privacy launch <integration> [options] [-- <extra-args>]")
			fmt.Println()
			fmt.Println("Options:")
			fmt.Println("  -p, --privacy string    Privacy mode: standard or strict (default \"standard\")")
			fmt.Println("  -m, --model string      Model to use (optional, agent discovers models if omitted)")
			fmt.Println("  -k, --api-key string    Proxy API key (default \"sk-proxy-dev\")")
			fmt.Println("      --provider string   Provider ID from configs.json (default: first provider)")
			fmt.Println("      --upstream string   Upstream base URL (overrides provider/config)")
			fmt.Println("      --upstream-key string Upstream API key (overrides provider/config)")
			fmt.Println("      --debug-upstream    Log upstream requests and payload summary")
			fmt.Println("      --trace-dir string  Directory for proxy log and trace files")
			fmt.Println("      --redact-pii            Redact email addresses from messages content (default: false)")
			fmt.Println("      --redact-secrets bool    Redact API keys and secrets from messages content (default: true)")
			fmt.Println("      --config-only       Configure without launching")
			fmt.Println()
			printIntegrations()
			fmt.Println()
			fmt.Println("AI setup help:")
			fmt.Println("  GitHub: https://github.com/joaoeudes7/proxy-privacy")
			fmt.Println()
			fmt.Println("Examples:")
			fmt.Println("  proxy-privacy launch claude --provider openai -m gpt-4o")
			fmt.Println("  proxy-privacy launch codex -m qwen3.5-coder -p strict")
			fmt.Println("  proxy-privacy launch opencode -m gpt-4o-mini -- --sandbox")
			return nil, fmt.Errorf("help requested")
		}

		if a == "--config-only" {
			cfg.configOnly = true
			continue
		}

		if a == "--debug-upstream" {
			cfg.debugUpstream = true
			continue
		}

		if a == "-s" || a == "--sort" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-s/--sort requires a value")
			}
			cfg.sort = args[i]
			continue
		}

		if a == "--provider-order" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--provider-order requires a value")
			}
			cfg.providerOrder = args[i]
			continue
		}

		if a == "--trace-dir" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--trace-dir requires a value")
			}
			cfg.traceDir = args[i]
			continue
		}

		if a == "--redact-pii" {
			cfg.redactPII = true
			continue
		}

		if a == "--redact-secrets" {
			// default is true, so only explicit --redact-secrets=false disables it
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--redact-secrets requires a value")
			}
			switch args[i] {
			case "false", "0", "no":
				cfg.redactSecrets = false
			case "true", "1", "yes":
				cfg.redactSecrets = true
			default:
				return nil, fmt.Errorf("--redact-secrets must be 'true' or 'false', got %q", args[i])
			}
			continue
		}

		if a == "--provider" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--provider requires a value")
			}
			cfg.provider = args[i]
			continue
		}

		if a == "-p" || a == "--privacy" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-p/--privacy requires a value")
			}
			cfg.privacyMode = args[i]
			if cfg.privacyMode != "standard" && cfg.privacyMode != "strict" {
				return nil, fmt.Errorf("invalid privacy mode %q. Use 'standard' or 'strict'", cfg.privacyMode)
			}
			continue
		}

		if a == "-m" || a == "--model" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-m/--model requires a value")
			}
			cfg.model = args[i]
			continue
		}

		if a == "-k" || a == "--api-key" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-k/--api-key requires a value")
			}
			cfg.proxyKey = args[i]
			continue
		}

		if a == "--upstream" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--upstream requires a value")
			}
			cfg.upstream = args[i]
			continue
		}

		if a == "--upstream-key" {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--upstream-key requires a value")
			}
			cfg.upstreamKey = args[i]
			continue
		}

		if !strings.HasPrefix(a, "-") && cfg.integrationName == "" {
			cfg.integrationName = a
		}
	}

	if cfg.integrationName == "" {
		return nil, fmt.Errorf("integration name required")
	}

	return cfg, nil
}

func anthropicURL(proxyAddr string) string {
	return strings.TrimRight(proxyAddr, "/")
}

func openAIURL(proxyAddr string) string {
	return strings.TrimRight(proxyAddr, "/") + "/v1"
}

func openAIURLSlash(proxyAddr string) string {
	return strings.TrimRight(proxyAddr, "/") + "/v1/"
}

type ClaudeRunner struct{}

func (r *ClaudeRunner) Name() string { return "Claude Code" }

func (r *ClaudeRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	path, err := exec.LookPath("claude")
	if err != nil {
		home, err2 := os.UserHomeDir()
		if err2 == nil {
			fallback := filepath.Join(home, ".claude", "local", "claude")
			if runtime.GOOS == "windows" {
				fallback += ".exe"
			}
			if _, err2 = os.Stat(fallback); err2 == nil {
				path = fallback
				err = nil
			}
		}
	}
	if err != nil {
		return fmt.Errorf("claude is not installed. Install from https://code.claude.com")
	}

	cmdArgs := args
	modelName := selectedClaudeModel(model)
	displayModel := modelName
	if displayModel == "" {
		displayModel = "(auto)"
	}
	if model != "" {
		cmdArgs = append([]string{"--model", model}, args...)
	}

	modelIDs := []string{}
	if modelName != "" {
		modelIDs = append(modelIDs, modelName)
	}
	settingsPath, cleanup, err := writeClaudeSettings(modelIDs)
	if err != nil {
		return fmt.Errorf("write claude settings: %w", err)
	}
	defer cleanup()
	fmt.Fprintf(os.Stderr, "Alert: Claude will show only one proxy model in /model: %s\n", displayModel)
	fmt.Fprintf(os.Stderr, "Advice: to choose another proxy model, start it via this proxy launcher with -m/--model, for example: proxy-privacy launch claude -m <model-id>\n")
	cmdArgs = append([]string{"--settings", settingsPath}, cmdArgs...)

	env := append(os.Environ(),
		"ANTHROPIC_BASE_URL="+anthropicURL(proxyAddr),
		"ANTHROPIC_API_KEY="+proxyKey,
		"ANTHROPIC_AUTH_TOKEN="+proxyKey,
		"CLAUDE_CODE_ATTRIBUTION_HEADER=0",
	)
	env = append(env, claudeSlotEnv(mapClaudeModelSlots(modelIDs))...)
	if model != "" {
		env = append(env, "CLAUDE_CODE_SUBAGENT_MODEL="+model)
	}

	cmd := exec.Command(path, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}

func writeClaudeSettings(modelIDs []string) (string, func(), error) {
	slots := mapClaudeModelSlots(modelIDs)
	available := make([]string, 0, len(slots))
	overrides := make(map[string]string, len(slots))
	for _, slot := range slots {
		available = append(available, slot.Alias)
		overrides[slot.Alias] = slot.ID
	}

	settings := map[string]any{
		"availableModels":        available,
		"enforceAvailableModels": true,
		"modelOverrides":         overrides,
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return "", nil, err
	}

	return writeManagedTempJSON("proxy-privacy-claude-settings.json", data)
}

type CodexRunner struct{}

func (r *CodexRunner) Name() string { return "Codex" }

func (r *CodexRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex is not installed. Install with: npm install -g @openai/codex")
	}

	modelIDs, _ := fetchProxyModels(proxyAddr, proxyKey)
	modelIDs = ensureModelIDs(modelIDs, model)

	catalogPath, cleanup, err := writeCodexModelCatalog(modelIDs)
	if err != nil {
		return fmt.Errorf("write codex model catalog: %w", err)
	}
	defer cleanup()

	cmd := exec.Command("codex", buildCodexCommandArgs(model, proxyKey, proxyAddr, catalogPath, args)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}

func writeCodexModelCatalog(modelIDs []string) (string, func(), error) {
	models := make([]map[string]any, 0, len(modelIDs))
	for _, id := range modelIDs {
		if id == "" {
			continue
		}
		models = append(models, map[string]any{
			"slug":                          id,
			"display_name":                  id,
			"description":                   nil,
			"supported_reasoning_levels":    []any{},
			"shell_type":                    "shell_command",
			"visibility":                    "list",
			"supported_in_api":              true,
			"priority":                      0,
			"availability_nux":              nil,
			"upgrade":                       nil,
			"base_instructions":             "",
			"supports_reasoning_summaries":  false,
			"default_reasoning_summary":     "auto",
			"support_verbosity":             false,
			"default_verbosity":             nil,
			"apply_patch_tool_type":         nil,
			"web_search_tool_type":          "text",
			"truncation_policy":             map[string]any{"mode": "bytes", "limit": 100000},
			"supports_parallel_tool_calls":  true,
			"supports_image_detail_original": false,
			"experimental_supported_tools":  []string{},
			"input_modalities":              []string{"text", "image"},
			"supports_search_tool":          false,
			"use_responses_lite":            false,
		})
	}

	catalog := map[string]any{"models": models}
	data, err := json.Marshal(catalog)
	if err != nil {
		return "", nil, err
	}

	return writeManagedTempJSON("proxy-privacy-codex-models.json", data)
}

func writeManagedTempJSON(name string, data []byte) (string, func(), error) {
	path := filepath.Join(os.TempDir(), name)
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "Replacing temporary file: %s\n", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", nil, err
	}

	cleanup := func() {
		_ = os.Remove(path)
	}
	return path, cleanup, nil
}

func buildCodexCommandArgs(model, proxyKey, proxyAddr, catalogPath string, extraArgs []string) []string {
	cmdArgs := []string{"--profile", "proxy-privacy-launch"}
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_provider=%s", "proxy-privacy"))
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_providers.proxy-privacy.name=%s", "Proxy Privacy"))
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_providers.proxy-privacy.base_url=%s", openAIURLSlash(proxyAddr)))
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_providers.proxy-privacy.env_key=%s", "OPENAI_API_KEY"))
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_providers.proxy-privacy.requires_openai_auth=%t", true))
	cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_providers.proxy-privacy.wire_api=%s", "responses"))
	if catalogPath != "" {
		cmdArgs = append(cmdArgs, "-c", fmt.Sprintf("model_catalog_json=%s", catalogPath))
	}
	if model != "" {
		cmdArgs = append(cmdArgs, "-m", model)
	}
	cmdArgs = append(cmdArgs, extraArgs...)
	return cmdArgs
}

type OpenCodeRunner struct{}

func (r *OpenCodeRunner) Name() string { return "OpenCode" }

type openCodeConfig struct {
	Schema   string           `json:"$schema"`
	Provider openCodeProvider `json:"provider"`
	Model    string           `json:"model"`
}

type openCodeProvider struct {
	ProxyPrivacy openCodeCustomProvider `json:"proxy-privacy"`
}

type openCodeCustomProvider struct {
	Name    string                    `json:"name"`
	Options openCodeProviderOptions   `json:"options"`
	Models  map[string]openCodeModel  `json:"models,omitempty"`
}

type openCodeProviderOptions struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey,omitempty"`
}

type openCodeModel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func (r *OpenCodeRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	opencodePath, ok := findOpenCode()
	if !ok {
		return fmt.Errorf("opencode is not installed. Install from https://opencode.ai")
	}

	modelIDs, _ := fetchProxyModels(proxyAddr, proxyKey)
	modelName := model
	if modelName == "" && len(modelIDs) > 0 {
		modelName = modelIDs[0]
	}
	models := make(map[string]openCodeModel, len(modelIDs)+1)
	for _, id := range modelIDs {
		if id == "" {
			continue
		}
		clean := id
		if strings.Contains(id, "/") {
			parts := strings.SplitN(id, "/", 2)
			clean = parts[1]
		}
		models[clean] = openCodeModel{ID: id, Name: formatModelDisplayName(id)}
	}
	if modelName != "" {
		clean := modelName
		if strings.Contains(modelName, "/") {
			parts := strings.SplitN(modelName, "/", 2)
			clean = parts[1]
		}
		models[clean] = openCodeModel{ID: modelName, Name: formatModelDisplayName(modelName)}
	}

	cfg := openCodeConfig{
		Schema: "https://opencode.ai/config.json",
		Provider: openCodeProvider{
			ProxyPrivacy: openCodeCustomProvider{
				Name: "Proxy Privacy",
				Options: openCodeProviderOptions{
					BaseURL: openAIURL(proxyAddr),
					APIKey:  proxyKey,
				},
				Models: models,
			},
		},
		Model: "proxy-privacy/" + modelName,
	}
	cfgBytes, _ := json.Marshal(cfg)

	env := append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
		"OPENCODE_CONFIG_CONTENT="+string(cfgBytes),
	)

	cmd := exec.Command(opencodePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}

func findOpenCode() (string, bool) {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	name := "opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fallback := filepath.Join(home, ".opencode", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return fallback, true
	}
	return "", false
}

type KiloRunner struct{}

func (r *KiloRunner) Name() string { return "Kilo Code" }

func (r *KiloRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	kiloPath, ok := findKilo()
	if !ok {
		return fmt.Errorf("kilo is not installed. Install with: npm install -g @kilocode/cli or brew install Kilo-Org/tap/kilo")
	}

	modelIDs, _ := fetchProxyModels(proxyAddr, proxyKey)
	modelName := model
	if modelName == "" && len(modelIDs) > 0 {
		modelName = modelIDs[0]
	}
	models := make(map[string]openCodeModel, len(modelIDs)+1)
	for _, id := range modelIDs {
		if id == "" {
			continue
		}
		clean := id
		if strings.Contains(id, "/") {
			parts := strings.SplitN(id, "/", 2)
			clean = parts[1]
		}
		models[clean] = openCodeModel{ID: id, Name: formatModelDisplayName(id)}
	}
	if modelName != "" {
		clean := modelName
		if strings.Contains(modelName, "/") {
			parts := strings.SplitN(modelName, "/", 2)
			clean = parts[1]
		}
		models[clean] = openCodeModel{ID: modelName, Name: formatModelDisplayName(modelName)}
	}

	cfg := openCodeConfig{
		Schema: "https://opencode.ai/config.json",
		Provider: openCodeProvider{
			ProxyPrivacy: openCodeCustomProvider{
				Name: "Proxy Privacy",
				Options: openCodeProviderOptions{
					BaseURL: openAIURL(proxyAddr),
					APIKey:  proxyKey,
				},
				Models: models,
			},
		},
		Model: "proxy-privacy/" + modelName,
	}
	cfgBytes, _ := json.Marshal(cfg)

	env := append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
		"OPENCODE_CONFIG_CONTENT="+string(cfgBytes),
	)

	cmd := exec.Command(kiloPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}

func findKilo() (string, bool) {
	if p, err := exec.LookPath("kilo"); err == nil {
		return p, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	name := "kilo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fallback := filepath.Join(home, ".kilo", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return fallback, true
	}
	return "", false
}

type CodexAppRunner struct{}

func (r *CodexAppRunner) Name() string { return "Codex App" }

func (r *CodexAppRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	return fmt.Errorf("codex-app is a desktop application. Configure it manually with:\n  OPENAI_BASE_URL=%s\n  OPENAI_API_KEY=%s", openAIURL(proxyAddr), proxyKey)
}

type CursorRunner struct{}

func (r *CursorRunner) Name() string { return "Cursor" }

func (r *CursorRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	return fmt.Errorf("cursor is a desktop editor. Configure it manually with:\n  OPENAI_BASE_URL=%s\n  OPENAI_API_KEY=%s", openAIURL(proxyAddr), proxyKey)
}

type ClineRunner struct{}

func (r *ClineRunner) Name() string { return "Cline" }

func (r *ClineRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if _, err := exec.LookPath("cline"); err != nil {
		return fmt.Errorf("cline is not installed. Install with: npm install -g cline@latest")
	}

	cmd := exec.Command("cline", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}

type HermesRunner struct{}

func (r *HermesRunner) Name() string { return "Hermes Agent" }

func (r *HermesRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if _, err := exec.LookPath("hermes"); err != nil {
		return fmt.Errorf("hermes is not installed. Install from https://hermes-agent.nousresearch.com")
	}

	cmdArgs := args
	if model != "" {
		cmdArgs = append([]string{"--model", model}, args...)
	}

	cmd := exec.Command("hermes", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}

type DroidRunner struct{}

func (r *DroidRunner) Name() string { return "Droid" }

func (r *DroidRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if _, err := exec.LookPath("droid"); err != nil {
		return fmt.Errorf("droid is not installed. Install from https://docs.factory.ai/cli")
	}

	cmdArgs := args
	if model != "" {
		cmdArgs = append([]string{"--model", model}, args...)
	}

	cmd := exec.Command("droid", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}

type CopilotRunner struct{}

func (r *CopilotRunner) Name() string { return "Copilot CLI" }

func (r *CopilotRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	path, err := copilotPath()
	if err != nil {
		return fmt.Errorf("copilot is not installed. See https://github.com/features/copilot/cli/")
	}

	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}

func copilotPath() (string, error) {
	if p, err := exec.LookPath("github-copilot-cli"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("copilot"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("not found")
}

type PiRunner struct{}

func (r *PiRunner) Name() string { return "Pi" }

func (r *PiRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if _, err := exec.LookPath("pi"); err != nil {
		return fmt.Errorf("pi is not installed. Install with: npm install -g @earendil-works/pi-coding-agent@latest")
	}

	restore, err := writePiProxyProvider(proxyAddr, proxyKey, model)
	if err != nil {
		return err
	}
	defer restore()

	cmdArgs := args
	if model != "" {
		cmdArgs = append([]string{"--model", model}, args...)
	}

	cmd := exec.Command("pi", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// piModelsPath is the path to Pi's custom models configuration.
var piModelsPath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent", "models.json")
}

var piHTTPClient = http.DefaultClient

// writePiProxyProvider fetches all models from the proxy and registers them
// in the proxy-privacy provider so Pi sees every model as available.
func writePiProxyProvider(proxyAddr, proxyKey, modelID string) (func(), error) {
	piDir := filepath.Dir(piModelsPath())
	if err := os.MkdirAll(piDir, 0755); err != nil {
		return nil, fmt.Errorf("create pi agent dir: %w", err)
	}

	var cfg struct {
		Providers map[string]any `json:"providers"`
	}
	existing, err := os.ReadFile(piModelsPath())
	if err == nil {
		json.Unmarshal(existing, &cfg)
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]any)
	}

	backup := existing

	modelIDs, _ := fetchProxyModelsWithClient(piHTTPClient, proxyAddr, proxyKey)
	modelIDs = ensureModelIDs(modelIDs, modelID)
	allModels := make([]map[string]any, 0, len(modelIDs))
	for _, id := range modelIDs {
		if id != "" {
			allModels = append(allModels, map[string]any{"id": id})
		}
	}

	apiKey := proxyKey
	if apiKey == "" {
		apiKey = "sk-proxy-dev"
	}

	cfg.Providers["proxy-privacy"] = map[string]any{
		"baseUrl":    openAIURL(proxyAddr),
		"apiKey":     apiKey,
		"api":        "openai-completions",
		"authHeader": true,
		"models":     allModels,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal pi provider config: %w", err)
	}

	if err := os.WriteFile(piModelsPath(), append(data, '\n'), 0644); err != nil {
		return nil, fmt.Errorf("write pi models.json: %w", err)
	}

	return func() {
		if backup != nil {
			os.WriteFile(piModelsPath(), backup, 0644)
		} else {
			os.Remove(piModelsPath())
		}
	}, nil
}

type GenericRunner struct{}

func (r *GenericRunner) Name() string { return "Generic OpenAI-compatible" }

func (r *GenericRunner) Run(model string, args []string, proxyAddr, proxyKey string) error {
	if len(args) == 0 {
		return fmt.Errorf("generic mode requires a command to run. Example:\n  proxy-privacy launch generic -- my-agent --flag value")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+proxyKey,
		"OPENAI_BASE_URL="+openAIURL(proxyAddr),
	)
	return cmd.Run()
}
