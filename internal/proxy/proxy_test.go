package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func newMockClient(apiKey, baseURL string, fn roundTripFunc) *Client {
	return NewWithHTTPClients(apiKey, baseURL, "test-client", false, nil, &http.Client{Transport: fn}, &http.Client{Transport: fn})
}

type errorReader struct{}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (r *errorReader) Close() error {
	return nil
}

func TestNew_CreatesClient(t *testing.T) {
	t.Parallel()

	c := New("sk-key", "https://test.com", "my-client", false, nil)

	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.apiKey != "sk-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "sk-key")
	}
	if c.baseURL != "https://test.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://test.com")
	}
	if c.clientName != "my-client" {
		t.Errorf("clientName = %q, want %q", c.clientName, "my-client")
	}
	if c.debug {
		t.Error("debug = true, want false")
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
	if c.streamClient == nil {
		t.Error("streamClient is nil")
	}
}

func TestNewWithHTTPClients_CustomClients(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{}
	streamClient := &http.Client{}
	c := NewWithHTTPClients("key", "https://test.com", "", false, nil, httpClient, streamClient)

	if c.httpClient != httpClient {
		t.Error("httpClient was not the injected one")
	}
	if c.streamClient != streamClient {
		t.Error("streamClient was not the injected one")
	}
}

func TestNewWithHTTPClients_NilClientsGetDefaults(t *testing.T) {
	t.Parallel()

	c := NewWithHTTPClients("key", "https://test.com", "c", false, nil, nil, nil)

	if c.httpClient == nil {
		t.Error("httpClient should have been initialized")
	}
	if c.streamClient == nil {
		t.Error("streamClient should have been initialized")
	}
}

func TestNewWithHTTPClients_EmptyNameDefaults(t *testing.T) {
	t.Parallel()

	c := NewWithHTTPClients("key", "https://test.com", "", false, nil, &http.Client{}, &http.Client{})

	if c.clientName != defaultClientName {
		t.Errorf("clientName = %q, want default %q", c.clientName, defaultClientName)
	}
}

func TestNewWithHTTPClients_TrailingSlashRemoved(t *testing.T) {
	t.Parallel()

	c := NewWithHTTPClients("key", "https://test.com/v1/", "c", false, nil, &http.Client{}, &http.Client{})

	if c.baseURL != "https://test.com/v1" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://test.com/v1")
	}
}

func TestHeaders_SetsCorrectValues(t *testing.T) {
	t.Parallel()

	c := NewWithHTTPClients("sk-test-123", "https://test.com", "my-client", false, nil, nil, nil)
	h := c.headers()

	if h.Get("Authorization") != "Bearer sk-test-123" {
		t.Errorf("Authorization = %q, want %q", h.Get("Authorization"), "Bearer sk-test-123")
	}
	if h.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", h.Get("Content-Type"), "application/json")
	}
	if h.Get("X-Title") != "my-client" {
		t.Errorf("X-Title = %q, want %q", h.Get("X-Title"), "my-client")
	}
}

func TestChatCompletion_Success(t *testing.T) {
	t.Parallel()

	mockResp := map[string]any{"id": "chat-123", "choices": []any{map[string]any{"text": "hello"}}}
	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" {
			t.Errorf("method = %q, want POST", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/chat/completions") {
			t.Errorf("URL = %q, want /chat/completions", req.URL.String())
		}
		return jsonResponse(http.StatusOK, mockResp), nil
	})

	body, _ := json.Marshal(map[string]any{"model": "gpt-4"})
	status, _, respBody, err := c.ChatCompletion(body)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["id"] != "chat-123" {
		t.Errorf("id = %v, want chat-123", parsed["id"])
	}
}

func TestChatCompletion_NetworkError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	_, _, _, err := c.ChatCompletion([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChatCompletionStream_Success(t *testing.T) {
	t.Parallel()

	streamData := `data: {"choices":[{"delta":{"content":"hello"}}]}`
	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" {
			t.Errorf("method = %q, want POST", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/chat/completions") {
			t.Errorf("URL = %q, want /chat/completions", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(streamData)),
		}, nil
	})

	body, _ := json.Marshal(map[string]any{"model": "gpt-4", "stream": true})
	resp, err := c.ChatCompletionStream(body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "hello") {
		t.Errorf("body = %q, want to contain 'hello'", string(respBody))
	}
}

func TestChatCompletionStream_NetworkError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	_, err := c.ChatCompletionStream([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListModels_Success(t *testing.T) {
	t.Parallel()

	mockResp := map[string]any{"data": []any{map[string]any{"id": "gpt-4"}}}
	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" {
			t.Errorf("method = %q, want GET", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/models") {
			t.Errorf("URL = %q, want /models", req.URL.String())
		}
		return jsonResponse(http.StatusOK, mockResp), nil
	})

	status, _, respBody, err := c.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		t.Fatal(err)
	}
	data := parsed["data"].([]any)
	if len(data) != 1 {
		t.Errorf("models = %v, want 1 model", data)
	}
}

func TestListModels_NetworkError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	_, _, _, err := c.ListModels()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCompletion_Success(t *testing.T) {
	t.Parallel()

	mockResp := map[string]any{"id": "cmp-123", "choices": []any{map[string]any{"text": "completion"}}}
	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" {
			t.Errorf("method = %q, want POST", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/completions") {
			t.Errorf("URL = %q, want /completions", req.URL.String())
		}
		return jsonResponse(http.StatusOK, mockResp), nil
	})

	body, _ := json.Marshal(map[string]any{"model": "gpt-4", "prompt": "hello"})
	status, _, respBody, err := c.Completion(body)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["id"] != "cmp-123" {
		t.Errorf("id = %v, want cmp-123", parsed["id"])
	}
}

func TestCompletion_NetworkError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	_, _, _, err := c.Completion([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDoRequest_NetworkError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	_, _, _, err := c.doRequest("GET", "https://api.test.com/models", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDoRequest_ReadError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://api.test.com", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(&errorReader{}),
		}, nil
	})
	_, _, _, err := c.doRequest("GET", "https://api.test.com/models", nil)
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestDebugRequest_DisabledDoesNotLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	c := NewWithHTTPClients("key", "https://test.com", "c", false, logger, &http.Client{}, &http.Client{})
	req, _ := http.NewRequest("POST", "https://test.com/chat", nil)
	c.debugRequest(req, []byte(`{"model":"gpt-4"}`))
	if buf.Len() > 0 {
		t.Errorf("expected no log output when debug=false, got: %s", buf.String())
	}
}

func TestDebugRequest_LogsRequestSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	c := NewWithHTTPClients("key", "https://test.com", "c", true, logger, &http.Client{}, &http.Client{})
	req, _ := http.NewRequest("POST", "https://test.com/chat/completions", nil)
	c.debugRequest(req, []byte(`{"model":"gpt-4","stream":true}`))

	output := buf.String()
	if !strings.Contains(output, "upstream request") {
		t.Errorf("log output = %q, want to contain 'upstream request'", output)
	}
	if !strings.Contains(output, "gpt-4") {
		t.Errorf("log output = %q, want to contain model", output)
	}
	if !strings.Contains(output, "stream") {
		t.Errorf("log output = %q, want to contain stream", output)
	}
}

func TestDebugRequest_NilLoggerUsesDefault(t *testing.T) {
	t.Parallel()

	c := NewWithHTTPClients("key", "https://test.com", "c", true, nil, &http.Client{}, &http.Client{})
	req, _ := http.NewRequest("POST", "https://test.com/chat", nil)
	c.debugRequest(req, []byte(`{"model":"gpt-4"}`))
}

func TestDebugRequest_MasksAuthorizationHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	c := NewWithHTTPClients("sk-secret-api-key-long", "https://test.com", "c", true, logger, &http.Client{}, &http.Client{})
	req, _ := http.NewRequest("POST", "https://test.com/chat", nil)
	req.Header = c.headers()
	c.debugRequest(req, []byte(`{}`))

	output := buf.String()
	if strings.Contains(output, "sk-secret-api-key-long") {
		t.Errorf("log output should not contain the full API key, got: %s", output)
	}
	if !strings.Contains(output, "sk-s") {
		t.Errorf("log output should contain masked key, got: %s", output)
	}
}

func TestDebugRequest_EmptyBody(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	c := NewWithHTTPClients("key", "https://test.com", "c", true, logger, &http.Client{}, &http.Client{})
	req, _ := http.NewRequest("GET", "https://test.com/models", nil)
	c.debugRequest(req, nil)

	output := buf.String()
	if !strings.Contains(output, "upstream request") {
		t.Errorf("log output = %q, want 'upstream request'", output)
	}
}

func TestMaskBearerToken_Normal(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("Bearer sk-secret-key-12345")
	want := "Bearer sk-s...2345"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestMaskBearerToken_ShortToken(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("Bearer ab")
	want := "Bearer [REDACTED]"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestMaskBearerToken_ExactlyEightChars(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("Bearer 12345678")
	want := "Bearer [REDACTED]"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestMaskBearerToken_NineChars(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("Bearer 123456789")
	want := "Bearer 1234...6789"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestMaskBearerToken_NoBearerPrefix(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("Token abc")
	want := "[REDACTED]"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestMaskBearerToken_EmptyValue(t *testing.T) {
	t.Parallel()

	result := maskBearerToken("")
	want := "[REDACTED]"
	if result != want {
		t.Errorf("maskBearerToken = %q, want %q", result, want)
	}
}

func TestFriendlyProviderError_Format(t *testing.T) {
	t.Parallel()

	data := FriendlyProviderError("gpt-4", "strict")
	if !json.Valid(data) {
		t.Fatal("FriendlyProviderError returned invalid JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatal("expected 'error' key with object value")
	}

	msg := errObj["message"].(string)
	if !strings.Contains(msg, "gpt-4") || !strings.Contains(msg, "strict") {
		t.Errorf("message = %q, want to contain model and mode", msg)
	}

	if errObj["type"] != "provider_privacy_error" {
		t.Errorf("type = %v, want provider_privacy_error", errObj["type"])
	}

	sug := errObj["suggestion"].(string)
	if !strings.Contains(sug, "standard") {
		t.Errorf("suggestion = %q, want to mention standard mode", sug)
	}
}

func TestFriendlyProviderStatus_IsBadRequest(t *testing.T) {
	t.Parallel()

	status := FriendlyProviderStatus()
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestProxyRequest_Passthrough(t *testing.T) {
	t.Parallel()

	upstreamBody := `{"result":"ok"}`
	c := newMockClient("key", "https://upstream.test.com", func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" {
			t.Errorf("method = %q, want POST", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/v1/completions?foo=bar") {
			t.Errorf("URL = %q, want /v1/completions?foo=bar", req.URL.String())
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "test-prompt") {
			t.Errorf("body = %q, want 'test-prompt'", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}, nil
	})

	req := httptest.NewRequest("POST", "/v1/completions?foo=bar", strings.NewReader(`{"prompt":"test-prompt"}`))
	w := httptest.NewRecorder()
	c.ProxyRequest(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if strings.TrimSpace(string(body)) != upstreamBody {
		t.Errorf("body = %q, want %q", strings.TrimSpace(string(body)), upstreamBody)
	}
}

func TestProxyRequest_UpstreamError(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://upstream.test.com", func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})

	req := httptest.NewRequest("POST", "/v1/completions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	c.ProxyRequest(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
	if !strings.Contains(string(body), "upstream request failed") {
		t.Errorf("body = %q, want upstream error message", string(body))
	}
}

func TestProxyRequest_CopiesResponseHeaders(t *testing.T) {
	t.Parallel()

	c := newMockClient("key", "https://upstream.test.com", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Custom":     []string{"response-value"},
			},
			Body: io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	c.ProxyRequest(w, req)

	resp := w.Result()
	resp.Body.Close()
	if resp.Header.Get("X-Custom") != "response-value" {
		t.Errorf("X-Custom header = %q, want %q", resp.Header.Get("X-Custom"), "response-value")
	}
}

func TestProxyRequest_RequestError(t *testing.T) {
	t.Parallel()

	c := New("key", "://invalid", "c", false, nil)

	req := httptest.NewRequest("POST", "/v1/chat", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	c.ProxyRequest(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
	if !strings.Contains(string(body), "unsupported protocol scheme") && !strings.Contains(string(body), "Bad Gateway") {
		// Should have some error message
		if len(body) == 0 {
			t.Error("expected error body, got empty")
		}
	}
}

func TestIsProviderError_DetectsPrivacyPolicyMessages(t *testing.T) {
	t.Parallel()

	cases := []string{
		`{"error":{"message":"No endpoints found matching your data policy (Zero data retention). Configure: https://openrouter.ai/settings/privacy"}}`,
		`{"error":{"message":"Model rejected provider.zdr=true"}}`,
		`{"error":{"message":"provider.allow_fallbacks=false is not supported"}}`,
	}

	for _, body := range cases {
		if !IsProviderError([]byte(body)) {
			t.Errorf("expected provider error for body: %s", body)
		}
	}
}

func TestIsProviderError_IgnoresNonPrivacyErrors(t *testing.T) {
	t.Parallel()

	body := `{"error":{"message":"model not found"}}`
	if IsProviderError([]byte(body)) {
		t.Errorf("unexpected provider error for body: %s", body)
	}
}

func TestIsProviderError_InvalidJSON(t *testing.T) {
	t.Parallel()

	body := `not json`
	if IsProviderError([]byte(body)) {
		t.Error("IsProviderError should return false for invalid JSON")
	}
}

func TestIsProviderError_NoErrorKey(t *testing.T) {
	t.Parallel()

	body := `{"id":"123"}`
	if IsProviderError([]byte(body)) {
		t.Error("IsProviderError should return false when no error key")
	}
}
