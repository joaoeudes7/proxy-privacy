package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient   *http.Client
	streamClient *http.Client
	baseURL      string
	apiKey       string
	clientName   string
	debug        bool
	logger       debugLogger
}

type debugLogger interface {
	Printf(format string, v ...any)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

func newStreamClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

const defaultClientName = "github.com/joaoeudes7/proxy-privacy"

func New(apiKey, baseURL, clientName string, debug bool, logger debugLogger) *Client {
	return NewWithHTTPClients(apiKey, baseURL, clientName, debug, logger, newHTTPClient(120*time.Second), newStreamClient())
}

func NewWithHTTPClients(apiKey, baseURL, clientName string, debug bool, logger debugLogger, httpClient, streamClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = newHTTPClient(120 * time.Second)
	}
	if streamClient == nil {
		streamClient = newStreamClient()
	}
	if clientName == "" {
		clientName = defaultClientName
	}
	return &Client{
		httpClient:   httpClient,
		streamClient: streamClient,
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		clientName:   clientName,
		debug:        debug,
		logger:       logger,
	}
}

func (c *Client) headers() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+c.apiKey)
	h.Set("Content-Type", "application/json")
	h.Set("X-Title", c.clientName)
	return h
}

func (c *Client) ChatCompletion(body []byte) (int, http.Header, []byte, error) {
	return c.doRequest("POST", c.baseURL+"/chat/completions", body)
}

func (c *Client) ChatCompletionStream(body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = c.headers()
	c.debugRequest(req, body)
	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) ListModels() (int, http.Header, []byte, error) {
	return c.doRequest("GET", c.baseURL+"/models", nil)
}

func (c *Client) Completion(body []byte) (int, http.Header, []byte, error) {
	return c.doRequest("POST", c.baseURL+"/completions", body)
}

func (c *Client) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	upstreamURL := c.baseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequest(r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	req.Header = c.headers()
	for k, vs := range r.Header {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Content-Type") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	c.debugRequest(req, body)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

const maxResponseBodySize = 50 << 20 // 50 MB

func (c *Client) doRequest(method, url string, body []byte) (int, http.Header, []byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header = c.headers()
	c.debugRequest(req, body)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	resp.Body = http.MaxBytesReader(nil, resp.Body, maxResponseBodySize)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("read failed: %w", err)
	}
	return resp.StatusCode, resp.Header, respBody, nil
}

func (c *Client) debugRequest(req *http.Request, body []byte) {
	if !c.debug {
		return
	}

	headers := make(map[string]string, len(req.Header))
	for key, values := range req.Header {
		value := strings.Join(values, ", ")
		if strings.EqualFold(key, "Authorization") {
			value = maskBearerToken(value)
		}
		headers[key] = value
	}

	summary := map[string]any{}
	if len(body) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			if model, ok := payload["model"]; ok {
				summary["model"] = model
			}
			if provider, ok := payload["provider"]; ok {
				summary["provider"] = provider
			}
			if stream, ok := payload["stream"]; ok {
				summary["stream"] = stream
			}
		}
	}

	logger := c.logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("upstream request method=%s url=%s headers=%v payload=%v", req.Method, req.URL.String(), headers, summary)
}

func maskBearerToken(value string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return "[REDACTED]"
	}
	token := strings.TrimPrefix(value, prefix)
	if len(token) <= 8 {
		return prefix + "[REDACTED]"
	}
	return prefix + token[:4] + "..." + token[len(token)-4:]
}

func IsProviderError(body []byte) bool {
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	msg := strings.ToLower(result.Error.Message)
	keywords := []string{
		"data_collection",
		"zdr",
		"zero data retention",
		"data policy",
		"allow_fallbacks",
	}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func FriendlyProviderError(model, privacyMode string) []byte {
	err := map[string]any{
		"error": map[string]any{
			"message":    fmt.Sprintf("Model '%s' may not support the privacy parameters required by '%s' mode. This often happens with free or limited models. Try switching to 'standard' privacy mode, or use a different model.", model, privacyMode),
			"type":       "provider_privacy_error",
			"suggestion": "Set privacy_mode to 'standard' to disable zdr and allow fallbacks, or use a model that supports strict privacy (e.g., paid models).",
		},
	}
	data, _ := json.Marshal(err)
	return data
}

func FriendlyProviderStatus() int {
	return http.StatusBadRequest
}
