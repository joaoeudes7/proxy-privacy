package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/joaoeudes7/proxy-privacy/internal/config"
	"github.com/joaoeudes7/proxy-privacy/internal/proxy"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newProxyTestHandler(t *testing.T, cfg config.AppConfig, transport roundTripFunc) http.Handler {
	t.Helper()
	client := &http.Client{Transport: transport}
	s := &Server{
		Cfg:         cfg,
		Env:         config.EnvConfig{APIKey: "upstream-key", ProxyAPIKey: "proxy-key", UpstreamBaseURL: "http://upstream.test"},
		ProxyClient: proxy.NewWithHTTPClients("upstream-key", "http://upstream.test", "", false, nil, client, client),
		ModelCache:  NewModelCache(),
	}
	return withMiddleware(setupRouter(s), s)
}

func performRequest(t *testing.T, handler http.Handler, method, path, authHeader, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newProxyClient(t *testing.T, transport roundTripFunc) *proxy.Client {
	t.Helper()
	httpClient := &http.Client{Transport: transport}
	return proxy.NewWithHTTPClients("test-key", "http://upstream.test", "", false, nil, httpClient, httpClient)
}

func TestFetchFirstModel_Success(t *testing.T) {
	client := newProxyClient(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://upstream.test/models" {
			t.Fatalf("url = %q, want %q", req.URL.String(), "http://upstream.test/models")
		}
		return jsonResponse(http.StatusOK, `{"data":[{"id":"model-a"},{"id":"model-b"}]}`), nil
	})
	id := fetchFirstModel(client)
	if id != "model-a" {
		t.Fatalf("fetchFirstModel = %q, want %q", id, "model-a")
	}
}

func TestFetchFirstModel_EmptyData(t *testing.T) {
	client := newProxyClient(t, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"data":[]}`), nil
	})
	if id := fetchFirstModel(client); id != "" {
		t.Fatalf("fetchFirstModel = %q, want empty", id)
	}
}

func TestFetchFirstModel_ErrorStatus(t *testing.T) {
	client := newProxyClient(t, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, `{"error":"upstream down"}`), nil
	})
	if id := fetchFirstModel(client); id != "" {
		t.Fatalf("fetchFirstModel = %q, want empty", id)
	}
}

func TestFetchFirstModel_InvalidJSON(t *testing.T) {
	client := newProxyClient(t, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `not json`), nil
	})
	if id := fetchFirstModel(client); id != "" {
		t.Fatalf("fetchFirstModel = %q, want empty", id)
	}
}

func TestFetchFirstModel_HTTPError(t *testing.T) {
	client := newProxyClient(t, func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	if id := fetchFirstModel(client); id != "" {
		t.Fatalf("fetchFirstModel = %q, want empty", id)
	}
}

func performRequestWithHeaders(t *testing.T, handler http.Handler, method, path, authHeader, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestModelsForOpenCodeAndPi_UsesOpenAIFormat(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"data":[{"id":"model-a"},{"id":"model-b"}]}`), nil
	})

	// OpenCode — no special headers, expects standard OpenAI format
	resp := performRequest(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	var openCodeResp struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&openCodeResp); err != nil {
		t.Fatal(err)
	}
	if len(openCodeResp.Data) != 2 {
		t.Fatalf("len(data) = %d, want %d", len(openCodeResp.Data), 2)
	}
	if openCodeResp.Data[0].ID != "model-a" {
		t.Fatalf("data[0].id = %q, want %q", openCodeResp.Data[0].ID, "model-a")
	}

	// Pi — same standard OpenAI format
	resp = performRequest(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	var piResp struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&piResp); err != nil {
		t.Fatal(err)
	}
	if len(piResp.Data) != 2 {
		t.Fatalf("len(data) = %d, want %d", len(piResp.Data), 2)
	}
}

func TestModelsForClaude_UsesAnthropicFormat(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"data":[{"id":"claude-sonnet-4","object":"model","created":1710000000,"owned_by":"anthropic"}]}`), nil
	})

	// Claude via anthropic-version header
	resp := performRequestWithHeaders(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "",
		map[string]string{"anthropic-version": "2023-06-01"})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var anthroResp struct {
		Data []struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
		} `json:"data"`
		FirstID string `json:"first_id"`
		LastID  string `json:"last_id"`
		HasMore bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		t.Fatal(err)
	}
	if len(anthroResp.Data) != 1 {
		t.Fatalf("len(data) = %d, want %d", len(anthroResp.Data), 1)
	}
	if anthroResp.Data[0].Type != "model" {
		t.Fatalf("type = %q, want %q", anthroResp.Data[0].Type, "model")
	}
	if anthroResp.Data[0].ID != "claude-sonnet-4" {
		t.Fatalf("id = %q, want %q", anthroResp.Data[0].ID, "claude-sonnet-4")
	}
	if anthroResp.FirstID != "claude-sonnet-4" {
		t.Fatalf("first_id = %q, want %q", anthroResp.FirstID, "claude-sonnet-4")
	}
	if anthroResp.LastID != "claude-sonnet-4" {
		t.Fatalf("last_id = %q, want %q", anthroResp.LastID, "claude-sonnet-4")
	}
	if anthroResp.HasMore {
		t.Fatal("has_more = true, want false")
	}

	// Claude via User-Agent header
	resp = performRequestWithHeaders(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "",
		map[string]string{"User-Agent": "ClaudeCode/1.0"})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		t.Fatal(err)
	}
	if len(anthroResp.Data) != 1 || anthroResp.Data[0].Type != "model" {
		t.Fatal("expected anthropic format for Claude User-Agent")
	}
}

func TestModelsForCodex_UsesCodexFormat(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"data":[{"id":"gpt-5-preview"},{"id":"gpt-5"}]}`), nil
	})

	// Codex via client_version query param
	resp := performRequest(t, handler, http.MethodGet, "/v1/models?client_version=1.0", "Bearer proxy-key", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var codexResp struct {
		Models []struct {
			Slug              string `json:"slug"`
			DisplayName       string `json:"display_name"`
			Visibility        string `json:"visibility"`
			SupportedInAPI    bool   `json:"supported_in_api"`
			ShellType         string `json:"shell_type"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&codexResp); err != nil {
		t.Fatal(err)
	}
	if len(codexResp.Models) != 2 {
		t.Fatalf("len(models) = %d, want %d", len(codexResp.Models), 2)
	}
	if codexResp.Models[0].Slug != "gpt-5-preview" {
		t.Fatalf("models[0].slug = %q, want %q", codexResp.Models[0].Slug, "gpt-5-preview")
	}
	if codexResp.Models[0].Visibility != "list" {
		t.Fatalf("visibility = %q, want %q", codexResp.Models[0].Visibility, "list")
	}
	if !codexResp.Models[0].SupportedInAPI {
		t.Fatal("supported_in_api = false, want true")
	}
	if codexResp.Models[0].ShellType != "shell_command" {
		t.Fatalf("shell_type = %q, want %q", codexResp.Models[0].ShellType, "shell_command")
	}
	if codexResp.Models[1].Slug != "gpt-5" {
		t.Fatalf("models[1].slug = %q, want %q", codexResp.Models[1].Slug, "gpt-5")
	}

	// Codex via User-Agent
	resp = performRequestWithHeaders(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "",
		map[string]string{"User-Agent": "Codex/1.0"})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	var codexResp2 struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&codexResp2); err != nil {
		t.Fatal(err)
	}
	if len(codexResp2.Models) != 2 {
		t.Fatalf("len(models) = %d, want %d", len(codexResp2.Models), 2)
	}
}

func TestModelsWithUpstreamError_PassesThrough(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, `{"error":"upstream down"}`), nil
	})

	// OpenAI client — passes through error
	resp := performRequest(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "")
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadGateway)
	}

	// Claude client — passes through error
	resp = performRequestWithHeaders(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "",
		map[string]string{"anthropic-version": "2023-06-01"})
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadGateway)
	}

	// Codex client — passes through error
	resp = performRequest(t, handler, http.MethodGet, "/v1/models?client_version=1.0", "Bearer proxy-key", "")
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadGateway)
	}
}

func TestHealthUnauthenticatedAndModelsAuthenticated(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/models" {
			t.Fatalf("unexpected upstream path %s", req.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"data":[{"id":"model-a"}]}`), nil
	})

	health := performRequest(t, handler, http.MethodGet, "/health", "", "")
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	modelsUnauthorized := performRequest(t, handler, http.MethodGet, "/v1/models", "", "")
	if modelsUnauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("models unauth status = %d, want %d", modelsUnauthorized.Code, http.StatusUnauthorized)
	}

	modelsAuthorized := performRequest(t, handler, http.MethodGet, "/v1/models", "Bearer proxy-key", "")
	if modelsAuthorized.Code != http.StatusOK {
		t.Fatalf("models auth status = %d, want %d", modelsAuthorized.Code, http.StatusOK)
	}
}

func TestChatCompletionsFallbackCachesStrictRejection(t *testing.T) {
	var mu sync.Mutex
	var seen []bool

	handler := newProxyTestHandler(t, config.AppConfig{
		DefaultModel: "fallback-model",
		PrivacyMode:  config.PrivacyStandard,
		RedactPII:    true,
	}, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected upstream path %s", req.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		provider := body["provider"].(map[string]any)
		_, hasZDR := provider["zdr"]

		mu.Lock()
		seen = append(seen, hasZDR)
		mu.Unlock()

		if hasZDR {
			return jsonResponse(http.StatusBadRequest, `{"error":{"message":"zdr unsupported"}}`), nil
		}

		if body["model"] != "fallback-model" {
			t.Fatalf("model = %v, want fallback-model", body["model"])
		}

		return jsonResponse(http.StatusOK, `{"id":"chatcmpl-1","model":"fallback-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})

	requestBody := `{"messages":[{"role":"user","content":"hello"}]}`
	for i := 0; i < 2; i++ {
		resp := performRequest(t, handler, http.MethodPost, "/v1/chat/completions", "Bearer proxy-key", requestBody)
		if resp.Code != http.StatusOK {
			t.Fatalf("chat status = %d body=%s", resp.Code, resp.Body.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("upstream attempts = %d, want 3", len(seen))
	}
	if !seen[0] || seen[1] || seen[2] {
		t.Fatalf("strict fallback sequence = %v, want [true false false]", seen)
	}
}

func TestResponsesEndpointTranslatesRequestAndResponse(t *testing.T) {
	handler := newProxyTestHandler(t, config.DefaultAppConfig(), func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected upstream path %s", req.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		if body["model"] != "gpt-test" {
			t.Fatalf("model = %v, want gpt-test", body["model"])
		}
		msgs, ok := body["messages"].([]any)
		if !ok || len(msgs) != 2 {
			t.Fatalf("messages = %v, want 2 entries", body["messages"])
		}
		systemMsg := msgs[0].(map[string]any)
		userMsg := msgs[1].(map[string]any)
		if systemMsg["content"] != "Be concise." {
			t.Fatalf("system content = %v", systemMsg["content"])
		}
		if userMsg["content"] != "Hello proxy" {
			t.Fatalf("user content = %v", userMsg["content"])
		}

		return jsonResponse(http.StatusOK, `{"id":"resp-1","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"Hi there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`), nil
	})

	resp := performRequest(t, handler, http.MethodPost, "/v1/responses", "Bearer proxy-key", `{"model":"gpt-test","instructions":"Be concise.","input":"Hello proxy"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("responses status = %d body=%s", resp.Code, resp.Body.String())
	}

	var translated map[string]any
	if err := json.NewDecoder(bytes.NewReader(resp.Body.Bytes())).Decode(&translated); err != nil {
		t.Fatal(err)
	}
	if translated["object"] != "response" {
		t.Fatalf("object = %v, want response", translated["object"])
	}
	if translated["output_text"] != "Hi there" {
		t.Fatalf("output_text = %v, want Hi there", translated["output_text"])
	}
	output := translated["output"].([]any)
	message := output[0].(map[string]any)
	content := message["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "output_text" || block["text"] != "Hi there" {
		t.Fatalf("output block = %v", block)
	}
}

func TestChatCompletionsPrivacyProviderErrorUsesNormalizedStatus(t *testing.T) {
	handler := newProxyTestHandler(t, config.AppConfig{
		DefaultModel: "strict-model",
		PrivacyMode:  config.PrivacyStrict,
		RedactPII:    true,
	}, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"error":{"message":"No endpoints found matching your data policy (Zero data retention). Configure: https://openrouter.ai/settings/privacy"}}`), nil
	})

	resp := performRequest(t, handler, http.MethodPost, "/v1/chat/completions", "Bearer proxy-key", `{"messages":[{"role":"user","content":"hello"}]}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("chat status = %d, want %d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func TestFormatModelDisplayName(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"openai/gpt-4o", "GPT 4o (openai/gpt-4o)"},
		{"anthropic/claude-sonnet-4", "Claude Sonnet 4 (anthropic/claude-sonnet-4)"},
		{"mistralai/mixtral-8x7b-instruct", "Mixtral 8x7b Instruct (mistralai/mixtral-8x7b-instruct)"},
		{"meta-llama/llama-3.3-70b-instruct", "Llama 3.3 70B Instruct (meta-llama/llama-3.3-70b-instruct)"},
		{"openai/gpt-4o-mini", "GPT 4o Mini (openai/gpt-4o-mini)"},
		{"model-a", "Model A (model-a)"},
	}
	for _, tc := range tests {
		got := formatModelDisplayName(tc.id)
		if got != tc.want {
			t.Errorf("formatModelDisplayName(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestResponsesPrivacyProviderErrorUsesNormalizedStatus(t *testing.T) {
	handler := newProxyTestHandler(t, config.AppConfig{
		DefaultModel: "strict-model",
		PrivacyMode:  config.PrivacyStrict,
		RedactPII:    true,
	}, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"error":{"message":"No endpoints found matching your data policy (Zero data retention). Configure: https://openrouter.ai/settings/privacy"}}`), nil
	})

	resp := performRequest(t, handler, http.MethodPost, "/v1/responses", "Bearer proxy-key", `{"model":"strict-model","input":"hello"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("responses status = %d, want %d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}
