package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joaoeudes7/proxy-privacy/internal/anthropic"
	"github.com/joaoeudes7/proxy-privacy/internal/config"
	"github.com/joaoeudes7/proxy-privacy/internal/privacy"
	"github.com/joaoeudes7/proxy-privacy/internal/proxy"
)

type ModelCache struct {
	mu    sync.RWMutex
	known map[string]bool
}

func NewModelCache() *ModelCache {
	return &ModelCache{known: make(map[string]bool)}
}

func (c *ModelCache) RejectsStrict(model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.known[model]
}

func (c *ModelCache) MarkRejectsStrict(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.known[model] = true
}

type Server struct {
	mu          sync.RWMutex
	Cfg         config.AppConfig
	Env         config.EnvConfig
	ProxyClient *proxy.Client
	ModelCache  *ModelCache
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "launch" {
		launchMain(os.Args[2:])
		return
	}

	providerFlag := flag.String("provider", "", "Provider ID from configs.json (default: first provider)")
	privacyFlag := flag.String("privacy", "standard", "Privacy mode: standard or strict")
	privacyShort := flag.String("p", "standard", "Privacy mode: standard or strict")
	modelFlag := flag.String("model", "", "Default model (auto-fetches from /v1/models if empty)")
	modelShort := flag.String("m", "", "Default model (auto-fetches from /v1/models if empty)")
	upstreamFlag := flag.String("upstream", "", "Upstream base URL (overrides provider config)")
	upstreamKeyFlag := flag.String("upstream-key", "", "Upstream API key (overrides provider config)")
	debugUpstreamFlag := flag.Bool("debug-upstream", false, "Log upstream headers and privacy payload summary")
	traceDirFlag := flag.String("trace-dir", "", "Directory for proxy log and trace files (default: current directory when debug is enabled)")
	portFlag := flag.Int("port", 8000, "Server port")
	hostFlag := flag.String("host", "127.0.0.1", "Bind address")
	keyFlag := flag.String("api-key", "", "Proxy API key (overrides env)")
	keyShort := flag.String("k", "", "Proxy API key (overrides env)")
	redactPIIFlag := flag.Bool("redact-pii", false, "Redact email addresses from messages content (default false)")
	redactSecretsFlag := flag.Bool("redact-secrets", true, "Redact API keys and secrets from messages content")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: proxy-privacy [options]\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nGitHub: https://github.com/joaoeudes7/proxy-privacy\n")
	}
	flag.Parse()

	cfg, env := resolveConfig(
		*providerFlag,
		*privacyFlag, *privacyShort, *modelFlag, *modelShort,
		*upstreamFlag, *upstreamKeyFlag, *keyFlag, *keyShort, *debugUpstreamFlag, *traceDirFlag,
		*redactPIIFlag, *redactSecretsFlag,
	)

	logResources, err := setupLogResources(env.DebugUpstream, env.TraceDir)
	if err != nil {
		log.Fatalf("Failed to set up logging: %v", err)
	}
	if logResources.cleanup != nil {
		defer logResources.cleanup()
	}

	s := newServer(cfg, env, logResources.traceLogger)
	if cfg.DefaultModel == "" {
		if id := fetchFirstModel(s.ProxyClient); id != "" {
			s.Cfg.DefaultModel = id
			cfg.DefaultModel = id
			config.SaveAppConfig(cfg)
		}
	}
	mux := setupRouter(s)
	addr := fmt.Sprintf("%s:%d", *hostFlag, *portFlag)
	fmt.Printf("  Proxy Privacy — %s mode — redact-pii=%v redact-secrets=%v — %s\n", cfg.PrivacyMode, cfg.RedactPII, cfg.RedactSecrets, env.UpstreamBaseURL)
	fmt.Printf("  http://%s\n", addr)
	fmt.Println()
	srv := &http.Server{
		Addr:         addr,
		Handler:      withMiddleware(mux, s),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // streaming
		IdleTimeout:  120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func resolveConfig(providerFlag, privacyFlag, privacyShort, modelFlag, modelShort, upstreamFlag, upstreamKeyFlag, keyFlag, keyShort string, debugUpstreamFlag bool, traceDirFlag string, redactPIIFlag bool, redactSecretsFlag bool) (config.AppConfig, config.EnvConfig) {
	privacyMode := privacyFlag
	if privacyShort != "standard" {
		privacyMode = privacyShort
	}
	if privacyMode != "standard" && privacyMode != "strict" {
		log.Fatalf("Invalid privacy mode %q. Use 'standard' or 'strict'.", privacyMode)
	}

	model := modelFlag
	if modelShort != "" {
		model = modelShort
	}

	appCfg, err := config.LoadAppConfig()
	if err != nil {
		log.Printf("Warning: %v", err)
		appCfg = config.DefaultAppConfig()
	}
	appCfg.PrivacyMode = config.PrivacyMode(privacyMode)
	appCfg.DefaultModel = model
	appCfg.RedactPII = redactPIIFlag
	appCfg.RedactSecrets = redactSecretsFlag

	envCfg := config.LoadEnvConfig()

	configs, _ := config.LoadConfigs()
	if provider, ok := config.SelectProvider(configs, providerFlag); ok {
		if envCfg.UpstreamBaseURL == "" {
			envCfg.UpstreamBaseURL = provider.BaseURL
		}
		if envCfg.APIKey == "" {
			envCfg.APIKey = provider.APIKey
		}
		if appCfg.DefaultModel == "" && provider.DefaultModel != "" {
			appCfg.DefaultModel = provider.DefaultModel
		}
	} else if envCfg.UpstreamBaseURL == "" && envCfg.APIKey == "" {
		if providerFlag != "" {
			log.Fatalf("Provider %q not found in configs.json.", providerFlag)
		}
		log.Fatal("No providers configured. Create ~/.proxy-privacy/configs.json or set UPSTREAM_BASE_URL and OPENAI_API_KEY env vars.")
	}

	if upstreamFlag != "" {
		envCfg.UpstreamBaseURL = upstreamFlag
	}
	if upstreamKeyFlag != "" {
		envCfg.APIKey = upstreamKeyFlag
	}
	if envCfg.UpstreamBaseURL == "" {
		log.Fatal("Upstream URL is required. Configure a provider in configs.json or set UPSTREAM_BASE_URL env var.")
	}
	if envCfg.APIKey == "" {
		log.Fatal("API key is required. Configure a provider in configs.json or set OPENAI_API_KEY env var.")
	}

	if keyFlag != "" {
		envCfg.ProxyAPIKey = keyFlag
	} else if keyShort != "" {
		envCfg.ProxyAPIKey = keyShort
	}
	if debugUpstreamFlag {
		envCfg.DebugUpstream = true
	}
	if traceDirFlag != "" {
		envCfg.TraceDir = traceDirFlag
	}

	if err := config.SaveAppConfig(appCfg); err != nil {
		log.Printf("Warning: could not save config: %v", err)
	}

	return appCfg, envCfg
}

func cloneBody(body map[string]any) map[string]any {
	data, err := json.Marshal(body)
	if err != nil {
		log.Printf("cloneBody marshal error: %v", err)
		return shallowCopy(body)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		log.Printf("cloneBody unmarshal error: %v", err)
		return shallowCopy(body)
	}
	return clone
}

func shallowCopy(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func newServer(cfg config.AppConfig, env config.EnvConfig, traceLogger interface{ Printf(string, ...any) }) *Server {
	return &Server{
		Cfg:         cfg,
		Env:         env,
		ProxyClient: proxy.New(env.APIKey, env.UpstreamBaseURL, env.DebugUpstream, traceLogger),
		ModelCache:  NewModelCache(),
	}
}

func (s *Server) loadCfg() config.AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Cfg
}

func setupRouter(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/completions", s.handleCompletions)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/messages", s.handleAnthropicMessages)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/", s.handleProxyPassthrough)
	return mux
}

func (s *Server) handleProxyPassthrough(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.ProxyClient.ProxyRequest(w, r)
}

func listenAndServe(s *Server, addr string) (*http.Server, string, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", err
	}
	mux := setupRouter(s)
	actualAddr := listener.Addr().String()
	srv := &http.Server{
		Handler:      withMiddleware(mux, s),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	go srv.Serve(listener)
	return srv, actualAddr, nil
}

func withMiddleware(next http.Handler, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if !s.authenticate(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"message": "Invalid API key", "type": "auth_error"},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticate(r *http.Request) bool {
	if r.URL.Path == "/health" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return strings.TrimPrefix(auth, "Bearer ") == s.Env.ProxyAPIKey
}

const maxRequestBodySize = 10 << 20 // 10 MB

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) requireContentType(r *http.Request, w http.ResponseWriter, expected string) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, expected) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnsupportedMediaType)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Content-Type must be %s", expected),
		})
		return false
	}
	return true
}

func (s *Server) respondBytes(w http.ResponseWriter, status int, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		log.Printf("response write error: %v", err)
	}
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.requireContentType(r, w, "application/json") {
		return
	}

	body, err := s.readBody(r)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	c := s.loadCfg()
	s.forwardChatLikeJSON(w, reqBody, c, func(status int, headers http.Header, body []byte) {
		for k, vs := range filterHeaders(headers) {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		s.respondBytes(w, status, "application/json", body)
	})
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.requireContentType(r, w, "application/json") {
		return
	}

	body, err := s.readBody(r)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	chatBody, err := translateResponsesRequest(reqBody)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	c := s.loadCfg()
	s.forwardChatLikeJSON(w, chatBody, c, func(status int, headers http.Header, body []byte) {
		respBody, err := translateResponsesResponse(body)
		if err != nil {
			s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "response translation failed"})
			return
		}
		for k, vs := range filterHeaders(headers) {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		s.respondBytes(w, status, "application/json", respBody)
	})
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.requireContentType(r, w, "application/json") {
		return
	}

	body, err := s.readBody(r)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	c := s.loadCfg()
	model, _ := reqBody["model"].(string)
	if model == "" {
		model = c.DefaultModel
		reqBody["model"] = model
	}

	// Standard mode: try strict first, fall back on provider rejection
	if c.PrivacyMode == config.PrivacyStandard && !s.ModelCache.RejectsStrict(model) {
		clone := cloneBody(reqBody)
		clone = privacy.Apply(clone, config.PrivacyStrict, c.RedactPII, c.RedactSecrets)
		cloneBytes, _ := json.Marshal(clone)

		status, headers, respBody, err := s.ProxyClient.Completion(cloneBytes)
		if err == nil {
			if status < 400 {
				for k, vs := range filterHeaders(headers) {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write(respBody)
				return
			}
			if proxy.IsProviderError(respBody) {
				s.ModelCache.MarkRejectsStrict(model)
			} else {
				s.respondBytes(w, status, "application/json", respBody)
				return
			}
		} else {
			s.respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}

	reqBody = privacy.Apply(reqBody, c.PrivacyMode, c.RedactPII, c.RedactSecrets)
	modifiedBody, _ := json.Marshal(reqBody)

	status, headers, respBody, err := s.ProxyClient.Completion(modifiedBody)
	if err != nil {
		s.respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// model rejected privacy params — return error, don't retry
	if status >= 400 && proxy.IsProviderError(respBody) {
		s.respondBytes(w, proxy.FriendlyProviderStatus(), "application/json", proxy.FriendlyProviderError(model, string(c.PrivacyMode)))
		return
	}

	for k, vs := range filterHeaders(headers) {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(respBody)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	status, headers, body, err := s.ProxyClient.ListModels()
	if err != nil {
		s.respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	isAnthropic := r.Header.Get("anthropic-version") != "" || strings.HasPrefix(r.UserAgent(), "Claude")
	isCodex := r.URL.Query().Get("client_version") != "" || strings.HasPrefix(r.UserAgent(), "Codex")

	if isCodex && status < 400 {
		var oaiResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &oaiResp); err == nil && len(oaiResp.Data) > 0 {
			codexModels := make([]map[string]any, 0, len(oaiResp.Data))
			for _, m := range oaiResp.Data {
				codexModels = append(codexModels, map[string]any{
					"slug":                          m.ID,
					"display_name":                  m.ID,
					"supported_reasoning_levels":    []string{},
					"shell_type":                    "shell_command",
					"visibility":                    "list",
					"supported_in_api":              true,
					"priority":                      0,
					"base_instructions":             "",
					"supports_reasoning_summaries":  false,
					"truncation_policy":             map[string]any{"mode": "bytes", "limit": 100000},
					"supports_parallel_tool_calls":  true,
					"experimental_supported_tools":  []string{},
				})
			}
			body, _ = json.Marshal(map[string]any{"models": codexModels})
		}
	} else if isAnthropic && status < 400 {
		var oaiResp struct {
			Data []struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				Created int64  `json:"created"`
				OwnedBy string `json:"owned_by"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &oaiResp); err == nil {
			anthroData := make([]map[string]any, 0, len(oaiResp.Data))
			for _, m := range oaiResp.Data {
				anthroData = append(anthroData, map[string]any{
					"type":         "model",
					"id":           m.ID,
					"display_name": m.ID,
					"created_at":   time.Unix(m.Created, 0).UTC().Format(time.RFC3339),
				})
			}
			anthroResp := map[string]any{
				"data": anthroData,
			}
			if len(oaiResp.Data) > 0 {
				anthroResp["first_id"] = oaiResp.Data[0].ID
				anthroResp["last_id"] = oaiResp.Data[len(oaiResp.Data)-1].ID
			}
			anthroResp["has_more"] = false
			body, _ = json.Marshal(anthroResp)
		}
	}

	for k, vs := range filterHeaders(headers) {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c := s.loadCfg()
		s.respondJSON(w, http.StatusOK, c)
	case http.MethodPut:
		if !s.requireContentType(r, w, "application/json") {
			return
		}
		body, err := s.readBody(r)
		if err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		var update map[string]any
		if err := json.Unmarshal(body, &update); err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		s.mu.Lock()
		if pm, ok := update["privacy_mode"].(string); ok {
			if pm != "standard" && pm != "strict" {
				s.mu.Unlock()
				s.respondJSON(w, http.StatusBadRequest, map[string]any{
					"error": fmt.Sprintf("invalid privacy_mode. Must be 'standard' or 'strict'"),
				})
				return
			}
			s.Cfg.PrivacyMode = config.PrivacyMode(pm)
		}
		if dm, ok := update["default_model"].(string); ok {
			s.Cfg.DefaultModel = dm
		}
		if rp, ok := update["redact_pii"].(bool); ok {
			s.Cfg.RedactPII = rp
		}
		if rs, ok := update["redact_secrets"].(bool); ok {
			s.Cfg.RedactSecrets = rs
		}
		s.mu.Unlock()
		c := s.loadCfg()
		if err := config.SaveAppConfig(c); err != nil {
			log.Printf("Warning: could not save config: %v", err)
		}
		s.respondJSON(w, http.StatusOK, c)
	default:
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.requireContentType(r, w, "application/json") {
		return
	}

	body, err := s.readBody(r)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var anthroReq struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream,omitempty"`
	}
	if err := json.Unmarshal(body, &anthroReq); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	c := s.loadCfg()
	modifiedBody := body
	if anthroReq.Model == "" {
		modifiedBody = injectModel(body, c.DefaultModel)
	}

	openAIBody, err := anthropic.TranslateRequest(modifiedBody)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	reqBody := make(map[string]any)
	if err := json.Unmarshal(openAIBody, &reqBody); err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse translated request"})
		return
	}
	s.forwardChatLikeAnthropic(w, reqBody, c, anthroReq.Stream)
}

func (s *Server) forwardChatLikeJSON(w http.ResponseWriter, reqBody map[string]any, c config.AppConfig, onSuccess func(int, http.Header, []byte)) {
	model, isStream, status, headers, respBody, err := s.forwardChatLike(reqBody, c)
	if err != nil {
		s.respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if status >= 400 {
		if proxy.IsProviderError(respBody) {
			s.respondBytes(w, proxy.FriendlyProviderStatus(), "application/json", proxy.FriendlyProviderError(model, string(c.PrivacyMode)))
			return
		}
		s.respondBytes(w, status, "application/json", respBody)
		return
	}
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(status)
		if _, err := w.Write(respBody); err != nil {
			log.Printf("stream write error: %v", err)
		}
		return
	}
	onSuccess(status, headers, respBody)
}

func (s *Server) forwardChatLikeAnthropic(w http.ResponseWriter, reqBody map[string]any, c config.AppConfig, isStream bool) {
	_, _, status, headers, respBody, err := s.forwardChatLike(reqBody, c)
	if err != nil {
		s.respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if status >= 400 {
		if proxy.IsProviderError(respBody) {
			model, _ := reqBody["model"].(string)
			s.respondBytes(w, proxy.FriendlyProviderStatus(), "application/json", proxy.FriendlyProviderError(model, string(c.PrivacyMode)))
			return
		}
		s.respondBytes(w, status, "application/json", respBody)
		return
	}
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(status)

		scanner := bufio.NewScanner(strings.NewReader(string(respBody)))
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		translator := anthropic.NewStreamTranslator()
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			events := translator.Translate([]byte(data))
			for _, evt := range events {
				if _, err := w.Write(anthropic.FormatSSE(evt)); err != nil {
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("anthropic stream scanner error: %v", err)
		}
		return
	}

	translated, err := anthropic.TranslateResponse(respBody)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "response translation failed"})
		return
	}
	for k, vs := range filterHeaders(headers) {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	s.respondBytes(w, status, "application/json", translated)
}

func (s *Server) forwardChatLike(reqBody map[string]any, c config.AppConfig) (string, bool, int, http.Header, []byte, error) {
	model, _ := reqBody["model"].(string)
	if model == "" {
		model = c.DefaultModel
		reqBody["model"] = model
	}
	isStream := isStreaming(reqBody)

	if c.PrivacyMode == config.PrivacyStandard && !s.ModelCache.RejectsStrict(model) {
		clone := cloneBody(reqBody)
		clone = privacy.Apply(clone, config.PrivacyStrict, c.RedactPII, c.RedactSecrets)
		clone["stream"] = isStream

		status, headers, respBody, err := s.sendChatLike(clone, isStream)
		if err != nil {
			return model, isStream, 0, nil, nil, err
		}
		if status < 400 {
			return model, isStream, status, headers, respBody, nil
		}
		if proxy.IsProviderError(respBody) {
			s.ModelCache.MarkRejectsStrict(model)
		} else {
			return model, isStream, status, headers, respBody, nil
		}
	}

	reqBody = privacy.Apply(reqBody, c.PrivacyMode, c.RedactPII, c.RedactSecrets)
	reqBody["stream"] = isStream
	status, headers, respBody, err := s.sendChatLike(reqBody, isStream)
	return model, isStream, status, headers, respBody, err
}

func (s *Server) sendChatLike(reqBody map[string]any, isStream bool) (int, http.Header, []byte, error) {
	modifiedBody, _ := json.Marshal(reqBody)
	if isStream {
		resp, err := s.ProxyClient.ChatCompletionStream(modifiedBody)
		if err != nil {
			return 0, nil, nil, err
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, resp.Header, nil, err
		}
		return resp.StatusCode, resp.Header, respBody, nil
	}
	return s.ProxyClient.ChatCompletion(modifiedBody)
}

func translateResponsesRequest(reqBody map[string]any) (map[string]any, error) {
	chatBody := map[string]any{}
	if model, ok := reqBody["model"].(string); ok && model != "" {
		chatBody["model"] = model
	}
	if stream, ok := reqBody["stream"].(bool); ok {
		chatBody["stream"] = stream
	}
	if maxOutputTokens, ok := asInt(reqBody["max_output_tokens"]); ok {
		chatBody["max_tokens"] = maxOutputTokens
	}

	var messages []any
	if instructions, ok := reqBody["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": instructions,
		})
	}

	input, exists := reqBody["input"]
	if !exists {
		return nil, fmt.Errorf("missing input")
	}

	switch v := input.(type) {
	case string:
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": v,
		})
	case []any:
		for _, item := range v {
			msg, ok := responseInputItemToMessage(item)
			if !ok {
				return nil, fmt.Errorf("unsupported responses input item")
			}
			messages = append(messages, msg)
		}
	default:
		return nil, fmt.Errorf("unsupported input type")
	}

	chatBody["messages"] = messages
	return chatBody, nil
}

func responseInputItemToMessage(item any) (map[string]any, bool) {
	msg, ok := item.(map[string]any)
	if !ok {
		return nil, false
	}
	role, _ := msg["role"].(string)
	if role == "" {
		role = "user"
	}
	content, err := responseContentToText(msg["content"])
	if err != nil {
		return nil, false
	}
	return map[string]any{
		"role":    role,
		"content": content,
	}, true
}

func responseContentToText(content any) (string, error) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("unsupported content type")
	}
}

func translateResponsesResponse(body []byte) ([]byte, error) {
	var upstream struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, err
	}

	role := "assistant"
	text := ""
	if len(upstream.Choices) > 0 {
		if upstream.Choices[0].Message.Role != "" {
			role = upstream.Choices[0].Message.Role
		}
		text = upstream.Choices[0].Message.Content
	}

	resp := map[string]any{
		"id":         upstream.ID,
		"object":     "response",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"model":      upstream.Model,
		"output": []any{
			map[string]any{
				"type": "message",
				"role": role,
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": text,
					},
				},
			},
		},
		"output_text": text,
	}
	if upstream.Usage != nil {
		resp["usage"] = upstream.Usage
	}
	return json.Marshal(resp)
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func filterHeaders(headers http.Header) http.Header {
	filtered := make(http.Header)
	for k, vs := range headers {
		if strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Content-Type") {
			continue
		}
		for _, v := range vs {
			filtered.Add(k, v)
		}
	}
	return filtered
}

func fetchFirstModel(client *proxy.Client) string {
	_, _, body, err := client.ListModels()
	if err != nil {
		return ""
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &resp) != nil || len(resp.Data) == 0 {
		return ""
	}
	return resp.Data[0].ID
}

func isStreaming(body map[string]any) bool {
	if v, ok := body["stream"].(bool); ok {
		return v
	}
	if s, ok := body["stream"].(string); ok {
		return s == "true" || s == "1"
	}
	return false
}

func injectModel(body []byte, model string) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	if _, ok := m["model"].(string); !ok || m["model"] == "" {
		m["model"] = model
	}
	out, _ := json.Marshal(m)
	return out
}
